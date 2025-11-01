/* ----- ----- ----- ----- */
// heartbeat.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2025/11/01
// Version: v1.0
/* ----- ----- ----- ----- */

package server

import (
	"time"

	"Chinese-Chess-v3-Server/logger"
)

func (s *Server) StartHeartbeatSystem() {
	// Client heartbeat check
	go s.CheckClientHeartbeat(ClientTimeoutLimit)

	// Server heartbeat broadcast
	go s.StartServerHeartbeat()
}

// Client heartbeat check
func (s *Server) CheckClientHeartbeat(timeoutLimit time.Duration) {
	ticker := time.NewTicker(ClientHeartbeatCheckInterval) // Global check interval
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		for c := range s.clients {
			if time.Since(c.LastSeenAt) > timeoutLimit {
				logger.Warnf("Client timed out: %s", c.RemoteAddr)
				c.Connection.Close()
				delete(s.clients, c)
			}
		}
		s.mu.Unlock()
	}
}

// Server heartbeat broadcast
func (s *Server) StartServerHeartbeat() {
	ticker := time.NewTicker(ServerHeartbeatSendInterval) // Every interval broadcast server heartbeat
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		for c := range s.clients {
			hbPkt := CreatePacket(PacketTypeHeartbeat, "Server", c.RoomId, "", c.Token)
			c.SendPacket(hbPkt) // Send to every client
		}
		s.mu.Unlock()
	}
}
