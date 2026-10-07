/* ----- ----- ----- ----- */
// resume_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

var countUpTimer = TimerSettings{Mode: "CountUp", TotalMinutes: 10, StepSeconds: 60}

// lastClocks is the clocks of the last TimerSync written to f.
func lastClocks(t *testing.T, f *fakeConn) ClocksData {
	t.Helper()
	syncs := f.packets(PacketTypeTimerSync)
	if len(syncs) == 0 {
		t.Fatal("no TimerSync")
	}
	var cd ClocksData
	json.Unmarshal([]byte(syncs[len(syncs)-1].Data), &cd)
	return cd
}

// A player away from a running game logs in again: its seat and player are the new connection,
// it is back (no away, no Disconnect deadline), it gets StartGame resumed with the moves so far and
// the clocks, and the opponent a TimerSync with away cleared.
func TestResumeGame(t *testing.T) {
	s := testServer(t)
	g, a, b, ca, cb := runningGame(t, s, countUpTimer)
	g.mu.Lock()
	g.commitMove([2]int{7, 7}, [2]int{4, 7}, "炮二平五", time.Now())
	g.mu.Unlock()
	ca.Close()
	s.RemoveClient(a)

	c, fc := connectFrom(s, "1.34.0.1:1")
	if !s.login(c, "a", "A", "") {
		t.Fatal("login refused")
	}
	if s.rooms.GameOf(c) != g || s.rooms.GameOf(a) != nil {
		t.Fatal("seat not moved")
	}
	g.mu.Lock()
	away, deadline, conn := g.clocks.away[0], g.deadline, g.players[0].conn
	g.mu.Unlock()
	if !away.IsZero() || deadline != nil || conn != c || !c.inGame {
		t.Fatalf("away %v, deadline %v, conn ok %v, in game %v", away, deadline, conn == c, c.inGame)
	}
	flush(t, c, fc)
	starts := fc.packets(PacketTypeStartGame)
	if len(starts) != 1 {
		t.Fatalf("%d StartGame", len(starts))
	}
	var sg StartGameData
	json.Unmarshal([]byte(starts[0].Data), &sg)
	if !sg.Resumed || sg.Ply != 1 || len(sg.Moves) != 1 || sg.Moves[0].Notation != "炮二平五" || sg.YourSide != 1 ||
		sg.GameId != "g" || sg.Clocks == nil || sg.Clocks.ToMove != 1 || sg.Clocks.Away[0] || sg.Position.ToMove != 1 {
		t.Fatalf("StartGame %+v", sg)
	}
	if len(fc.packets(PacketTypeRoomState)) != 1 {
		t.Fatal("no RoomState")
	}
	flush(t, b, cb)
	if cd := lastClocks(t, cb); cd.Away[0] || cd.Away[1] {
		t.Fatalf("opponent sees away %v", cd.Away)
	}
	if rid, _ := c.HeartbeatInfo(); rid != g.room.Id {
		t.Fatal("room id not set")
	}
}

// A live connection in a game replaced by a new login from the same IP: the new one resumes; the
// old one's disconnect, handled afterwards, neither marks the seat away nor leaves; its packets
// act for nothing.
func TestResumeReplacesLiveInGame(t *testing.T) {
	s := testServer(t)
	g, a, _, ca, _ := runningGame(t, s, countUpTimer)
	a.RemoteAddr = "1.34.0.1:1"
	s.mu.Lock()
	s.clients[a] = true
	s.mu.Unlock()
	// The old connection's disconnect has found the game but waits for its lock (as disconnect does).
	gOld := s.rooms.GameOf(a)

	c, _ := connectFrom(s, "1.34.0.1:2")
	if !s.login(c, "a", "A", "") {
		t.Fatal("login refused")
	}
	if !stoppedWithin(a, 2*time.Second) {
		t.Fatal("old connection not closed")
	}
	if got := authData(ca); len(got) != 1 || got[0] != AuthReplacedByNewLogin {
		t.Fatalf("old got %v", got)
	}
	gOld.mu.Lock()
	marked := s.setAway(gOld, a, time.Now())
	gOld.mu.Unlock()
	s.RemoveClient(a)
	s.handleGameAction(a, &Packet{Type: PacketTypeGameAction, Data: `{"seq":1,"type":"resign"}`})
	s.leaveRoom(a, false)

	g.mu.Lock()
	away, over := g.clocks.away[0], g.over
	g.mu.Unlock()
	if marked || !away.IsZero() || over || s.rooms.GameOf(c) != g {
		t.Fatalf("marked %v, away %v, over %v", marked, away, over)
	}
}

