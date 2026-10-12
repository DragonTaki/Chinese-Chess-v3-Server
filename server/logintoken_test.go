/* ----- ----- ----- ----- */
// logintoken_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/12
// Update Date: 2026/10/12
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"Chinese-Chess-v3-Server/server/db"
)

// tokenServer is a test server with the account a (a@test.com, password pw).
func tokenServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t)
	h, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	if err := s.dbConn.Create(&db.User{UID: "a", Email: "a@test.com", PasswordHash: string(h)}).Error; err != nil {
		t.Fatal(err)
	}
	return s
}

// handshake runs the whole handshake on a new connection from remote with the stage-2 AuthData
// creds; it returns the connection, whether it was admitted and the last AuthResponse.
func handshake(t *testing.T, s *Server, remote string, creds AuthData) (*Client, bool, *Packet) {
	t.Helper()
	c, f := connectFrom(s, remote)
	line := func(d any) string {
		b, _ := json.Marshal(d)
		p, _ := json.Marshal(CreatePacket(PacketTypeAuthRequest, "", "", string(b), ""))
		return string(p) + "\n"
	}
	in := line(AuthData{Version: ServerVersion}) + line(creds)
	ok := s.Authenticate(c, bufio.NewScanner(strings.NewReader(in)), 2*time.Second)
	flush(t, c, f)
	resp := f.packets(PacketTypeAuthResponse)
	if len(resp) == 0 {
		t.Fatal("no AuthResponse")
	}
	return c, ok, resp[len(resp)-1]
}

// logoff ends c as a disconnect does.
func logoff(s *Server, c *Client) {
	c.Close()
	s.RemoveClient(c)
}

// A password login asking to be remembered gets a login token (only its hash stored); one not
// asking gets none.
func TestRememberIssuesLoginToken(t *testing.T) {
	s := tokenServer(t)
	c, ok, p := handshake(t, s, "1.34.0.1:1", AuthData{Username: "a@test.com", Password: "pw"})
	if !ok || p.Data != AuthSuccessString || p.RefreshToken != "" {
		t.Fatalf("plain login %v %+v", ok, p)
	}
	logoff(s, c)
	c, ok, p = handshake(t, s, "1.34.0.1:2", AuthData{Username: "a@test.com", Password: "pw", Remember: true})
	if !ok || p.Data != AuthSuccessString || len(p.RefreshToken) != 43 || p.Token == "" {
		t.Fatalf("remembered login %v %+v", ok, p)
	}
	var rows []db.LoginToken
	s.dbConn.Find(&rows)
	if len(rows) != 1 || rows[0].UserID != "a" || rows[0].Hash == p.RefreshToken || rows[0].Revoked ||
		rows[0].ExpiresAt.Sub(rows[0].CreatedAt) != db.LoginTokenTTL {
		t.Fatalf("rows %+v", rows)
	}
	logoff(s, c)
}

// A login token logs in (a session token issued) and is rotated: the AuthResponse carries a new
// one, which logs in again.
func TestLoginTokenLoginRotates(t *testing.T) {
	s := tokenServer(t)
	c, _, p := handshake(t, s, "1.34.0.1:1", AuthData{Username: "a@test.com", Password: "pw", Remember: true})
	logoff(s, c)
	rt := p.RefreshToken
	c, ok, p := handshake(t, s, "8.8.8.8:1", AuthData{RefreshToken: rt})
	if !ok || p.Data != AuthSuccessString || p.Token == "" || p.RefreshToken == "" || p.RefreshToken == rt || c.Id() != "a" {
		t.Fatalf("token login %v %+v", ok, p)
	}
	logoff(s, c)
	c, ok, p2 := handshake(t, s, "8.8.8.8:2", AuthData{RefreshToken: p.RefreshToken})
	if !ok || p2.RefreshToken == "" || p2.RefreshToken == p.RefreshToken {
		t.Fatalf("second token login %v %+v", ok, p2)
	}
	logoff(s, c)
}

