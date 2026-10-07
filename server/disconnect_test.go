/* ----- ----- ----- ----- */
// disconnect_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"Chinese-Chess-v3-Server/server/db"
)

func TestReadTimeout(t *testing.T) {
	if readTimeout(true) != InGameTimeoutLimit || readTimeout(false) != ClientTimeoutLimit {
		t.Fatal("wrong read timeout")
	}
	if InGameHeartbeatInterval*3 > InGameTimeoutLimit {
		t.Fatal("the in-game heartbeat interval leaves too little margin")
	}
}

// SetInGame applies the in-game deadline to a read already waiting with the long one.
func TestSetInGameShortensWaitingRead(t *testing.T) {
	server, peer := net.Pipe()
	defer server.Close()
	defer peer.Close()
	c := NewClient(server, nil)
	c.refreshReadDeadline()
	errc := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := server.Read(make([]byte, 1))
		errc <- err
	}()
	time.Sleep(50 * time.Millisecond)
	c.SetInGame(true)
	err := <-errc
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read ended with %v", err)
	}
	if d := time.Since(start); d > InGameTimeoutLimit+500*time.Millisecond {
		t.Fatalf("read took %v", d)
	}
	// Back out of the game the long deadline applies again.
	c.SetInGame(false)
	go func() { peer.Write([]byte("x")) }()
	if _, err := server.Read(make([]byte, 1)); err != nil {
		t.Fatalf("read after the game: %v", err)
	}
}

// fakeConn records what is written to it; reads wait until it is closed.
type fakeConn struct {
	mu     sync.Mutex
	buf    strings.Builder
	closed chan struct{}
	once   sync.Once
}

func newFakeConn() *fakeConn { return &fakeConn{closed: make(chan struct{})} }

