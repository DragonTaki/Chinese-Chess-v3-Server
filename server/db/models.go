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

// Game is a played game (not used yet: games are not implemented).
type Game struct {
	ID          string `gorm:"primaryKey"`
	PlayerRed   string
	PlayerBlack string
	Winner      *string
	StartedAt   time.Time
	EndedAt     *time.Time
}
