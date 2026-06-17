package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"

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
	target TEXT,
	sustained_duration INTEGER NOT NULL DEFAULT 0,
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
	schedule_mode TEXT NOT NULL DEFAULT 'interval',
	schedule_hour INTEGER NOT NULL DEFAULT 3,
	schedule_minute INTEGER NOT NULL DEFAULT 0,
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

CREATE TABLE IF NOT EXISTS NetSample (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts INTEGER NOT NULL,
	target TEXT NOT NULL,
	kind TEXT NOT NULL,
	rtt_ms REAL,
	status TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_net_sample_ts ON NetSample(ts);
CREATE INDEX IF NOT EXISTS idx_net_sample_target_ts ON NetSample(target, ts);

CREATE TABLE IF NOT EXISTS NetEvent (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts INTEGER NOT NULL,
	kind TEXT NOT NULL,
	detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_net_event_ts ON NetEvent(ts);
CREATE INDEX IF NOT EXISTS idx_net_event_kind_ts ON NetEvent(kind, ts);

CREATE TABLE IF NOT EXISTS lan_devices (
	mac TEXT PRIMARY KEY,
	ip TEXT NOT NULL,
	vendor TEXT NOT NULL DEFAULT 'Unknown',
	first_seen INTEGER NOT NULL,
	last_seen INTEGER NOT NULL,
	online INTEGER NOT NULL DEFAULT 1,
	missed_sweeps INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_lan_devices_online_lastseen
	ON lan_devices(online DESC, last_seen DESC);

CREATE TABLE IF NOT EXISTS rules (
	id              TEXT    PRIMARY KEY,
	title           TEXT    NOT NULL,
	condition_json  TEXT    NOT NULL,
	severity        TEXT    NOT NULL CHECK (severity IN ('info','warn','critical')),
	verdict         TEXT    NOT NULL,
	recommendation  TEXT    NOT NULL,
	links_json      TEXT    NOT NULL DEFAULT '[]',
	enabled         INTEGER NOT NULL DEFAULT 1,
	source          TEXT    NOT NULL DEFAULT 'bundled' CHECK (source IN ('bundled','user')),
	created_at      INTEGER NOT NULL,
	updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rules_enabled_severity ON rules(enabled, severity);

CREATE TABLE IF NOT EXISTS rule_state (
	rule_id                TEXT    PRIMARY KEY,
	last_evaluated_at      INTEGER NOT NULL,
	last_value             INTEGER NOT NULL,
	last_change_at         INTEGER NOT NULL,
	transitions_in_window  INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (rule_id) REFERENCES rules(id)
);

-- Persistent brute-force lockout state. Survives binary restarts so an
-- attacker's accumulated failure count is not reset by a service bounce
-- (BL-009 / BG-022). first_at is unix nanoseconds, count is the number of
-- failures within the lockout window.
CREATE TABLE IF NOT EXISTS brute_force_attempts (
	ip       TEXT    PRIMARY KEY,
	count    INTEGER NOT NULL,
	first_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_brute_force_first_at ON brute_force_attempts(first_at);

-- Single-row AI provider configuration (feature: ai-insights). The api_key_enc
-- column holds the provider key encrypted at rest via the shared AES-GCM secrets
-- mechanism (ULTRON_SECRET_KEY) — never plaintext. Single admin ⇒ id pinned to 1.
CREATE TABLE IF NOT EXISTS ai_settings (
	id             INTEGER PRIMARY KEY CHECK (id = 1),
	enabled        INTEGER NOT NULL DEFAULT 0,
	endpoint_url   TEXT    NOT NULL DEFAULT '',
	model          TEXT    NOT NULL DEFAULT '',
	api_key_enc    TEXT    NOT NULL DEFAULT '',
	telegram_push  INTEGER NOT NULL DEFAULT 0,
	timeout_ms     INTEGER NOT NULL DEFAULT 10000,
	allow_insecure INTEGER NOT NULL DEFAULT 0,
	updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
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

	// Apply pragmas via DSN so every connection in the pool gets them at
	// handshake — busy_timeout is per-connection, and PRAGMA executed after
	// sql.Open() only affects whichever pooled connection runs it. Without
	// this, new connections spawned under load default to busy_timeout=0
	// and return SQLITE_BUSY immediately on lock contention (BG-017).
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)",
		dbPath,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("cannot open database %q: %w", dbPath, err)
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

	// Idempotent ALTER TABLE migrations for existing DBs. The expected
	// failure mode on a re-run is "duplicate column name" — anything
	// else (DB locked, table missing, IO error) is a real fault and
	// must surface. BG-029 / BL-004 closed the silent-error gap that
	// hid those.
	idempotentAlters := []string{
		`ALTER TABLE ActionLog ADD COLUMN source TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE BackupConfig ADD COLUMN schedule_mode TEXT NOT NULL DEFAULT 'interval'`,
		`ALTER TABLE BackupConfig ADD COLUMN schedule_hour INTEGER NOT NULL DEFAULT 3`,
		`ALTER TABLE BackupConfig ADD COLUMN schedule_minute INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE AlertConfig ADD COLUMN target TEXT`,
		`ALTER TABLE AlertConfig ADD COLUMN sustained_duration INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range idempotentAlters {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) {
			db.Close()
			return nil, fmt.Errorf("migration %q: %w", stmt, err)
		}
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

// isDuplicateColumnErr returns true when an ALTER TABLE ADD COLUMN
// failed because the column already exists — the expected outcome of
// an idempotent re-run. Anything else (locked DB, missing table, IO
// error) must surface so a corrupt schema state doesn't get silently
// papered over. modernc.org/sqlite returns the SQLite engine message
// verbatim, which always begins "duplicate column name:".
//
// @aitri-trace BG-029 BL-004
func isDuplicateColumnErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}

// Backup creates a consistent copy of the database at the destination path
// using SQLite's VACUUM INTO command. This is safe even with WAL mode active.
//
// The destination is sanity-checked here as a defense-in-depth measure — the
// HTTP handler that accepts admin-supplied backup directories runs the full
// ValidateBackupPath against ULTRON_BACKUP_ROOT, but VACUUM INTO cannot be
// parameterised so any caller reaching this function must not pass attacker-
// controlled bytes (NUL/control chars, relative paths) regardless.
func (db *DB) Backup(dstPath string) error {
	if dstPath == "" {
		return fmt.Errorf("backup destination is empty")
	}
	if strings.ContainsRune(dstPath, 0) {
		return fmt.Errorf("backup destination contains NUL byte")
	}
	for _, r := range dstPath {
		if unicode.IsControl(r) {
			return fmt.Errorf("backup destination contains control character")
		}
	}
	if !filepath.IsAbs(dstPath) {
		return fmt.Errorf("backup destination must be absolute, got %q", dstPath)
	}

	// Ensure the destination doesn't exist (VACUUM INTO fails if it does)
	_ = os.Remove(dstPath)

	escapedPath := strings.ReplaceAll(dstPath, "'", "''")
	_, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapedPath))
	if err != nil {
		return fmt.Errorf("database backup failed: %w", err)
	}
	return nil
}
