/* ----- ----- ----- ----- */
// db.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/05
// Version: v1.1
/* ----- ----- ----- ----- */

package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// InitDB opens the SQLite database server/db/chess.db (relative to the working directory: run the
// server from the repo root) and migrates the tables.
func InitDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("server/db/chess.db"), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&User{}, &Token{}, &Game{}); err != nil {
		return nil, err
	}
	return db, nil
}