// Outside a game the new login takes over the seat in the waiting room (RoomState to everyone);
// the old connection's disconnect afterwards does not leave it.
func TestResumeWaitingRoom(t *testing.T) {
	s := testServer(t)
	a, fa := connectFrom(s, "1.34.0.1:1")
	b, fb := connectFrom(s, "1.34.0.2:1")
	s.login(a, "a", "A", "")
	s.login(b, "b", "B", "")
	r, _, _ := s.rooms.Create(a, testSettings())
	s.rooms.Join(b, r.Id)
	s.rooms.SetReady(a, true)

	c, fc := connectFrom(s, "1.34.0.1:2")
	if !s.login(c, "a", "A", "") {
		t.Fatal("login refused")
	}
	stoppedWithin(a, 2*time.Second)
	s.RemoveClient(a)
	view, members := s.rooms.View(r)
	if len(members) != 2 || members[0] != c || view.HostId != "a" || !view.Seats[0].Ready {
		t.Fatalf("room %+v", view)
	}
	flush(t, c, fc)
	flush(t, b, fb)
	if len(fc.packets(PacketTypeRoomState)) != 1 || len(fb.packets(PacketTypeRoomState)) != 1 {
		t.Fatal("RoomState not sent")
	}
	if len(authData(fa)) != 2 {
		t.Fatal("old connection not told")
	}
	// The new connection acts for the seat.
	if _, _, code := s.rooms.SetReady(c, false); code != "" {
		t.Fatalf("ready: %s", code)
	}
}

// A game that ended while the player was away is not resumed: the player is out of the room.
func TestNoResumeAfterGameEnd(t *testing.T) {
	s := testServer(t)
	g, a, _, ca, _ := runningGame(t, s, countUpTimer)
	ca.Close()
	s.RemoveClient(a)
	g.mu.Lock()
	s.endGame(g, 2, ClockReasonDisconnect)
	g.mu.Unlock()
	c, fc := connectFrom(s, "1.34.0.1:1")
	s.login(c, "a", "A", "")
	flush(t, c, fc)
	if len(fc.packets(PacketTypeStartGame)) != 0 || len(fc.packets(PacketTypeRoomState)) != 0 || s.rooms.inRoom(c) {
		t.Fatal("resumed an ended game")
	}
}

// A login arriving when the away player's clock has just run out (its deadline not handled yet)
// ends the game by the clocks instead of resuming it.
func TestNoResumeWhenClockRanOut(t *testing.T) {
	s := testServer(t)
	g, a, b, ca, cb := runningGame(t, s, TimerSettings{Mode: "CountDown", TotalMinutes: 10, StepSeconds: 1, StepTimer: true})
	ca.Close()
	s.RemoveClient(a)
	g.mu.Lock()
	g.stopDeadline() // as if the deadline callback were late
	g.mu.Unlock()
	time.Sleep(1100 * time.Millisecond)
	c, fc := connectFrom(s, "1.34.0.1:1")
	s.login(c, "a", "A", "")
	flush(t, c, fc)
	flush(t, b, cb)
	ends := cb.packets(PacketTypeEndGame)
	if len(fc.packets(PacketTypeStartGame)) != 0 || len(ends) != 1 || s.rooms.inRoom(c) {
		t.Fatalf("StartGame %d, EndGame %v", len(fc.packets(PacketTypeStartGame)), ends)
	}
	var e EndGameData
	json.Unmarshal([]byte(ends[0].Data), &e)
	if e.Winner != 2 || e.Reason != ClockReasonTimeUp {
		t.Fatalf("end %+v", e)
	}
}

