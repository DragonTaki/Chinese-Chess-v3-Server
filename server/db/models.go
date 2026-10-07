/* ----- ----- ----- ----- */
// models.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package db

import "time"

// User is an account.
type User struct {
	UID          string `gorm:"primaryKey"`      // User id (the test script makes jumping numeric ids)
	Email        string `gorm:"unique;not null"` // Login name
	PasswordHash string `gorm:"not null"`        // bcrypt hash of the password
	Username     string // Nickname (free to change)
	CreatedAt    time.Time
	LastLogin    time.Time
}

// Token is an issued session token (JWT) and its lifetime.
type Token struct {
	Token     string `gorm:"primaryKey"`
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
	LastSeen  time.Time
}

// Game is a played online game, written by the server when it ends (the record that counts: the
// client's own copies are not trusted).
type Game struct {
	ID        string `gorm:"primaryKey"`
	Kind      string // GameKind name (Traditional ...)
	Mode      string // Online mode (Custom ...)
	Rules     string // The game's rule switches and clocks, JSON
	Players   string // The players' user ids in turn order (Player1 first), JSON array
	Winner    string // The winner's user id; "" for a draw
	Reason    string // GameOverReason name
	Moves     string // The moves in order, JSON array of {from, to, notation, ms}
	Clocks    string // The clocks when the game ended, JSON {sides:[{usedMs,stepMs}], toMove, away}
	StartedAt time.Time
	EndedAt   time.Time
}