// A rotated login token used again is refused and revokes every token of its account (theft).
func TestRotatedLoginTokenReuseRevokesAll(t *testing.T) {
	s := tokenServer(t)
	c, _, p := handshake(t, s, "1.34.0.1:1", AuthData{Username: "a@test.com", Password: "pw", Remember: true})
	logoff(s, c)
	old := p.RefreshToken
	c, _, p = handshake(t, s, "1.34.0.1:2", AuthData{RefreshToken: old})
	logoff(s, c)
	c, ok, p2 := handshake(t, s, "1.34.0.1:3", AuthData{RefreshToken: old})
	if ok || p2.Data != AuthFailInvalidLoginToken || p2.RefreshToken != "" || c.Authenticated() {
		t.Fatalf("reuse %v %+v", ok, p2)
	}
	if _, ok, p3 := handshake(t, s, "1.34.0.1:4", AuthData{RefreshToken: p.RefreshToken}); ok || p3.Data != AuthFailInvalidLoginToken {
		t.Fatalf("newest token still valid: %+v", p3)
	}
}

// Expired, revoked (logout), unknown login tokens are refused; Logout revokes only a token of the
// logged-in account and is answered with a Logout.
func TestLoginTokenExpiredAndLogout(t *testing.T) {
	s := tokenServer(t)
	c, _, p := handshake(t, s, "1.34.0.1:1", AuthData{Username: "a@test.com", Password: "pw", Remember: true})
	expired := p.RefreshToken
	logoff(s, c)
	s.dbConn.Model(&db.LoginToken{}).Where("1 = 1").Update("expires_at", time.Now().Add(-time.Second))
	if _, ok, p := handshake(t, s, "1.34.0.1:2", AuthData{RefreshToken: expired}); ok || p.Data != AuthFailInvalidLoginToken {
		t.Fatalf("expired: %+v", p)
	}
	if _, ok, p := handshake(t, s, "1.34.0.1:3", AuthData{RefreshToken: "nope"}); ok || p.Data != AuthFailInvalidLoginToken {
		t.Fatalf("unknown: %+v", p)
	}

	c, _, p = handshake(t, s, "1.34.0.1:4", AuthData{Username: "a@test.com", Password: "pw", Remember: true})
	rt := p.RefreshToken
	other, _ := connectFrom(s, "1.34.0.2:1")
	other.MarkAuthenticated("b", "B", "")
	s.logout(other, &Packet{Type: PacketTypeLogout, Data: `{"refreshToken":"` + rt + `"}`})
	if live, _ := db.CheckLoginToken(s.dbConn, rt); live != "a" {
		t.Fatal("another account revoked the token")
	}
	f := c.Connection.(*fakeConn)
	s.logout(c, &Packet{Type: PacketTypeLogout, Data: `{"refreshToken":"` + rt + `"}`})
	flush(t, c, f)
	if len(f.packets(PacketTypeLogout)) != 1 {
		t.Fatal("no Logout reply")
	}
	logoff(s, c)
	if _, ok, p := handshake(t, s, "1.34.0.1:5", AuthData{RefreshToken: rt}); ok || p.Data != AuthFailInvalidLoginToken {
		t.Fatalf("logged-out token: %+v", p)
	}
}

// A token login refused by the duplicate-login rule does not spend the token.
func TestLoginTokenRefusedNotSpent(t *testing.T) {
	s := tokenServer(t)
	s.geo = fakeGeo
	_, _, p := handshake(t, s, "1.34.0.1:1", AuthData{Username: "a@test.com", Password: "pw", Remember: true})
	// The first connection stays live; a login from far away is refused.
	if _, ok, p2 := handshake(t, s, "8.8.8.8:1", AuthData{RefreshToken: p.RefreshToken}); ok || p2.Data != AuthFailAlreadyLoggedIn {
		t.Fatalf("far login %+v", p2)
	}
	if uid, err := db.CheckLoginToken(s.dbConn, p.RefreshToken); err != nil || uid != "a" {
		t.Fatalf("token spent: %v", err)
	}
}
