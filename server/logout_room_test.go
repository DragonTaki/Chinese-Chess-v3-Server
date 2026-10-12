/* ----- ----- ----- ----- */
// logout_room_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/12
// Update Date: 2026/10/12
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Chinese-Chess-v3-Server/server/db"
)

// A Logout in a waiting room leaves it at once (no seat grace, no away; the host passes on) and
// the disconnect after it changes nothing.
func TestLogoutInWaitingRoomLeavesAtOnce(t *testing.T) {
	s, r, a, b, _, fb := graceRoom(t, time.Hour)
	s.logout(a, &Packet{Type: PacketTypeLogout})
	if s.rooms.inRoom(a) {
		t.Fatal("still seated")
	}
	v := lastRoom(t, b, fb)
	if v.Seats[0] != nil || v.Seats[1] == nil || v.Seats[1].Away || v.HostId != "b" {
		t.Fatalf("room %+v %+v host %s", v.Seats[0], v.Seats[1], v.HostId)
	}
	a.Close()
	s.RemoveClient(a)
	if v, members := s.rooms.View(r); len(members) != 1 || v.Seats[1].Away || v.HostId != "b" {
		t.Fatalf("after disconnect %+v", v)
	}
}

// A Logout in a running game resigns it (EndGame Resign to all, the record written).
func TestLogoutInGameResigns(t *testing.T) {
	dbConn, err := db.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(dbConn, nil)
	g, a, b, _, cb := runningGame(t, s, TimerSettings{Mode: "CountUp", TotalMinutes: 10, StepSeconds: 60})
	s.logout(a, &Packet{Type: PacketTypeLogout})
	a.Close()
	s.RemoveClient(a)
	flush(t, b, cb)
	ends := cb.packets(PacketTypeEndGame)
	if len(ends) != 1 || !strings.Contains(ends[0].Data, `"Resign"`) || !g.over {
		t.Fatalf("EndGame %v", ends)
	}
	var rec db.Game
	if err := dbConn.First(&rec, "id = ?", "g").Error; err != nil || rec.Reason != "Resign" || rec.Winner != "b" {
		t.Fatalf("record %+v (%v)", rec, err)
	}
}

// A Logout outside any room just replies.
func TestLogoutOutsideRoomsUnchanged(t *testing.T) {
	s := NewServer(nil, nil)
	c, f := connectFrom(s, "1.34.0.9:1")
	s.login(c, "x", "X", "")
	s.logout(c, &Packet{Type: PacketTypeLogout})
	flush(t, c, f)
	if len(f.packets(PacketTypeLogout)) != 1 || len(f.packets(PacketTypeError)) != 0 {
		t.Fatalf("packets %v", f.packets(PacketTypeError))
	}
}
