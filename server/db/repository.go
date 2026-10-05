/* ----- ----- ----- ----- */
// repository.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package db

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"Chinese-Chess-v3-Server/logger"
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

	// Query the user by email (unknown user and query failure both fail the login)
	if err := db.First(&user, "email = ?", email).Error; err != nil {
		return "", false
	}

	// Compare password hash
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
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

	// Update last login (a failure here does not fail the login)
	if err := db.Model(&user).Update("last_login", time.Now()).Error; err != nil {
		logger.Warnf("Could not update last login of %s: %v", user.UID, err)
	}

	return token, true
}
