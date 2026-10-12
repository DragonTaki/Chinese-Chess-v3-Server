/* ----- ----- ----- ----- */
// roomgrace_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/12
// Update Date: 2026/10/12
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"encoding/json"
	"testing"
	"time"
)

// graceRoom seats a (host) and b in a waiting room of a server whose seat grace is grace; both
// are ready.
func graceRoom(t *testing.T, grace time.Duration) (*Server, *Room, *Client, *Client, *fakeConn, *fakeConn) {
	t.Helper()
	s := NewServer(nil, nil)
	s.seatGrace = grace
	a, fa := connectFrom(s, "1.34.0.1:1")
	b, fb := connectFrom(s, "1.34.0.2:1")
	s.login(a, "a", "A", "")
	s.login(b, "b", "B", "")
	r, _, _ := s.rooms.Create(a, testSettings())
	if _, code := s.rooms.Join(b, r.Id); code != "" {
		t.Fatalf("join: %s", code)
	}
	s.rooms.SetReady(a, true)
	s.rooms.SetReady(b, true)
	return s, r, a, b, fa, fb
}

// lastRoom is the last RoomState written to f.
func lastRoom(t *testing.T, c *Client, f *fakeConn) RoomView {
	t.Helper()
	flush(t, c, f)
	states := f.packets(PacketTypeRoomState)
	if len(states) == 0 {
		t.Fatal("no RoomState")
	}
	var v RoomView
	json.Unmarshal([]byte(states[len(states)-1].Data), &v)
	return v
}

// A disconnect in a waiting room keeps the seat, away with its ready cleared (the others' ready
// stays; the others are sent it); logging in again within the grace takes the seat back (no longer
// away, RoomState to everyone) and the grace running out later changes nothing.
func TestRoomGraceReconnectKeepsSeat(t *testing.T) {
	s, r, a, b, _, fb := graceRoom(t, 100*time.Millisecond)
	a.Close()
	s.RemoveClient(a)
	v := lastRoom(t, b, fb)
	if v.Seats[0] == nil || !v.Seats[0].Away || v.Seats[0].Ready || !v.Seats[1].Ready || v.Seats[1].Away || v.HostId != "a" {
		t.Fatalf("while away %+v %+v", v.Seats[0], v.Seats[1])
	}
	c, fc := connectFrom(s, "1.34.0.1:2")
	if !s.login(c, "a", "A", "") {
		t.Fatal("login refused")
	}
	if v := lastRoom(t, c, fc); v.Seats[0] == nil || v.Seats[0].Away {
		t.Fatalf("back %+v", v.Seats[0])
	}
	if v := lastRoom(t, b, fb); v.Seats[0].Away {
		t.Fatal("others still see away")
	}
	time.Sleep(200 * time.Millisecond)
	if v, members := s.rooms.View(r); len(members) != 2 || members[0] != c || v.HostId != "a" || v.Seats[0].Away {
		t.Fatalf("after the grace %+v", v)
	}
	if _, _, code := s.rooms.SetReady(c, true); code != "" {
		t.Fatalf("ready: %s", code)
	}
}

// When the grace runs out the player leaves: the seat is free, everyone's ready cleared, the
// others are sent the room.
func TestRoomGraceExpiryLeaves(t *testing.T) {
	s, r, a, b, fa, _ := graceRoom(t, 50*time.Millisecond)
	b.Close()
	s.RemoveClient(b)
	waitMembers(t, s, r, 1)
	v := lastRoom(t, a, fa)
	if v.Seats[1] != nil || v.Seats[0].Ready || v.HostId != "a" || s.rooms.inRoom(b) {
		t.Fatalf("after the grace %+v", v)
	}
	// b can join a room again.
	c, _ := connectFrom(s, "1.34.0.2:2")
	s.login(c, "b", "B", "")
	if _, code := s.rooms.Join(c, r.Id); code != "" {
		t.Fatalf("rejoin: %s", code)
	}
}

// The host away until the grace runs out: the host passes to the next player; a room whose only
// player goes is removed.
func TestRoomGraceHostHandover(t *testing.T) {
	s, r, a, b, _, fb := graceRoom(t, 50*time.Millisecond)
	a.Close()
	s.RemoveClient(a)
	if s.rooms.HostOf(r) != "a" {
		t.Fatal("host passed during the grace")
	}
	waitMembers(t, s, r, 1)
	if v := lastRoom(t, b, fb); v.HostId != "b" || v.Seats[0] != nil {
		t.Fatalf("after the grace %+v", v)
	}
	b.Close()
	s.RemoveClient(b)
	for end := time.Now().Add(2 * time.Second); len(s.rooms.List()) != 0; time.Sleep(time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("empty room not removed")
		}
	}
}

// A room with an away seat cannot begin a game, even were every seat marked ready.
func TestRoomGraceBlocksBegin(t *testing.T) {
	s, r, a, b, _, _ := graceRoom(t, time.Minute)
	b.Close()
	s.RemoveClient(b)
	if _, all, _ := s.rooms.SetReady(a, true); all {
		t.Fatal("all ready with a player away")
	}
	s.rooms.mu.Lock()
	r.seats[1].ready = true
	s.rooms.mu.Unlock()
	if _, ok := s.rooms.BeginGame(r); ok {
		t.Fatal("game began with a player away")
	}
	s.rooms.mu.Lock()
	r.seats[1].stopGrace()
	s.rooms.mu.Unlock()
}
