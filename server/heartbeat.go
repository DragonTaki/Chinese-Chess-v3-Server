/* ----- ----- ----- ----- */
// heartbeat.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package server

import (
	"time"

	"Chinese-Chess-v3-Server/logger"
)

// StartHeartbeatSystem starts the two heartbeat goroutines: the client timeout check and the server heartbeat.
func (s *Server) StartHeartbeatSystem() {
	// Client heartbeat check
	go s.CheckClientHeartbeat(ClientTimeoutLimit)

	// Server heartbeat broadcast
	go s.StartServerHeartbeat()
}

// CheckClientHeartbeat drops (and disconnects) every client silent for longer than timeoutLimit,
// checking every ClientHeartbeatCheckInterval. Any line from a client counts as a sign of life.
func (s *Server) CheckClientHeartbeat(timeoutLimit time.Duration) {
	ticker := time.NewTicker(ClientHeartbeatCheckInterval) // Global check interval
	defer ticker.Stop()

	for range ticker.C {
		var timedOut []*Client
		s.mu.Lock()
		for c := range s.clients {
			if time.Since(c.LastSeen()) > timeoutLimit {
				timedOut = append(timedOut, c)
				delete(s.clients, c)
			}
		}
		s.mu.Unlock()

		// Closed outside the lock; each client's Listen then ends.
		for _, c := range timedOut {
			logger.Warnf("Client timed out: %s", c.RemoteAddr)
			c.Connection.Close()
		}
	}
}

// StartServerHeartbeat sends a heartbeat packet to every authenticated client every ServerHeartbeatSendInterval.
func (s *Server) StartServerHeartbeat() {
	ticker := time.NewTicker(ServerHeartbeatSendInterval) // Every interval broadcast server heartbeat
	defer ticker.Stop()

	for range ticker.C {
		for _, c := range s.snapshotClients() {
			// Connections still in the handshake get no heartbeat (the auth timeout covers them).
			if !c.Authenticated() {
				continue
			}
			roomId, token := c.HeartbeatInfo()
			hbPkt := CreatePacket(PacketTypeHeartbeat, "Server", roomId, "", token)
			c.SendPacket(hbPkt) // Send to every client
		}
	}
}
