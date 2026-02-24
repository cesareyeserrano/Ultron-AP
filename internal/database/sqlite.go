package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS User (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS Session (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	csrf_token TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL,
	FOREIGN KEY (user_id) REFERENCES User(id)
);

CREATE TABLE IF NOT EXISTS Alert (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	config_id INTEGER,
	severity TEXT NOT NULL CHECK(severity IN ('critical', 'warning', 'info')),
	message TEXT NOT NULL,
	source TEXT NOT NULL,
	value REAL,
	acknowledged INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (config_id) REFERENCES AlertConfig(id)
);

CREATE TABLE IF NOT EXISTS AlertConfig (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	metric TEXT NOT NULL,
	operator TEXT NOT NULL CHECK(operator IN ('>', '<', '>=', '<=', '==')),
	threshold REAL NOT NULL,
	severity TEXT NOT NULL CHECK(severity IN ('critical', 'warning', 'info')),
	enabled INTEGER DEFAULT 1,
	cooldown_minutes INTEGER DEFAULT 15,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS NotificationConfig (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	channel TEXT NOT NULL UNIQUE,
	enabled INTEGER DEFAULT 0,
	config TEXT NOT NULL DEFAULT '{}',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS BackupConfig (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	enabled INTEGER NOT NULL DEFAULT 1,
	interval_hours INTEGER NOT NULL DEFAULT 24,
	retention_count INTEGER NOT NULL DEFAULT 7,
	destination_mode TEXT NOT NULL DEFAULT 'local_only',
	local_path TEXT NOT NULL DEFAULT '',
	encrypt_enabled INTEGER NOT NULL DEFAULT 0,
	encryption_key_ref TEXT NOT NULL DEFAULT '',
	upload_timeout_sec INTEGER NOT NULL DEFAULT 30,
	max_upload_size_mb INTEGER NOT NULL DEFAULT 50,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ActionLog (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER,
	source TEXT NOT NULL DEFAULT '',
	action TEXT NOT NULL,
	target TEXT NOT NULL,
	result TEXT NOT NULL,
	details TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (user_id) REFERENCES User(id)
);
`

type DB struct {
	*sql.DB
}

func New(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create database directory %q: %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open database %q: %w", dbPath, err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot enable WAL mode: %w", err)
	}

	// Set busy timeout to prevent "database is locked" errors during spikes
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot set busy timeout: %w", err)
	}

	// Run schema migration
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot initialize schema: %w", err)
	}

	// Migration: Remove restricted CHECK constraint from NotificationConfig if present
	var ncSQL string
	if err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='NotificationConfig'").Scan(&ncSQL); err == nil {
		// If the SQL contains the old CHECK constraint (which only allowed telegram/email), we migrate.
		if strings.Contains(ncSQL, "CHECK") && !strings.Contains(ncSQL, "performance") {
			log.Println("database: migrating NotificationConfig table to remove restricted CHECK constraint...")
			tx, err := db.Begin()
			if err != nil {
				db.Close()
				return nil, fmt.Errorf("migration tx failed: %w", err)
			}

			if _, err := tx.Exec("ALTER TABLE NotificationConfig RENAME TO NotificationConfig_old"); err != nil {
				tx.Rollback()
				db.Close()
				return nil, fmt.Errorf("migration rename failed: %w", err)
			}

			// Create new table without the restricted CHECK constraint
			if _, err := tx.Exec(`CREATE TABLE NotificationConfig (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				channel TEXT NOT NULL UNIQUE,
				enabled INTEGER DEFAULT 0,
				config TEXT NOT NULL DEFAULT '{}',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`); err != nil {
				tx.Rollback()
				db.Close()
				return nil, fmt.Errorf("migration create failed: %w", err)
			}

			// Copy data
			if _, err := tx.Exec("INSERT INTO NotificationConfig (id, channel, enabled, config, created_at, updated_at) SELECT id, channel, enabled, config, created_at, updated_at FROM NotificationConfig_old"); err != nil {
				tx.Rollback()
				db.Close()
				return nil, fmt.Errorf("migration copy failed: %w", err)
			}

			if _, err := tx.Exec("DROP TABLE NotificationConfig_old"); err != nil {
				tx.Rollback()
				db.Close()
				return nil, fmt.Errorf("migration drop failed: %w", err)
			}

			if err := tx.Commit(); err != nil {
				db.Close()
				return nil, fmt.Errorf("migration commit failed: %w", err)
			}
			log.Println("database: NotificationConfig migration completed successfully")
		}
	}

	// Add source column to ActionLog if not present (migration for existing DBs)
	_, _ = db.Exec(`ALTER TABLE ActionLog ADD COLUMN source TEXT NOT NULL DEFAULT ''`)

	// Integrity check
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		db.Close()
		return nil, fmt.Errorf("database integrity check failed: %w", err)
	}
	if result != "ok" {
		db.Close()
		return nil, fmt.Errorf("database integrity check failed: %s — consider backing up and recreating the database", result)
	}

	return &DB{db}, nil
}

// Backup creates a consistent copy of the database at the destination path
// using SQLite's VACUUM INTO command. This is safe even with WAL mode active.
func (db *DB) Backup(dstPath string) error {
	// Ensure the destination doesn't exist (VACUUM INTO fails if it does)
	_ = os.Remove(dstPath)

	escapedPath := strings.ReplaceAll(dstPath, "'", "''")
	_, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapedPath))
	if err != nil {
		return fmt.Errorf("database backup failed: %w", err)
	}
	return nil
}
