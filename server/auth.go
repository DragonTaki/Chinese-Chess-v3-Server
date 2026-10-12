/* ----- ----- ----- ----- */
// auth.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/12
// Version: v1.4
/* ----- ----- ----- ----- */

package server

import (
	"bufio"
	"encoding/json"
	"time"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server/db"
)

// AuthMessage is not used (the handshake reads AuthData through Packet.ParseAuthData).
type AuthMessage struct {
	Type     string `json:"type"`
	SenderId string `json:"id"`
	Version  string `json:"version"`
}

// verified is the account a handshake proved (ok false: the handshake failed); admitted by
// loginWith back on Authenticate's goroutine, so it runs before the connection's RemoveClient can.
type verified struct {
	ok               bool
	uid, name, token string
	refresh          func() (string, bool) // the login token to send (see loginWith); nil: none
}

// Authenticate runs the two-stage handshake (version, then username / password or a login token)
// on c, reading with scanner (the connection's only reader, shared with Client.Listen), then admits
// the account (loginWith: the duplicate-login rule). Returns true on success; false on failure, refusal or when it
// does not finish within timeout.
func (s *Server) Authenticate(c *Client, scanner *bufio.Scanner, timeout time.Duration) bool {
	dbConn := s.dbConn
	authCh := make(chan verified, 1)

	go func() {
		// Stage 1: Version
		// Stage 2: Email (sent as AuthData.Username) and password, or a login token
		stage := 1

		for scanner.Scan() {
			line := scanner.Text()
			pkt, err := DeserializePacket(line)
			if err != nil {
				logger.Warnf("Invalid packet from %s: %v", c.RemoteAddr, err)
				continue
			}

			if pkt.Type != PacketTypeAuthRequest {
				logger.Warnf("Unexpected packet type from %s: %s", c.RemoteAddr, pkt.Type)
				continue
			}

			ad, err := pkt.ParseAuthData()
			if err != nil {
				logger.Warnf("Failed to parse auth data from %s: %v", c.RemoteAddr, err)
				continue
			}

			switch stage {
			case 1:
				if ad.Version != ServerVersion {
					logger.Warnf("Version mismatch from %s: %s != %s", c.RemoteAddr, ad.Version, ServerVersion)
					s.rejectAuth(c, AuthFailVersionMismatch)
					authCh <- verified{}
					return
				}

				// Request email and password for stage 2
				respPkt := CreatePacket(PacketTypeAuthRequest, "Server", "", "Please provide username/password", "")
				c.SendPacket(respPkt)
				stage = 2

			case 2:
				if ad.RefreshToken != "" {
					authCh <- s.verifyLoginToken(c, ad.RefreshToken)
					return
				}
				if ad.Username == "" || ad.Password == "" {
					logger.Warnf("Missing username/password from %s", c.RemoteAddr)
					s.rejectAuth(c, AuthFailMissingCredentials)
					authCh <- verified{}
					return
				}

				uid, token, ok := db.VerifyUser(dbConn, ad.Username, ad.Password)
				if !ok {
					logger.Warnf("Invalid credentials from %s", c.RemoteAddr)
					s.rejectAuth(c, AuthFailInvalidCredentials)
					authCh <- verified{}
					return
				}

				v := verified{ok: true, uid: uid, name: db.UserName(dbConn, uid), token: token}
				if ad.Remember {
					v.refresh = func() (string, bool) {
						rt, err := db.IssueLoginToken(dbConn, uid)
						if err != nil {
							// The login still succeeds, only not remembered.
							logger.Warnf("Could not issue a login token for %s: %v", uid, err)
							return "", true
						}
						return rt, true
					}
				}
				authCh <- v
				return
			}
		}
	}()

	select {
	case v := <-authCh:
		if !v.ok || !s.loginWith(c, v.uid, v.name, v.token, v.refresh) {
			return false
		}
		logger.Infof("Client %s authenticated successfully", c.RemoteAddr)
		return true
	case <-time.After(timeout):
		logger.Warnf("Client %s failed to authenticate in time", c.RemoteAddr)
		return false
	}
}

// verifyLoginToken checks a login token sent instead of the email and password (ONLINE-PLAY 8.25):
// a live one logs its account in (a session token issued as for a password login) and is rotated
// when the login is admitted; any other is refused (AuthFailInvalidLoginToken).
func (s *Server) verifyLoginToken(c *Client, refreshToken string) verified {
	uid, err := db.CheckLoginToken(s.dbConn, refreshToken)
	if err != nil {
		if err != db.ErrInvalidLoginToken {
			logger.Warnf("Could not check the login token from %s: %v", c.RemoteAddr, err)
		}
		logger.Warnf("Invalid login token from %s", c.RemoteAddr)
		s.rejectAuth(c, AuthFailInvalidLoginToken)
		return verified{}
	}
	token, ok := db.IssueSession(s.dbConn, uid)
	if !ok {
		s.rejectAuth(c, AuthFailInvalidLoginToken)
		return verified{}
	}
	refresh := func() (string, bool) {
		rt, err := db.RotateLoginToken(s.dbConn, uid, refreshToken)
		if err != nil {
			logger.Warnf("Login token from %s could not be rotated: %v", c.RemoteAddr, err)
			return "", false
		}
		return rt, true
	}
	return verified{ok: true, uid: uid, name: db.UserName(s.dbConn, uid), token: token, refresh: refresh}
}

