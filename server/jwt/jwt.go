/* ----- ----- ----- ----- */
// jwt.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/02
// Update Date: 2025/11/02
// Version: v1.0
/* ----- ----- ----- ----- */

package jwt

import (
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET 未設定")
	}
	jwtSecret = []byte(secret)
}

// GenerateTokenJWT 產生簽名 JWT，包含 UserID 與過期時間
func GenerateTokenJWT(userID string, ttl time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"userID": userID,
		"exp":    time.Now().Add(ttl).Unix(),
		"iat":    time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateTokenJWT 驗證 Token 是否有效
func ValidateTokenJWT(tokenStr string) (*jwt.Token, error) {
	return jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
}

// ExtractUserID 從有效 Token 取出 UserID
func ExtractUserID(tokenStr string) (string, error) {
	tok, err := ValidateTokenJWT(tokenStr)
	if err != nil || !tok.Valid {
		return "", err
	}

	if claims, ok := tok.Claims.(jwt.MapClaims); ok {
		if uid, ok := claims["userID"].(string); ok {
			return uid, nil
		}
	}

	return "", err
}
