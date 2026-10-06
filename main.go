/* ----- ----- ----- ----- */
// main.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/06
// Version: v1.2
/* ----- ----- ----- ----- */

package main

import (
	"fmt"
	"net"
	"os"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server"
	"Chinese-Chess-v3-Server/server/db"
	"Chinese-Chess-v3-Server/server/rules"
)

// DefaultListenAddr is the address served when CHESS_LISTEN_ADDR is not set (this machine only).
const DefaultListenAddr = "127.0.0.1:8080"

// main opens the database, starts the heartbeat system and serves TCP clients, one goroutine per
// connection. Environment: JWT_SECRET (required, see server/jwt), CHESS_LISTEN_ADDR (default
// DefaultListenAddr), CHESS_DB_PATH (default db.DefaultPath), CHESS_RULES_PATH (the built
// rules host from the rules submodule; optional until games are implemented).
func main() {
	fmt.Println("== Server Booting ==")

	// Init database
	dbConn, err := db.InitDB(envOr("CHESS_DB_PATH", db.DefaultPath))
	if err != nil {
		logger.Errorf("Failed to initialize DB: %v", err)
		os.Exit(1)
	}

	// Start the rules host (the shared rules' C# program that checks moves), when configured
	var rulesHost *rules.Host
	if path := os.Getenv("CHESS_RULES_PATH"); path != "" {
		rulesHost, err = rules.Start(path)
		if err != nil {
			logger.Errorf("Failed to start the rules host: %v", err)
			os.Exit(1)
		}
		defer rulesHost.Close()
	} else {
		logger.Warnf("CHESS_RULES_PATH is not set: no rules host (moves cannot be checked)")
	}

	// Create server instance
	srv := server.NewServer(dbConn, rulesHost)

	// Launch heartbeat system
	srv.StartHeartbeatSystem()

	// Start TCP listener
	addr := envOr("CHESS_LISTEN_ADDR", DefaultListenAddr)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Errorf("Failed to start server: %v", err)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Infof("Chess server started at %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Errorf("Connection error: %v", err)
			continue
		}

		// Handle client
		go srv.HandleNewClient(conn)
	}
}

// envOr returns the environment variable name, or fallback when it is unset or empty.
func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
