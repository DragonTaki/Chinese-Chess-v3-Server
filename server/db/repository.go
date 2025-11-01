/* ----- ----- ----- ----- */
// repository.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2025/11/01
// Version: v1.0
/* ----- ----- ----- ----- */

package db

import (
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"Chinese-Chess-v3-Server/server/jwt"
)

func CreateToken(db *gorm.DB, userID string, token string, ttl time.Duration) error {
	t := Token{
		Token:     token,
		UserID:    userID,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
		LastSeen:  time.Now(),
	}
	return db.Create(&t).Error
}

func UpdateTokenHeartbeat(db *gorm.DB, token string) error {
	return db.Model(&Token{}).Where("token = ?", token).Update("last_seen", time.Now()).Error
}

// Verify user's email and password
// Return token if success
func VerifyUser(db *gorm.DB, email string, password string) (string, bool) {
	var user User

	// Query username
	err := db.First(&user, "email = ?", email).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", false
		}
		return "", false
	}

	// Compare password hash
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	fmt.Println("Stored hash:", user.PasswordHash)
	if err != nil {
		return "", false
	}

	// Generate token
	token, err := jwt.GenerateTokenJWT(user.UID, time.Hour*24)
	if err != nil {
		return "", false
	}

	// Write to database
	err = CreateToken(db, user.UID, token, time.Hour*24)
	if err != nil {
		return "", false
	}

	// Update last login
	db.Model(&user).Update("last_login", time.Now())

	return token, true
}
