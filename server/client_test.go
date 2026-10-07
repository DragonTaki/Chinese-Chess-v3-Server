/* ----- ----- ----- ----- */
// client_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// flush waits until every packet queued to c so far is written to f (a marker packet is queued
// after them and waited for).
func flush(t *testing.T, c *Client, f *fakeConn) {
	t.Helper()
	marker := "flush-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	c.SendPacket(CreatePacket("TestFlush", "", "", marker, ""))
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(time.Millisecond) {
		f.mu.Lock()
		got := strings.Contains(f.buf.String(), marker)
		f.mu.Unlock()
		if got {
			return
		}
	}
	t.Fatal("queued packets not written")
}

// stoppedWithin reports whether c's writer goroutine ends within d.
func stoppedWithin(c *Client, d time.Duration) bool {
	select {
	case <-c.stopped:
		return true
	case <-time.After(d):
		return false
	}
}

// Packets reach the connection in the order sent, also those still queued at Finish; the writer
// then ends and the connection is closed.
func TestSendOrderAndFinish(t *testing.T) {
	f := newFakeConn()
	c := NewClient(f, nil)
	for i := 0; i < 100; i++ {
		c.SendPacket(CreatePacket(PacketTypeServer, "", "", strconv.Itoa(i), ""))
	}
	c.Finish()
	if !stoppedWithin(c, 2*time.Second) {
		t.Fatal("writer did not end")
	}
	got := f.packets(PacketTypeServer)
	if len(got) != 100 {
		t.Fatalf("%d packets written", len(got))
	}
	for i, p := range got {
		if p.Data != strconv.Itoa(i) {
			t.Fatalf("packet %d is %s", i, p.Data)
		}
	}
	select {
	case <-f.closed:
	case <-time.After(time.Second):
		t.Fatal("connection not closed")
	}
}

// A client that does not read: once its queue is full the next send closes it instead of waiting;
// the writer (stuck in a write) ends with the connection.
func TestFullQueueClosesClient(t *testing.T) {
	conn, peer := net.Pipe() // nobody reads peer: every write blocks
	defer peer.Close()
	c := NewClient(conn, nil)
	start := time.Now()
	for i := 0; i < SendQueueSize+10; i++ {
		c.SendPacket(CreatePacket(PacketTypeServer, "", "", strconv.Itoa(i), ""))
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("sending took %v", d)
	}
	if !c.Closed() {
		t.Fatal("slow client not closed")
	}
	if !stoppedWithin(c, 2*time.Second) {
		t.Fatal("writer did not end")
	}
}

// Sending after Close (or Finish) neither panics nor blocks, from many goroutines at once.
func TestSendAfterClose(t *testing.T) {
	c := NewClient(newFakeConn(), nil)
	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func() {
			for i := 0; i < SendQueueSize*2; i++ {
				c.SendPacket(CreatePacket(PacketTypeServer, "", "", "x", ""))
			}
			done <- struct{}{}
		}()
	}
	c.Close()
	c.Close()
	c.Finish()
	for g := 0; g < 8; g++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("send blocked")
		}
	}
	if !stoppedWithin(c, time.Second) {
		t.Fatal("writer did not end")
	}
}

// A failed write closes the client.
func TestWriteErrorCloses(t *testing.T) {
	f := newFakeConn()
	c := NewClient(f, nil)
	f.Close()
	c.SendPacket(CreatePacket(PacketTypeServer, "", "", "x", ""))
	if !stoppedWithin(c, time.Second) || !c.Closed() {
		t.Fatal("client not closed after a write error")
	}
}
