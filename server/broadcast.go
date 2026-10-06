/* ----- ----- ----- ----- */
// broadcast.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/06
// Version: v1.1
/* ----- ----- ----- ----- */

package server

// Broadcast sends pkt to every authenticated client except sender.
func (s *Server) Broadcast(sender *Client, pkt *Packet) {
	for _, c := range s.snapshotClients() {
		// Connections still in the handshake receive nothing.
		if c != sender && c.Authenticated() {
			c.SendPacket(pkt)
		}
	}
}
