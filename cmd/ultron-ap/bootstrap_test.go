package main

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// @aitri-tc TC-007c — the bootstrapped admin password is stored as a
// bcrypt hash, never plaintext (AC-007-005).
func TestBootstrapAdmin_StoresBcryptHash(t *testing.T) {
	cfg := &config.Config{
		DBPath:    filepath.Join(t.TempDir(), "boot.db"),
		AdminUser: "admin",
		AdminPass: "hunter2-plaintext",
	}
	db, err := database.New(cfg.DBPath)
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	defer db.Close()

	if err := bootstrapAdmin(cfg, db); err != nil {
		t.Fatalf("bootstrapAdmin: %v", err)
	}

	user, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}

	if user.PasswordHash == cfg.AdminPass {
		t.Fatal("admin password stored as plaintext")
	}
	if !strings.HasPrefix(user.PasswordHash, "$2a$") && !strings.HasPrefix(user.PasswordHash, "$2b$") {
		t.Fatalf("stored credential is not a bcrypt hash: %q", user.PasswordHash[:4])
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(cfg.AdminPass)); err != nil {
		t.Fatalf("stored hash does not verify against the password: %v", err)
	}
	if strings.Contains(user.PasswordHash, cfg.AdminPass) {
		t.Fatal("hash embeds the plaintext password")
	}
}
