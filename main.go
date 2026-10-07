/* ----- ----- ----- ----- */
// main.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/07
// Version: v1.3
/* ----- ----- ----- ----- */

package main

import (
	"crypto/tls"
	"fmt"
	"os"

	"Chinese-Chess-v3-Server/logger"
	"Chinese-Chess-v3-Server/server"
	"Chinese-Chess-v3-Server/server/db"
	"Chinese-Chess-v3-Server/server/rules"
)

// DefaultListenAddr is the address served when CHESS_LISTEN_ADDR is not set (this machine only).
const DefaultListenAddr = "127.0.0.1:8080"

// tlsConfig is the server's TLS setup (author 2026-10-07: every connection is TLS, no plain-text
// fallback): the certificate and key files from CHESS_TLS_CERT / CHESS_TLS_KEY (PEM), TLS 1.3 only.
func tlsConfig() (*tls.Config, error) {
	certPath, keyPath := os.Getenv("CHESS_TLS_CERT"), os.Getenv("CHESS_TLS_KEY")
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("CHESS_TLS_CERT and CHESS_TLS_KEY must name the certificate and key files")
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("loading the TLS certificate: %w", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}, nil
}

// main opens the database, starts the heartbeat system and serves TLS clients, one goroutine per
// connection. Environment: JWT_SECRET (required, see server/jwt), CHESS_TLS_CERT and CHESS_TLS_KEY
// (required: the PEM certificate and key), CHESS_LISTEN_ADDR (default DefaultListenAddr),
// CHESS_DB_PATH (default db.DefaultPath), CHESS_RULES_PATH (the built rules host from the rules
// submodule; optional until games are implemented).
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

	// Start the TLS listener
	tlsConf, err := tlsConfig()
	if err != nil {
		logger.Errorf("Failed to start server: %v", err)
		os.Exit(1)
	}
	addr := envOr("CHESS_LISTEN_ADDR", DefaultListenAddr)
	listener, err := tls.Listen("tcp", addr, tlsConf)
	if err != nil {
		logger.Errorf("Failed to start server: %v", err)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Infof("Chess server started at %s (TLS 1.3)", addr)

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
