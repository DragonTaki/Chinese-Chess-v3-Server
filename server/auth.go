/* ----- ----- ----- ----- */
// auth.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/07
// Version: v1.3
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

// Authenticate runs the two-stage handshake (version, then username / password) on c, reading
// with scanner (the connection's only reader, shared with Client.Listen), then admits the account
// (login: the duplicate-login rule). Returns true on success; false on failure, refusal or when it
// does not finish within timeout.
func (s *Server) Authenticate(c *Client, scanner *bufio.Scanner, timeout time.Duration) bool {
	dbConn := s.dbConn
	// The verified account (ok false: the handshake failed); admitted by login back on the caller's
	// goroutine, so it runs before this connection's RemoveClient can.
	type verified struct {
		ok               bool
		uid, name, token string
	}
	authCh := make(chan verified, 1)

	go func() {
		// Stage 1: Version
		// Stage 2: Email (sent as AuthData.Username) and password
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

				authCh <- verified{ok: true, uid: uid, name: db.UserName(dbConn, uid), token: token}
				return
			}
		}
	}()

	select {
	case v := <-authCh:
		if !v.ok || !s.login(c, v.uid, v.name, v.token) {
			return false
		}
		logger.Infof("Client %s authenticated successfully", c.RemoteAddr)
		return true
	case <-time.After(timeout):
		logger.Warnf("Client %s failed to authenticate in time", c.RemoteAddr)
		return false
	}
}

// login admits c as the account uid whose credentials it just proved, under the duplicate-login
// rule (ONLINE-PLAY 8.14, 8.24): when another live connection of the account exists, c replaces it
// if both come from the same place (closeLogins) and is refused (AuthFailAlreadyLoggedIn)
// otherwise. c then takes over the account's seat (resume). A replaced connection is told so (AuthReplacedByNewLogin) and closed. An earlier
// connection that is already dead (closed, or gone from the server) is no obstacle: this is a
// reconnect. Logins are handled one at a time (loginMu), so two at once cannot both pass. Returns
// whether c was admitted (it was then sent the successful AuthResponse).
func (s *Server) login(c *Client, uid, name, token string) bool {
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

	// The client is known by its account's id from now on, never by the id it puts in its packets
	// (the client is untrusted).
	c.MarkAuthenticated(uid, name, token)
	c.SendPacket(CreatePacket(PacketTypeAuthResponse, "Server", "", AuthSuccessString, token))

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

// rejectAuth tells the client why its handshake failed (a failed AuthResponse, reason in Data).
func (s *Server) rejectAuth(c *Client, reason string) {
	c.SendPacket(CreatePacket(PacketTypeAuthResponse, "Server", "", reason, ""))
}
