/* ----- ----- ----- ----- */
// client.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2025/11/01
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

// Client represents a connected client
type Client struct {
	Connection      net.Conn
	RemoteAddr      string
	Server          *Server
	LastSeenAt      time.Time
	IsAuthenticated bool
	SenderId        string // Format: GUID
	Token           string
	RoomId          string
}

func NewClient(conn net.Conn, srv *Server) *Client {
	return &Client{
		Connection: conn,
		RemoteAddr: conn.RemoteAddr().String(),
		Server:     srv,
	}
}

func (c *Client) Listen() {
	defer func() {
		c.Connection.Close()
		c.Server.RemoveClient(c)
	}()

	// Auth session
	if ok := c.Server.Authenticate(c, AuthTimeoutLimit); !ok { // If auth fail
		return
	}

	// Send JSON welcome message using CreatePacket
	welcomePkt := CreatePacket(PacketTypeServer, "Server", "", "Welcome to Go-Chess-Server! Type message to chat.", "")
	c.SendPacket(welcomePkt)

	scanner := bufio.NewScanner(c.Connection)
	for scanner.Scan() {
		line := scanner.Text()
		c.LastSeenAt = time.Now()

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

		// Broadcast chat packet to other clients
		if pkt.Type == PacketTypeChat {
			c.Server.Broadcast(c, pkt)
		}
	}
}

// SendPacket sends a Packet to the client as JSON
func (c *Client) SendPacket(pkt *Packet) {
	fmt.Fprintln(c.Connection, pkt.SerializePacket())
}
