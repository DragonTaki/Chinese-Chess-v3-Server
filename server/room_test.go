/* ----- ----- ----- ----- */
// room_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import "testing"

func testSettings() RoomSettings {
	return RoomSettings{Kind: "Traditional", Mode: "Custom", FirstMover: "Host",
		Timer: TimerSettings{Mode: "CountUp", TotalMinutes: 10, StepSeconds: 60}}
}

// A second connection of an account that holds a seat is in no room: it cannot take another seat,
// and leaving, readying or a game lookup through it do not touch the first connection's seat.
func TestSameAccountSecondConnection(t *testing.T) {
	m := NewRoomManager()
	first := &Client{SenderId: "a", IsAuthenticated: true}
	second := &Client{SenderId: "a", IsAuthenticated: true}
	r, code, _ := m.Create(first, testSettings())
	if code != "" {
		t.Fatalf("create: %s", code)
	}
	if _, code, _ := m.Create(second, testSettings()); code != RoomErrAlreadyInRoom {
		t.Fatalf("second create: %q", code)
	}
	if _, code := m.Join(second, r.Id); code != RoomErrAlreadyInRoom {
		t.Fatalf("second join: %q", code)
	}
	if _, _, code := m.SetReady(second, true); code != RoomErrNotInRoom {
		t.Fatalf("second ready: %q", code)
	}
	if left, _ := m.Leave(second); left != nil {
		t.Fatal("second connection left the first's room")
	}
	if g := m.GameOf(second); g != nil {
		t.Fatal("second connection sees a game")
	}
	if left, _ := m.Leave(first); left != r {
		t.Fatal("first connection could not leave")
	}
	if len(m.rooms) != 0 || len(m.byUser) != 0 {
		t.Fatal("room not removed")
	}
}

// An unauthenticated client (no account id) can take no seat.
func TestUnauthenticatedNoRoom(t *testing.T) {
	m := NewRoomManager()
	if _, code, _ := m.Create(&Client{}, testSettings()); code != RoomErrAlreadyInRoom {
		t.Fatalf("create: %q", code)
	}
}

// Leaving hands the host to the next seated account.
func TestHostHandover(t *testing.T) {
	m := NewRoomManager()
	a := &Client{SenderId: "a"}
	b := &Client{SenderId: "b"}
	r, _, _ := m.Create(a, testSettings())
	if _, code := m.Join(b, r.Id); code != "" {
		t.Fatalf("join: %s", code)
	}
	m.Leave(a)
	if m.HostOf(r) != "b" {
		t.Fatalf("host %q", m.HostOf(r))
	}
}
