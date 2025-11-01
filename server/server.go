/* ----- ----- ----- ----- */
// server.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2025/11/01
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"net"
	"sync"
	"time"

	"Chinese-Chess-v3-Server/logger"

	"gorm.io/gorm"
)

type Server struct {
	dbConn  *gorm.DB
	clients map[*Client]bool
	mu      sync.Mutex
}

func NewServer(dbConn *gorm.DB) *Server {
	return &Server{
		dbConn:  dbConn,
		clients: make(map[*Client]bool),
	}
}

// 處理新連線
func (s *Server) HandleNewClient(conn net.Conn) {
	client := NewClient(conn, s)
	client.LastSeenAt = time.Now()

	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	logger.Infof("New client connected: %s", conn.RemoteAddr().String())

	client.Listen()
}

// 廣播訊息給所有玩家
func (s *Server) Broadcast(sender *Client, pkt *Packet) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for c := range s.clients {
		if c != sender {
			c.SendPacket(pkt)
		}
	}
}

// 客戶端離線時移除
func (s *Server) RemoveClient(c *Client) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
	logger.Warnf("Client disconnected: %s\n", c.RemoteAddr)
}
