/* ----- ----- ----- ----- */
// client.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/06
// Version: v1.2
/* ----- ----- ----- ----- */

package server

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server/db"
)

// Client represents a connected client. LastSeenAt, IsAuthenticated, SenderId, Token and RoomId
// are written by the client's own goroutine and read by the heartbeat goroutines: access them
// through the methods below, which hold mu.
type Client struct {
	Connection      net.Conn
	RemoteAddr      string
	Server          *Server
	LastSeenAt      time.Time
	IsAuthenticated bool
	SenderId        string // The account's user id, set when authenticated
	DisplayName     string // The account's name (Username, else Email), set when authenticated
	Token           string
	RoomId          string

	mu     sync.Mutex
	sendMu sync.Mutex // one packet at a time on the connection (room updates come from other goroutines)

	// When the token's last-seen time was last written (only the client's own goroutine uses it).
	tokenSeenWrittenAt time.Time
}

// Touch records that the client was just heard from (LastSeenAt = now).
func (c *Client) Touch() {
	c.mu.Lock()
	c.LastSeenAt = time.Now()
	c.mu.Unlock()
}

// LastSeen returns when the client was last heard from.
func (c *Client) LastSeen() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.LastSeenAt
}

// MarkAuthenticated records a successful handshake: the client's id, display name and token, and now as last seen.
func (c *Client) MarkAuthenticated(senderId, name, token string) {
	c.mu.Lock()
	c.IsAuthenticated = true
	c.SenderId = senderId
	c.DisplayName = name
	c.Token = token
	c.LastSeenAt = time.Now()
	c.mu.Unlock()
}

// Authenticated reports whether the client has passed the handshake.
func (c *Client) Authenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.IsAuthenticated
}

// Id returns the client's account id (empty before the handshake).
func (c *Client) Id() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.SenderId
}

// HeartbeatInfo returns the room id and token a heartbeat packet to the client carries.
func (c *Client) HeartbeatInfo() (roomId, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.RoomId, c.Token
}

// NewClient wraps a new connection of srv.
func NewClient(conn net.Conn, srv *Server) *Client {
	return &Client{
		Connection: conn,
		RemoteAddr: conn.RemoteAddr().String(),
		Server:     srv,
	}
}

// Listen serves the connection: the handshake, a welcome message, then one JSON packet per line
// until the connection ends or the client sends "/quit". Only chat packets are handled so far
// (broadcast to the other clients); the room / game packet types are not implemented.
func (c *Client) Listen() {
	defer func() {
		c.Connection.Close()
		c.Server.RemoveClient(c)
	}()

	// One reader for the whole connection: the auth stage and the chat loop below share it, so
	// nothing the client sends right after authenticating is lost in a second reader's buffer.
	scanner := bufio.NewScanner(c.Connection)
	scanner.Buffer(make([]byte, 0, 64*1024), MaxPacketSize)

	// Auth session
	if ok := c.Server.Authenticate(c, scanner, AuthTimeoutLimit); !ok { // If auth fail
		return
	}

	// Send JSON welcome message using CreatePacket
	welcomePkt := CreatePacket(PacketTypeServer, "Server", "", "Welcome to Go-Chess-Server! Type message to chat.", "")
	c.SendPacket(welcomePkt)

	for scanner.Scan() {
		line := scanner.Text()
		c.Touch()

		if line == "/quit" {
			break
		}

		// Deserialize client packet
		pkt, err := DeserializePacket(line)
		if err != nil {
			errPkt := CreatePacket(PacketTypeError, "Server", "", fmt.Sprintf("Invalid JSON: %v", err), "")
			c.SendPacket(errPkt)
			continue
		}

		switch pkt.Type {
		case PacketTypeChat:
			// Broadcast to the other clients, as sent by this client's account: the sender id the
			// client wrote is replaced and its token never goes to anyone else.
			pkt.SenderId = c.Id()
			pkt.Token = ""
			c.Server.Broadcast(c, pkt)
		case PacketTypeHeartbeat:
			// A sign of life (already recorded above); also kept on the session token, throttled.
			c.recordTokenSeen()
		case PacketTypeGameAction:
			c.Server.handleGameAction(c, pkt)
		default:
			if c.Server.handleRoomPacket(c, pkt) {
				continue
			}
			// Anything else is not a client packet.
			errPkt := CreatePacket(PacketTypeError, "Server", "", fmt.Sprintf("Unsupported packet type: %s", pkt.Type), "")
			c.SendPacket(errPkt)
		}
	}

	// The loop ends on EOF (no error), on a closed connection, or on a line over MaxPacketSize.
	if err := scanner.Err(); err != nil {
		logger.Warnf("Read error from %s: %v", c.RemoteAddr, err)
	}
}

// SendPacket sends a Packet to the client as JSON (one line); safe from any goroutine.
func (c *Client) SendPacket(pkt *Packet) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	fmt.Fprintln(c.Connection, pkt.SerializePacket())
}

// Name is the account's display name.
func (c *Client) Name() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.DisplayName
}

// SetRoom records the room c is in ("" for none).
func (c *Client) SetRoom(roomId string) {
	c.mu.Lock()
	c.RoomId = roomId
	c.mu.Unlock()
}

// recordTokenSeen writes the token's last-seen time (db.UpdateTokenHeartbeat), at most once per
// TokenSeenWriteInterval. Called from the client's own goroutine only.
func (c *Client) recordTokenSeen() {
	if time.Since(c.tokenSeenWrittenAt) < TokenSeenWriteInterval {
		return
	}
	c.tokenSeenWrittenAt = time.Now()
	_, token := c.HeartbeatInfo()
	if err := db.UpdateTokenHeartbeat(c.Server.dbConn, token); err != nil {
		logger.Warnf("Could not update the token of %s: %v", c.RemoteAddr, err)
	}
}
