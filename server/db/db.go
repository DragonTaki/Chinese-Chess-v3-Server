/* ----- ----- ----- ----- */
// db.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2025/11/01
// Update Date: 2026/10/06
// Version: v1.2
/* ----- ----- ----- ----- */

package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DefaultPath is the database file used when CHESS_DB_PATH is not set (relative to the working
// directory: run the server from the repo root).
const DefaultPath = "server/db/chess.db"

// InitDB opens the SQLite database at path and migrates the tables.
func InitDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&User{}, &Token{}, &Game{}); err != nil {
		return nil, err
	}
	return db, nil
}
