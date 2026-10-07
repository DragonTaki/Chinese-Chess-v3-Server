/* ----- ----- ----- ----- */
// packettype.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package server

// PacketType names a packet's kind; the values must match the client's PacketType enum (same names).
type PacketType string

const (
	PacketTypeNotDefined PacketType = "NotDefined"

	// Auth
	PacketTypeAuthRequest  PacketType = "AuthRequest"
	PacketTypeAuthResponse PacketType = "AuthResponse"

	// Room
	PacketTypeRoomList   PacketType = "RoomList"
	PacketTypeCreateRoom PacketType = "CreateRoom"
	PacketTypeJoinRoom   PacketType = "JoinRoom"
	PacketTypeLeaveRoom  PacketType = "LeaveRoom"
	PacketTypeReady      PacketType = "Ready"
	PacketTypeRoomState  PacketType = "RoomState"

	// Chess game
	PacketTypeStartGame  PacketType = "StartGame"
	PacketTypeEndGame    PacketType = "EndGame"
	PacketTypeGameAction PacketType = "GameAction"
	PacketTypeTimerSync  PacketType = "TimerSync"

	// Chat
	PacketTypeChat PacketType = "Chat"

	// Other
	PacketTypeServer    PacketType = "Server"
	PacketTypeHeartbeat PacketType = "Heartbeat"
	PacketTypeError     PacketType = "Error"
)
