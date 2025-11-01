/* ----- ----- ----- ----- */
// auth.go
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
	"time"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server/db"
)

type AuthMessage struct {
	Type     string `json:"type"`
	SenderId string `json:"id"`
	Version  string `json:"version"`
}

// Return true if success; return false if fail or overtime
func (s *Server) Authenticate(c *Client, timeout time.Duration) bool {
	dbConn := s.dbConn
	authCh := make(chan bool, 1)

	go func() {
		scanner := bufio.NewScanner(c.Connection)

		// Stage 1: Version
		// Stage 2: Email and password
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
					authCh <- false
					return
				}

				// Request email and password for stage 2
				respPkt := CreatePacket(PacketTypeAuthRequest, "Server", "", "Please provide username/password", "")
				c.SendPacket(respPkt)
				stage = 2

			case 2:
				if ad.Username == "" || ad.Password == "" {
					logger.Warnf("Missing username/password from %s", c.RemoteAddr)
					authCh <- false
					return
				}

fmt.Println("Input password:", ad.Password)
				token, ok := db.VerifyUser(dbConn, ad.Username, ad.Password)
				if !ok {
					logger.Warnf("Invalid credentials from %s", c.RemoteAddr)
					authCh <- false
					return
				}

				// Auth success
				c.SenderId = pkt.SenderId
				c.IsAuthenticated = true
				c.LastSeenAt = time.Now()

				respPkt := CreatePacket(PacketTypeAuthResponse, "Server", "", AuthSuccessString, token)
				c.SendPacket(respPkt)

				authCh <- true
				return
			}
		}
	}()

	select {
	case ok := <-authCh:
		if ok {
			logger.Infof("Client %s authenticated successfully", c.RemoteAddr)
		}
		return ok
	case <-time.After(timeout):
		logger.Warnf("Client %s failed to authenticate in time", c.RemoteAddr)
		return false
	}
}
