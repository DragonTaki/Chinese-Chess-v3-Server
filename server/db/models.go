/* ----- ----- ----- ----- */
// models.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2025/11/01
// Version: v1.0
/* ----- ----- ----- ----- */

package db

import "time"

type User struct {
	UID          string `gorm:"primaryKey"`      // UUID
	Email        string `gorm:"unique;not null"` // 登入帳號
	PasswordHash string `gorm:"not null"`        // 密碼雜湊
	Username     string // 暱稱（可自由修改）
	CreatedAt    time.Time
	LastLogin    time.Time
}

type Token struct {
	Token     string `gorm:"primaryKey"`
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
	LastSeen  time.Time
}

type Game struct {
	ID          string `gorm:"primaryKey"`
	PlayerRed   string
	PlayerBlack string
	Winner      *string
	StartedAt   time.Time
	EndedAt     *time.Time
}