func (f *fakeConn) Read([]byte) (int, error) { <-f.closed; return 0, net.ErrClosed }
func (f *fakeConn) Write(b []byte) (int, error) {
	select {
	case <-f.closed:
		return 0, net.ErrClosed
	default:
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.Write(b)
}
func (f *fakeConn) Close() error                     { f.once.Do(func() { close(f.closed) }); return nil }
func (f *fakeConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (f *fakeConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (f *fakeConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// packets is every packet of type typ written to f so far.
func (f *fakeConn) packets(typ PacketType) []*Packet {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*Packet
	sc := bufio.NewScanner(strings.NewReader(f.buf.String()))
	for sc.Scan() {
		if p, err := DeserializePacket(sc.Text()); err == nil && p.Type == typ {
			out = append(out, p)
		}
	}
	return out
}

// runningGame seats accounts a (host) and b in a room of s and starts a game of timer between them
// (as startGame, without the rules host).
func runningGame(t *testing.T, s *Server, timer TimerSettings) (*game, *Client, *Client, *fakeConn, *fakeConn) {
	t.Helper()
	ca, cb := newFakeConn(), newFakeConn()
	a, b := NewClient(ca, s), NewClient(cb, s)
	a.MarkAuthenticated("a", "A", "")
	b.MarkAuthenticated("b", "B", "")
	settings := testSettings()
	settings.Timer = timer
	r, code, _ := s.rooms.Create(a, settings)
	if code != "" {
		t.Fatalf("create: %s", code)
	}
	if _, code := s.rooms.Join(b, r.Id); code != "" {
		t.Fatalf("join: %s", code)
	}
	s.rooms.SetReady(a, true)
	s.rooms.SetReady(b, true)
	players, ok := s.rooms.BeginGame(r)
	if !ok {
		t.Fatal("game did not begin")
	}
	now := time.Now()
	g := &game{id: "g", room: r, kind: "Traditional", players: players, position: standardPosition(),
		startedAt: now, clocks: newClockState(timer, len(players), now), done: make(chan struct{})}
	g.mu.Lock()
	s.rooms.SetGame(r, g)
	s.armDeadline(g, now)
	for _, p := range players {
		p.conn.SetInGame(true)
	}
	g.mu.Unlock()
	return g, a, b, ca, cb
}

// A disconnect during a running game keeps the seat: the side is away, the game goes on, the
// opponent gets a TimerSync showing it, and the Disconnect deadline is armed.
func TestDisconnectKeepsSeat(t *testing.T) {
	s := NewServer(nil, nil)
	g, a, b, ca, cb := runningGame(t, s, TimerSettings{Mode: "CountUp", TotalMinutes: 10, StepSeconds: 60})
	defer func() { g.mu.Lock(); g.stopDeadline(); g.mu.Unlock() }()
	ca.Close()
	s.RemoveClient(a)

	g.mu.Lock()
	over, away, deadline := g.over, g.clocks.away[0], g.deadline
	_, _, at, _ := g.clocks.expiry(time.Now())
	g.mu.Unlock()
	if over || away.IsZero() || deadline == nil {
		t.Fatalf("over %v, away %v, deadline %v", over, away, deadline)
	}
	if d := at.Sub(away); d != DisconnectGraceSeat {
		t.Fatalf("Disconnect after %v", d)
	}
	if s.rooms.GameOf(a) != g || s.rooms.GameOf(b) != g {
		t.Fatal("seat not kept")
	}
	syncs := cb.packets(PacketTypeTimerSync)
	if len(syncs) != 1 {
		t.Fatalf("%d TimerSync", len(syncs))
	}
	var cd ClocksData
	json.Unmarshal([]byte(syncs[0].Data), &cd)
	if len(cd.Away) != 2 || !cd.Away[0] || cd.Away[1] {
		t.Fatalf("away %v", cd.Away)
	}
	if len(cb.packets(PacketTypeEndGame)) != 0 {
		t.Fatal("game ended")
	}
}

// A disconnect outside a running game leaves the room, as LeaveRoom.
func TestDisconnectOutsideGameLeaves(t *testing.T) {
	s := NewServer(nil, nil)
	a := NewClient(newFakeConn(), s)
	b := NewClient(newFakeConn(), s)
	a.MarkAuthenticated("a", "A", "")
	b.MarkAuthenticated("b", "B", "")
	r, _, _ := s.rooms.Create(a, testSettings())
	s.rooms.Join(b, r.Id)
	s.RemoveClient(a)
	if s.rooms.HostOf(r) != "b" || len(s.rooms.Members(r)) != 1 {
		t.Fatal("disconnected host still in the room")
	}
}

// When the game ends while a player is away, that player leaves the room (the host passes on) and
// the end is recorded with its reason; the remaining player is back to the long read deadline.
func TestGameEndRemovesAwayPlayer(t *testing.T) {
	dbConn, err := db.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(dbConn, nil)
	g, a, b, ca, cb := runningGame(t, s, TimerSettings{Mode: "CountUp", TotalMinutes: 10, StepSeconds: 60})
	ca.Close()
	s.RemoveClient(a)

	g.mu.Lock()
	s.endGame(g, 2, ClockReasonDisconnect)
	g.mu.Unlock()

	r := g.room
	if s.rooms.HostOf(r) != "b" || len(s.rooms.Members(r)) != 1 || s.rooms.GameOf(b) != nil {
		t.Fatal("away player still seated")
	}
	if b.inGame {
		t.Fatal("remaining player still in game")
	}
	ends := cb.packets(PacketTypeEndGame)
	if len(ends) != 1 || !strings.Contains(ends[0].Data, ClockReasonDisconnect) {
		t.Fatalf("EndGame %v", ends)
	}
	var rec db.Game
	if err := dbConn.First(&rec, "id = ?", "g").Error; err != nil || rec.Reason != ClockReasonDisconnect || rec.Winner != "b" {
		t.Fatalf("record %+v (%v)", rec, err)
	}
}

// An explicit LeaveRoom during a game still resigns.
func TestLeaveDuringGameResigns(t *testing.T) {
	dbConn, err := db.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(dbConn, nil)
	g, a, _, _, cb := runningGame(t, s, TimerSettings{Mode: "CountUp", TotalMinutes: 10, StepSeconds: 60})
	s.leaveRoom(a, true)
	ends := cb.packets(PacketTypeEndGame)
	if len(ends) != 1 || !strings.Contains(ends[0].Data, `"Resign"`) || !g.over {
		t.Fatalf("EndGame %v", ends)
	}
}
