/* ----- ----- ----- ----- */
// uid_generator.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package utils

import (
	"math/rand"

	"gorm.io/gorm"

	"Chinese-Chess-v3-Server/server/db"
)

const (
	MinUID       = 100_000_000
	MaxJumpDelta = 10 // Each new id is 1 to 10 above the largest
)

// GenerateNextUID returns the next user id: the largest in the database (at least MinUID - 1)
// plus a random 1 to MaxJumpDelta. Not used yet; note User.UID is a string, so MAX(uid) compares
// as text (to be settled with the registration flow).
func GenerateNextUID(gormDB *gorm.DB) (int64, error) {
	// The global source is seeded randomly since Go 1.20 (rand.Seed is deprecated).

	var maxUID int64
	err := gormDB.Model(&db.User{}).Select("MAX(uid)").Scan(&maxUID).Error
	if err != nil {
		return 0, err
	}

	if maxUID < MinUID {
		maxUID = MinUID - 1
	}

	// Random step
	delta := int64(rand.Intn(MaxJumpDelta) + 1)
	nextUID := maxUID + delta

	return nextUID, nil
}
