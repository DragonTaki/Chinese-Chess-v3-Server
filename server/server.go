/* ----- ----- ----- ----- */
// server.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/12
// Version: v1.5
/* ----- ----- ----- ----- */

package server

import (
	"net"
	"sync"
	"time"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server/rules"

	"gorm.io/gorm"
)

// Server holds the database, the rules host and the connected clients (mu guards clients).
type Server struct {
	dbConn  *gorm.DB
	rules   *rules.Host // checks moves; nil when CHESS_RULES_PATH is not set (games are not implemented yet)
	geo     geoLookup   // places logins (duplicate-login rule); nil without a database (SetGeoDB)
	clients map[*Client]bool
	rooms   *RoomManager
	mu      sync.Mutex
	loginMu sync.Mutex      // one login admitted at a time (login)
	pending *pendingResults // results of games that ended while a player was away (delivered at their next login)
	// seatGrace is how long a waiting room keeps a disconnected player's seat (RoomSeatGrace; shorter in tests).
	seatGrace time.Duration
}

// NewServer creates a server using dbConn and the rules host (may be nil).
func NewServer(dbConn *gorm.DB, rulesHost *rules.Host) *Server {
	return &Server{
		dbConn:  dbConn,
		rules:   rulesHost,
		clients: make(map[*Client]bool),
		rooms:   NewRoomManager(),
		pending: newPendingResults(),

		seatGrace: RoomSeatGrace,
	}
}

// SetGeoDB makes the duplicate-login rule place public addresses with db (call before serving).
func (s *Server) SetGeoDB(db *GeoDB) {
	s.geo = db.Lookup
}

// HandleNewClient registers a new connection and serves it until it ends (call in its own goroutine).
func (s *Server) HandleNewClient(conn net.Conn) {
	client := NewClient(conn, s)
	client.Touch()

	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	logger.Infof("New client connected: %s", conn.RemoteAddr().String())

	client.Listen()
}

// snapshotClients returns the connected clients; taken under mu, so the caller can then write to
// them without holding mu (a slow connection must not block everyone else).
func (s *Server) snapshotClients() []*Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]*Client, 0, len(s.clients))
	for c := range s.clients {
		list = append(list, c)
	}
	return list
}

// RemoveClient forgets a client whose connection ended.
func (s *Server) RemoveClient(c *Client) {
	s.mu.Lock()
	_, present := s.clients[c]
	delete(s.clients, c)
	s.mu.Unlock()
	// A disconnect keeps the seat (away): in a running game by the game's rules, in a waiting room
	// for seatGrace; otherwise it leaves the room.
	s.disconnect(c)
	// A client the heartbeat already dropped for timing out was logged there.
	if present {
		logger.Warnf("Client disconnected: %s", c.RemoteAddr)
	}
}
