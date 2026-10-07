/* ----- ----- ----- ----- */
// login_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"testing"
	"time"
)

// connectFrom makes a connection of s from the remote address remote, registered as
// HandleNewClient does (not logged in yet).
func connectFrom(s *Server, remote string) (*Client, *fakeConn) {
	f := newFakeConn()
	c := NewClient(f, s)
	c.RemoteAddr = remote
	s.mu.Lock()
	s.clients[c] = true
	s.mu.Unlock()
	return c, f
}

// authData is the Data of every AuthResponse written to f.
func authData(f *fakeConn) []string {
	var out []string
	for _, p := range f.packets(PacketTypeAuthResponse) {
		out = append(out, p.Data)
	}
	return out
}

// A second login of an account from the same IP replaces the first: the old connection is told
// (ReplacedByNewLogin) and closed, the new one admitted.
func TestLoginReplacesSameIP(t *testing.T) {
	s := NewServer(nil, nil)
	old, fo := connectFrom(s, "1.34.0.1:5000")
	if !s.login(old, "a", "A", "t1") {
		t.Fatal("first login refused")
	}
	c, fc := connectFrom(s, "1.34.0.1:6000")
	if !s.login(c, "a", "A", "t2") {
		t.Fatal("second login refused")
	}
	if !stoppedWithin(old, 2*time.Second) || !old.Closed() {
		t.Fatal("old connection not closed")
	}
	if got := authData(fo); len(got) != 2 || got[0] != AuthSuccessString || got[1] != AuthReplacedByNewLogin {
		t.Fatalf("old got %v", got)
	}
	flush(t, c, fc)
	if got := authData(fc); len(got) != 1 || got[0] != AuthSuccessString || c.Id() != "a" {
		t.Fatalf("new got %v", got)
	}
}

// The same city (by the geolocation database) also replaces.
func TestLoginReplacesSameCity(t *testing.T) {
	s := NewServer(nil, nil)
	s.geo = fakeGeo
	old, _ := connectFrom(s, "1.34.0.1:5000")
	s.login(old, "a", "A", "")
	c, _ := connectFrom(s, "61.216.0.1:6000")
	if !s.login(c, "a", "A", "") || !stoppedWithin(old, 2*time.Second) {
		t.Fatal("not replaced")
	}
}

// From too far away the new login is refused (AlreadyLoggedIn) and the old connection untouched;
// without a database a different public IP is too far as well.
func TestLoginRefusedFarAway(t *testing.T) {
	for _, geo := range []geoLookup{fakeGeo, nil} {
		s := NewServer(nil, nil)
		s.geo = geo
		old, fo := connectFrom(s, "1.34.0.1:5000")
		s.login(old, "a", "A", "")
		far := "8.8.8.8:6000"
		if geo == nil {
			far = "61.216.0.1:6000" // the same city, unknown without the database
		}
		c, fc := connectFrom(s, far)
		if s.login(c, "a", "A", "") {
			t.Fatal("far login admitted")
		}
		flush(t, c, fc)
		if got := authData(fc); len(got) != 1 || got[0] != AuthFailAlreadyLoggedIn || c.Authenticated() {
			t.Fatalf("new got %v", got)
		}
		flush(t, old, fo)
		if old.Closed() || len(authData(fo)) != 1 {
			t.Fatal("old connection touched")
		}
	}
}

// Another account's connection is no obstacle.
func TestLoginOtherAccount(t *testing.T) {
	s := NewServer(nil, nil)
	s.geo = fakeGeo
	b, _ := connectFrom(s, "1.34.0.1:5000")
	s.login(b, "b", "B", "")
	c, _ := connectFrom(s, "8.8.8.8:6000")
	if !s.login(c, "a", "A", "") || b.Closed() {
		t.Fatal("other account affected")
	}
}

// An earlier connection that is already dead (closed but not yet removed, or removed) makes the
// new login a reconnect: admitted from anywhere, nothing sent to the old one.
func TestLoginAfterDeadConnection(t *testing.T) {
	for _, removed := range []bool{false, true} {
		s := NewServer(nil, nil)
		s.geo = fakeGeo
		old, fo := connectFrom(s, "1.34.0.1:5000")
		s.login(old, "a", "A", "")
		flush(t, old, fo)
		old.Close()
		if removed {
			s.RemoveClient(old)
		}
		c, _ := connectFrom(s, "8.8.8.8:6000")
		if !s.login(c, "a", "A", "") {
			t.Fatalf("reconnect refused (removed %v)", removed)
		}
		if got := authData(fo); len(got) != 1 {
			t.Fatalf("old got %v", got)
		}
	}
}
