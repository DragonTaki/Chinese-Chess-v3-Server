/* ----- ----- ----- ----- */
// packet.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package server

import "encoding/json"

// Packet is one protocol message, sent as one JSON line.
type Packet struct {
	Type     PacketType `json:"type"`
	SenderId string     `json:"senderId"`
	RoomId   string     `json:"roomId,omitempty"`
	Data     string     `json:"data"`
	Token    string     `json:"token"`
}

// AuthData is an AuthRequest's Data: the version (stage 1) or the login (stage 2). Username
// carries the account's email (VerifyUser looks users up by email).
type AuthData struct {
	Version  string `json:"version,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// CreatePacket builds a packet.
func CreatePacket(pktType PacketType, senderId, roomId, data, token string) *Packet {
	return &Packet{
		Type:     pktType,
		SenderId: senderId,
		RoomId:   roomId,
		Data:     data,
		Token:    token,
	}
}

// SerializePacket returns the packet as a JSON string (a Packet always marshals, so the error is ignored).
func (p *Packet) SerializePacket() string {
	b, _ := json.Marshal(p)
	return string(b)
}

// DeserializePacket parses a JSON string into a Packet.
func DeserializePacket(jsonStr string) (*Packet, error) {
	var p Packet
	err := json.Unmarshal([]byte(jsonStr), &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ParseAuthData parses the packet's Data as AuthData.
func (p *Packet) ParseAuthData() (*AuthData, error) {
	var ad AuthData
	err := json.Unmarshal([]byte(p.Data), &ad)
	if err != nil {
		return nil, err
	}
	return &ad, nil
}
