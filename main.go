/* ----- ----- ----- ----- */
// main.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package main

import (
	"fmt"
	"net"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server"
	"Chinese-Chess-v3-Server/server/db"
)

func main() {
	// Init database
	fmt.Println("== Server Booting ==")

	dbConn, err := db.InitDB()
	if err != nil {
		logger.Errorf("Failed to initialize DB: %v", err)
	}

	// Create server instance
	srv := server.NewServer(dbConn)

	// Launch heartbeat system
	srv.StartHeartbeatSystem()

	// Start TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		logger.Errorf("Failed to start server: %v", err)
	}
	defer listener.Close()

	logger.Infof("Chess server started at 127.0.0.1:8080")

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Errorf("Connection error:", err)
			continue
		}

		// Handle client
		go srv.HandleNewClient(conn)
	}
}
