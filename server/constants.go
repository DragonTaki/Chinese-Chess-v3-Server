/* ----- ----- ----- ----- */
// constants.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/07
// Version: v1.3
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
// ClientTimeoutLimit is dropped (its read deadline, and a check every ClientHeartbeatCheckInterval),
// one playing a running game already when silent for InGameTimeoutLimit (its read deadline), so a
// disconnect is noticed within a second; the server sends a heartbeat every
// ServerHeartbeatSendInterval.

const (
	AuthTimeoutLimit             = 10 * time.Second
	ClientTimeoutLimit           = 1 * time.Minute
	ClientHeartbeatCheckInterval = 10 * time.Second
	ServerHeartbeatSendInterval  = 3 * time.Second
	InGameTimeoutLimit           = 1 * time.Second

	// InGameHeartbeatInterval is how often a client playing a game is expected to send a Heartbeat
	// packet (from its StartGame until its EndGame), well inside InGameTimeoutLimit; not used by
	// the server itself.
	InGameHeartbeatInterval = 300 * time.Millisecond

	// TokenSeenWriteInterval: a client's heartbeats update its token's last-seen time in the
	// database at most this often (the client sends one every second, every
	// InGameHeartbeatInterval while it plays).
	TokenSeenWriteInterval = 30 * time.Second
)

// Game end reasons of the clocks: the mover's count-down time ran out (TimeUp), or a side was away
// longer than DisconnectGraceSeat where no step time would end its game (Disconnect).
const (
	ClockReasonTimeUp     = "TimeUp"
	ClockReasonDisconnect = "Disconnect"
)

// DisconnectGraceSeat is how long an away side keeps its seat (count-up clocks, or count-down
// without the step timer) before it loses by ClockReasonDisconnect.
const DisconnectGraceSeat = 15 * time.Second

// TimerSyncInterval is how often a running game sends every player its clocks (TimerSync), so a
// client's display stays near the server's between moves.
const TimerSyncInterval = 3 * time.Second
