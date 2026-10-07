/* ----- ----- ----- ----- */
// client.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/07
// Version: v1.4
/* ----- ----- ----- ----- */

package server

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server/db"
)

// Client represents a connected client. LastSeenAt, IsAuthenticated, SenderId, Token and RoomId
// are written by the client's own goroutine and read by the heartbeat goroutines, inGame by the
// game's: access them through the methods below, which hold mu.
//
// Outgoing packets go through a bounded queue drained by the client's writer goroutine (writeLoop),
// so a sender (often holding a game's lock) never waits on the network. Make clients with
// NewClient, which starts the writer.
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

	// inGame is whether the client plays a running game: its read deadline is then
	// InGameTimeoutLimit instead of ClientTimeoutLimit.
	inGame bool

	mu sync.Mutex

	queue     chan []byte   // packet lines waiting for writeLoop, in send order (SendQueueSize)
	done      chan struct{} // closed by Close: nothing more is sent, writeLoop ends
	closing   chan struct{} // closed by Finish: writeLoop writes what is queued, then Close
	stopped   chan struct{} // closed when writeLoop has ended
	closeOnce sync.Once
	finOnce   sync.Once

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

// readTimeout is how long a client may stay silent before it is dropped: InGameTimeoutLimit while
// it plays a running game, ClientTimeoutLimit otherwise.
func readTimeout(inGame bool) time.Duration {
	if inGame {
		return InGameTimeoutLimit
	}
	return ClientTimeoutLimit
}

// refreshReadDeadline sets the connection's read deadline to now plus the client's readTimeout.
// Under mu, so it never undoes a SetInGame running meanwhile (whichever runs last uses the
// current inGame).
func (c *Client) refreshReadDeadline() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Connection != nil {
		c.Connection.SetReadDeadline(time.Now().Add(readTimeout(c.inGame)))
	}
}

// SetInGame records whether the client plays a running game and applies the matching read
// deadline at once (also to a read already waiting). Harmless on a closed connection.
func (c *Client) SetInGame(inGame bool) {
	c.mu.Lock()
	c.inGame = inGame
	c.mu.Unlock()
	c.refreshReadDeadline()
}

// NewClient wraps a new connection of srv and starts its writer goroutine (it ends on Close).
func NewClient(conn net.Conn, srv *Server) *Client {
	c := &Client{
		Connection: conn,
		RemoteAddr: conn.RemoteAddr().String(),
		Server:     srv,
		queue:      make(chan []byte, SendQueueSize),
		done:       make(chan struct{}),
		closing:    make(chan struct{}),
		stopped:    make(chan struct{}),
	}
	go c.writeLoop()
	return c
}

// Listen serves the connection: the handshake, a welcome message, then one JSON packet per line
// until the connection ends or the client sends "/quit". Only chat packets are handled so far
// (broadcast to the other clients); the room / game packet types are not implemented.
func (c *Client) Listen() {
	// The reply already queued (an auth failure's reason, say) still goes out before the close.
	defer func() {
		c.Finish()
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

	// Each line must arrive within the client's readTimeout (Heartbeat packets keep a quiet client
	// alive); a read past the deadline ends the loop and so the connection.
	for {
		c.refreshReadDeadline()
		if !scanner.Scan() {
			break
		}
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

	// The loop ends on EOF (no error), on a closed connection, on the read deadline or on a line
	// over MaxPacketSize.
	var netErr net.Error
	if err := scanner.Err(); errors.As(err, &netErr) && netErr.Timeout() {
		logger.Warnf("Client silent too long: %s", c.RemoteAddr)
	} else if err != nil {
		logger.Warnf("Read error from %s: %v", c.RemoteAddr, err)
	}
}

// SendPacket queues a Packet for the client as JSON (one line); safe from any goroutine and never
// blocks. Packets reach the connection in the order queued. A client whose queue is full (it does
// not read, or its link is too slow) is closed and the packet dropped; so is every packet after
// Close or Finish (an away player's seat still points at a closed client).
func (c *Client) SendPacket(pkt *Packet) {
	select {
	case <-c.done:
		return
	case <-c.closing:
		return
	default:
	}
	line := append([]byte(pkt.SerializePacket()), '\n')
	select {
	case c.queue <- line:
	default:
		logger.Warnf("Send queue full, closing %s", c.RemoteAddr)
		c.Close()
	}
}

// Close ends the client at once: queued packets are dropped, the writer goroutine ends and the
// connection is closed (so its read loop ends too and the client goes through RemoveClient).
// Idempotent; never blocks (a TLS close may wait to send its close alert, so it runs on its own).
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		go c.Connection.Close()
	})
}

// Finish ends the client once its queued packets are written (each within WriteTimeout): for the
// end of the read loop, so a last reply still goes out. Nothing sent after it is queued.
func (c *Client) Finish() {
	c.finOnce.Do(func() { close(c.closing) })
}

// Closed reports whether the client was closed or finished (its connection is gone or going).
func (c *Client) Closed() bool {
	select {
	case <-c.done:
		return true
	case <-c.closing:
		return true
	default:
		return false
	}
}

// writeLoop writes the queued packet lines to the connection in order, each within WriteTimeout;
// a failed or timed-out write closes the client. It ends on Close, or on Finish once the queue is
// empty.
func (c *Client) writeLoop() {
	defer close(c.stopped)
	for {
		select {
		case <-c.done:
			return
		case line := <-c.queue:
			if !c.write(line) {
				return
			}
		case <-c.closing:
			for {
				select {
				case line := <-c.queue:
					if !c.write(line) {
						return
					}
				default:
					c.Close()
					return
				}
			}
		}
	}
}

// write writes one line within WriteTimeout; on an error it closes the client and returns false.
func (c *Client) write(line []byte) bool {
	c.Connection.SetWriteDeadline(time.Now().Add(WriteTimeout))
	if _, err := c.Connection.Write(line); err != nil {
		select {
		case <-c.done: // closed meanwhile: the error is the close's
		default:
			logger.Warnf("Write to %s failed: %v", c.RemoteAddr, err)
		}
		c.Close()
		return false
	}
	return true
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
