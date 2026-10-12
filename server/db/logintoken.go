/* ----- ----- ----- ----- */
// logintoken.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/12
// Update Date: 2026/10/12
// Version: v1.0
/* ----- ----- ----- ----- */

package db

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Remembered logins (ONLINE-PLAY 8.25): a login token is 32 random bytes, base64url without
// padding (43 characters); it is valid for LoginTokenTTL and an account has at most
// LoginTokenMaxPerAccount live ones (the oldest is revoked).
const (
	LoginTokenTTL           = 30 * 24 * time.Hour
	LoginTokenMaxPerAccount = 10
)

// Why a login token was revoked (LoginToken.RevokedBy).
const (
	LoginTokenRotated = "Rotated" // used to log in: replaced by a new one
	LoginTokenLogout  = "Logout"  // the client logged out
	LoginTokenCapped  = "Capped"  // the account had too many
	LoginTokenReused  = "Reused"  // a rotated token of the account was used again: all revoked
)

// ErrInvalidLoginToken: the login token is unknown, expired or revoked.
var ErrInvalidLoginToken = errors.New("invalid login token")

// hashLoginToken is the stored form of a login token.
func hashLoginToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// newLoginToken makes a token and its row for uid (not stored).
func newLoginToken(uid string, now time.Time) (string, LoginToken) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand does not fail on supported platforms
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	return token, LoginToken{Hash: hashLoginToken(token), UserID: uid, CreatedAt: now, ExpiresAt: now.Add(LoginTokenTTL), LastUsed: now}
}

// storeLoginToken stores row (in tx) and keeps the account within bounds: its expired rows are
// deleted and live tokens beyond LoginTokenMaxPerAccount revoked, oldest first.
func storeLoginToken(tx *gorm.DB, row LoginToken, now time.Time) error {
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ? AND expires_at <= ?", row.UserID, now).Delete(&LoginToken{}).Error; err != nil {
		return err
	}
	var live []LoginToken
	if err := tx.Where("user_id = ? AND revoked = ?", row.UserID, false).Order("created_at desc, hash").Find(&live).Error; err != nil {
		return err
	}
	if len(live) <= LoginTokenMaxPerAccount {
		return nil
	}
	var old []string
	for _, t := range live[LoginTokenMaxPerAccount:] {
		old = append(old, t.Hash)
	}
	return tx.Model(&LoginToken{}).Where("hash IN ?", old).
		Updates(map[string]any{"revoked": true, "revoked_by": LoginTokenCapped}).Error
}

// IssueLoginToken issues a new login token for the user uid (a password login asking to be
// remembered) and returns it.
func IssueLoginToken(db *gorm.DB, uid string) (string, error) {
	now := time.Now()
	token, row := newLoginToken(uid, now)
	err := db.Transaction(func(tx *gorm.DB) error { return storeLoginToken(tx, row, now) })
	if err != nil {
		return "", err
	}
	return token, nil
}

// revokeAll revokes every live login token of uid as LoginTokenReused.
func revokeAll(db *gorm.DB, uid string) error {
	return db.Model(&LoginToken{}).Where("user_id = ? AND revoked = ?", uid, false).
		Updates(map[string]any{"revoked": true, "revoked_by": LoginTokenReused}).Error
}

// CheckLoginToken returns the account a login token logs in (without using it up: see
// RotateLoginToken). ErrInvalidLoginToken when it is unknown, expired or revoked; a token revoked
// by rotation (used before) revokes every token of its account too (it was copied).
func CheckLoginToken(db *gorm.DB, token string) (string, error) {
	var row LoginToken
	if err := db.First(&row, "hash = ?", hashLoginToken(token)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrInvalidLoginToken
		}
		return "", err
	}
	if row.Revoked {
		if row.RevokedBy == LoginTokenRotated {
			if err := revokeAll(db, row.UserID); err != nil {
				return "", err
			}
		}
		return "", ErrInvalidLoginToken
	}
	if !time.Now().Before(row.ExpiresAt) {
		return "", ErrInvalidLoginToken
	}
	return row.UserID, nil
}

// RotateLoginToken uses up the login token of the user uid (revoked as LoginTokenRotated) and
// returns its replacement. ErrInvalidLoginToken when it is no longer live (used meanwhile by
// another login: then every token of the account is revoked, as in CheckLoginToken; or expired).
func RotateLoginToken(db *gorm.DB, uid, token string) (string, error) {
	now := time.Now()
	next, row := newLoginToken(uid, now)
	reused := false
	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&LoginToken{}).
			Where("hash = ? AND user_id = ? AND revoked = ? AND expires_at > ?", hashLoginToken(token), uid, false, now).
			Updates(map[string]any{"revoked": true, "revoked_by": LoginTokenRotated, "last_used": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			reused = true
			return ErrInvalidLoginToken
		}
		return storeLoginToken(tx, row, now)
	})
	if reused {
		var old LoginToken
		if db.First(&old, "hash = ?", hashLoginToken(token)).Error == nil && old.RevokedBy == LoginTokenRotated {
			if err := revokeAll(db, uid); err != nil {
				return "", err
			}
		}
	}
	if err != nil {
		return "", err
	}
	return next, nil
}

// RevokeLoginToken revokes the user uid's login token (logout); false when it was not a live token of uid.
func RevokeLoginToken(db *gorm.DB, uid, token string) (bool, error) {
	res := db.Model(&LoginToken{}).Where("hash = ? AND user_id = ? AND revoked = ?", hashLoginToken(token), uid, false).
		Updates(map[string]any{"revoked": true, "revoked_by": LoginTokenLogout})
	return res.RowsAffected > 0, res.Error
}