// The game's deadline firing at the same time as the login: whichever takes the game's lock first
// wins, never both (no resume of an ended game, no end of a resumed one by the old deadline).
func TestResumeRacesDeadline(t *testing.T) {
	for i := 0; i < 20; i++ {
		s := testServer(t)
		g, a, _, ca, _ := runningGame(t, s, countUpTimer)
		ca.Close()
		s.RemoveClient(a)
		g.mu.Lock()
		gen := g.gen
		g.clocks.away[0] = time.Now().Add(-DisconnectGraceSeat) // the grace is just over
		g.mu.Unlock()
		c, fc := connectFrom(s, "1.34.0.1:1")
		done := make(chan struct{})
		go func() { s.onDeadline(g, gen); close(done) }()
		s.login(c, "a", "A", "")
		<-done
		flush(t, c, fc)
		g.mu.Lock()
		over := g.over
		g.mu.Unlock()
		resumed := len(fc.packets(PacketTypeStartGame)) == 1
		if over == resumed {
			t.Fatalf("over %v, resumed %v", over, resumed)
		}
		if !over {
			g.mu.Lock()
			s.endGame(g, 1, "Resign")
			g.mu.Unlock()
		}
	}
}

// A player away when the game ends (here its step clock runs out) gets the EndGame, with the game's
// id, at its next login, once; the opponent's EndGame carries the id too.
func TestMissedResultDeliveredOnce(t *testing.T) {
	s := testServer(t)
	g, a, b, ca, cb := runningGame(t, s, TimerSettings{Mode: "CountDown", TotalMinutes: 10, StepSeconds: 1, StepTimer: true})
	ca.Close()
	s.RemoveClient(a)
	select {
	case <-g.done:
	case <-time.After(3 * time.Second):
		t.Fatal("game did not end")
	}
	flush(t, b, cb)
	if ends := cb.packets(PacketTypeEndGame); len(ends) != 1 || !strings.Contains(ends[0].Data, `"gameId":"g"`) {
		t.Fatalf("opponent EndGame %v", ends)
	}

	c, fc := connectFrom(s, "1.34.0.1:1")
	s.login(c, "a", "A", "")
	flush(t, c, fc)
	ends := fc.packets(PacketTypeEndGame)
	if len(ends) != 1 {
		t.Fatalf("EndGame %v", ends)
	}
	var e EndGameData
	json.Unmarshal([]byte(ends[0].Data), &e)
	if e.Winner != 2 || e.Reason != ClockReasonTimeUp || e.GameId != "g" {
		t.Fatalf("end %+v", e)
	}
	if auth := fc.packets(PacketTypeAuthResponse); len(auth) != 1 {
		t.Fatal("no AuthResponse")
	}

	c.Close()
	s.RemoveClient(c)
	c2, fc2 := connectFrom(s, "1.34.0.1:2")
	s.login(c2, "a", "A", "")
	flush(t, c2, fc2)
	if len(fc2.packets(PacketTypeEndGame)) != 0 {
		t.Fatal("result delivered twice")
	}
}

// Pending results expire and the store is bounded.
func TestPendingResultsBounds(t *testing.T) {
	p := newPendingResults()
	now := time.Now()
	p.put("old", "r", EndGameData{}, now.Add(-pendingResultTTL-time.Second))
	if _, ok := p.take("old", now); ok {
		t.Fatal("expired result delivered")
	}
	for i := 0; i < pendingResultMax+10; i++ {
		p.put(fmt.Sprint("u", i), "r", EndGameData{}, now.Add(time.Duration(i)*time.Millisecond))
	}
	if len(p.list) != pendingResultMax {
		t.Fatalf("size %d", len(p.list))
	}
	if _, ok := p.take("u0", now); ok {
		t.Fatal("oldest not evicted")
	}
}
