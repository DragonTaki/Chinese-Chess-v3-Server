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
	MaxJumpDelta = 10 // 每次隨機增加 1~10
)

// GenerateNextUID 從資料庫查最大 UID，生成下一個遞增跳號 UID
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

	// 隨機增量
	delta := int64(rand.Intn(MaxJumpDelta) + 1)
	nextUID := maxUID + delta

	return nextUID, nil
}
