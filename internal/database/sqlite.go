package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

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

CREATE TABLE IF NOT EXISTS ActionLog (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER,
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

	// Run schema migration
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot initialize schema: %w", err)
	}

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
