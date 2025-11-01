/* ----- ----- ----- ----- */
// create_fake_user.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2025/11/01
// Version: v1.0
/* ----- ----- ----- ----- */

package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"time"

	"Chinese-Chess-v3-Server/server/db"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	MinUID       = 100_000_000
	MaxJumpDelta = 10
)

func main() {
	numUsers := 10 // 可改成任意生成數量

	// 初始化 SQLite 測試資料庫
	gormDB, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// 自動建立 table
	if err := gormDB.AutoMigrate(&db.User{}); err != nil {
		log.Fatal(err)
	}

	rand.Seed(time.Now().UnixNano())

	// 取得目前最大 UID
	var maxUID sql.NullInt64
	gormDB.Model(&db.User{}).Select("MAX(uid)").Scan(&maxUID)
	var currentUID int64 = MinUID
	if maxUID.Valid {
		currentUID = maxUID.Int64 + 1
	}

	// 生成測試玩家
	for i := 1; i <= numUsers; i++ {
		delta := int64(rand.Intn(MaxJumpDelta) + 1)
		currentUID += delta

		// 密碼 hash
		password := fmt.Sprintf("%d", i)
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal(err)
		}

		user := db.User{
			UID:          fmt.Sprintf("%d", currentUID),
			Email:        fmt.Sprintf("%d@test.com", i),
			PasswordHash: string(passwordHash),
			Username:     fmt.Sprintf("User%d", i),
			CreatedAt:    time.Now(),
			LastLogin:    time.Now(),
		}

		if err := gormDB.Create(&user).Error; err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Created test user: UID=%s, Email=%s, Password=%s\n",
			user.UID, user.Email, password)
	}
}
