/* ----- ----- ----- ----- */
// repository_test.go
// Do not distribute or modify
// Author: DragonTaki (https://github.com/DragonTaki)
// Create Date: 2026/10/07
// Update Date: 2026/10/07
// Version: v1.0
/* ----- ----- ----- ----- */

package db

import (
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Two logins of one account back-to-back (same second) get different tokens and both are stored.
func TestVerifyUserUniqueTokens(t *testing.T) {
	d, err := InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	h, _ := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	if err := d.Create(&User{UID: "u", Email: "u@test.com", PasswordHash: string(h)}).Error; err != nil {
		t.Fatal(err)
	}
	_, t1, ok1 := VerifyUser(d, "u@test.com", "pw")
	_, t2, ok2 := VerifyUser(d, "u@test.com", "pw")
	if !ok1 || !ok2 {
		t.Fatal("login failed")
	}
	if t1 == t2 {
		t.Fatal("identical tokens")
	}
	var n int64
	d.Model(&Token{}).Where("user_id = ?", "u").Count(&n)
	if n != 2 {
		t.Fatalf("stored %d tokens", n)
	}
}
