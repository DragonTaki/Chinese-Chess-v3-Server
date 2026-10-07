/* ----- ----- ----- ----- */
// jwt.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/02
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

// init reads the signing secret from JWT_SECRET (taken as is, quotes included); the package
// panics at startup without it.
func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET is not set")
	}
	jwtSecret = []byte(secret)
}

// GenerateTokenJWT returns an HS256-signed JWT with the user id, issue time, expiry (now + ttl) and a
// random jti, so two tokens issued for one user in the same second still differ (tokens are stored unique).
func GenerateTokenJWT(userID string, ttl time.Duration) (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	claims := jwt.MapClaims{
		"userID": userID,
		"exp":    time.Now().Add(ttl).Unix(),
		"iat":    time.Now().Unix(),
		"jti":    hex.EncodeToString(id),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateTokenJWT parses and verifies a token (signature, HS256, expiry).
func ValidateTokenJWT(tokenStr string) (*jwt.Token, error) {
	// Only HS256, the method GenerateTokenJWT signs with: a token naming another algorithm is rejected.
	return jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
}

// ExtractUserID returns the user id of a valid token.
func ExtractUserID(tokenStr string) (string, error) {
	tok, err := ValidateTokenJWT(tokenStr)
	if err != nil {
		return "", err
	}
	if !tok.Valid {
		return "", errors.New("invalid token")
	}

	if claims, ok := tok.Claims.(jwt.MapClaims); ok {
		if uid, ok := claims["userID"].(string); ok {
			return uid, nil
		}
	}

	return "", errors.New("token has no userID claim")
}
