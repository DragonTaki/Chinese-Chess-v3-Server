/* ----- ----- ----- ----- */
// constants.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package server

import "time"

// ServerVersion is the protocol version a client must send in the first handshake stage.
const ServerVersion = "v1.0.0"

// AuthSuccessString is the Data of a successful AuthResponse (the client's AuthManager checks for it).
const AuthSuccessString = "Taki"

// The Data of a failed AuthResponse: why the handshake failed (any Data other than
// AuthSuccessString is a failure to the client). The connection is closed after it.
const (
	AuthFailVersionMismatch    = "VersionMismatch"
	AuthFailMissingCredentials = "MissingCredentials"
	AuthFailInvalidCredentials = "InvalidCredentials"
)

// MaxPacketSize is the longest packet line accepted (bytes); a longer line ends the connection.
const MaxPacketSize = 1 << 20

// Timeouts and intervals: the handshake must finish within AuthTimeoutLimit; a client silent for
// ClientTimeoutLimit is dropped (checked every ClientHeartbeatCheckInterval); the server sends a
// heartbeat every ServerHeartbeatSendInterval.

const (
	AuthTimeoutLimit             = 10 * time.Second
	ClientTimeoutLimit           = 1 * time.Minute
	ClientHeartbeatCheckInterval = 10 * time.Second
	ServerHeartbeatSendInterval  = 3 * time.Second

	// TokenSeenWriteInterval: a client's heartbeats update its token's last-seen time in the
	// database at most this often (the client sends one every second).
	TokenSeenWriteInterval = 30 * time.Second
)