// login is loginWith without a login token to send.
func (s *Server) login(c *Client, uid, name, token string) bool {
	return s.loginWith(c, uid, name, token, nil)
}

// loginWith admits c as the account uid whose credentials it just proved, under the duplicate-login
// rule (ONLINE-PLAY 8.14, 8.24): when another live connection of the account exists, c replaces it
// if both come from the same place (closeLogins) and is refused (AuthFailAlreadyLoggedIn)
// otherwise. c then takes over the account's seat (resume). A replaced connection is told so (AuthReplacedByNewLogin) and closed. An earlier
// connection that is already dead (closed, or gone from the server) is no obstacle: this is a
// reconnect. Logins are handled one at a time (loginMu), so two at once cannot both pass. Returns
// whether c was admitted (it was then sent the successful AuthResponse). Once the rule lets c in,
// refresh (unless nil) gives the login token the AuthResponse carries (RefreshToken; "" none); its
// false refuses the login after all (AuthFailInvalidLoginToken: the token used was spent meanwhile).
func (s *Server) loginWith(c *Client, uid, name, token string, refresh func() (string, bool)) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	others := s.liveSessions(c, uid)
	for _, o := range others {
		if !closeLogins(o.RemoteAddr, c.RemoteAddr, s.geo) {
			logger.Warnf("Login of %s from %s refused: already logged in from %s", uid, c.RemoteAddr, o.RemoteAddr)
			s.rejectAuth(c, AuthFailAlreadyLoggedIn)
			return false
		}
	}

	resp := CreatePacket(PacketTypeAuthResponse, "Server", "", AuthSuccessString, token)
	if refresh != nil {
		rt, ok := refresh()
		if !ok {
			s.rejectAuth(c, AuthFailInvalidLoginToken)
			return false
		}
		resp.RefreshToken = rt
	}

	// The client is known by its account's id from now on, never by the id it puts in its packets
	// (the client is untrusted).
	c.MarkAuthenticated(uid, name, token)
	c.SendPacket(resp)

	// The account's seat (a game or a waiting room) moves to c before any replaced connection is
	// closed, so that connection's disconnect finds no seat to leave or mark away.
	s.resume(c)

	// The result of a game that ended while this account was away, once.
	if r, ok := s.pending.take(uid, time.Now()); ok {
		data, _ := json.Marshal(r.data)
		c.SendPacket(CreatePacket(PacketTypeEndGame, "Server", r.roomId, string(data), ""))
	}

	for _, o := range others {
		logger.Infof("Connection %s of %s replaced by a new login from %s", o.RemoteAddr, uid, c.RemoteAddr)
		o.SendPacket(CreatePacket(PacketTypeAuthResponse, "Server", "", AuthReplacedByNewLogin, ""))
		o.Finish()
	}
	return true
}

// liveSessions is every other connection of the account uid that is still open (normally at most
// one).
func (s *Server) liveSessions(c *Client, uid string) []*Client {
	var list []*Client
	for _, o := range s.snapshotClients() {
		if o != c && o.Id() == uid && !o.Closed() {
			list = append(list, o)
		}
	}
	return list
}

// logout handles a Logout packet: the login token in its Data ({"refreshToken"}; may be absent)
// is revoked when it is a live one of c's account, c is sent a Logout back and its connection ends
// (the caller stops reading; the disconnect is handled as any other).
func (s *Server) logout(c *Client, pkt *Packet) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if pkt.Data != "" {
		if err := json.Unmarshal([]byte(pkt.Data), &req); err != nil {
			logger.Warnf("Invalid Logout data from %s", c.RemoteAddr)
		}
	}
	if req.RefreshToken != "" && s.dbConn != nil {
		if _, err := db.RevokeLoginToken(s.dbConn, c.Id(), req.RefreshToken); err != nil {
			logger.Warnf("Could not revoke the login token of %s: %v", c.Id(), err)
		}
	}
	c.SendPacket(CreatePacket(PacketTypeLogout, "Server", "", "", ""))
	logger.Infof("Client %s (%s) logged out", c.RemoteAddr, c.Id())
}

// rejectAuth tells the client why its handshake failed (a failed AuthResponse, reason in Data).
func (s *Server) rejectAuth(c *Client, reason string) {
	c.SendPacket(CreatePacket(PacketTypeAuthResponse, "Server", "", reason, ""))
}
