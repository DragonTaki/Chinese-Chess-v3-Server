/* ----- ----- ----- ----- */
// models.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/12
// Version: v1.2
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

// LoginToken is a remembered login (ONLINE-PLAY 8.25): a long-lived, revocable token the client
// keeps (encrypted by the OS) to log in without the password. Only its SHA-256 hash is stored. Each
// use rotates it (the used one is revoked with RevokedBy LoginTokenRotated, a new one issued); a
// revoked row stays until it expires, so a rotated token used again is recognized (theft).
type LoginToken struct {
	Hash      string `gorm:"primaryKey"` // SHA-256 of the token, hex
	UserID    string `gorm:"index;not null"`
	CreatedAt time.Time
	ExpiresAt time.Time
	LastUsed  time.Time
	Revoked   bool
	RevokedBy string // why it was revoked: LoginTokenRotated, LoginTokenLogout, LoginTokenCapped, LoginTokenReused
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
