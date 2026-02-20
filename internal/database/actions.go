package database

import (
	"database/sql"
	"fmt"
	"time"
)

// ActionLog represents a single audited action entry.
type ActionLog struct {
	ID        int64
	UserID    *int64
	Source    string // "docker" or "systemd"
	Action    string
	Target    string
	Result    string
	Details   string
	CreatedAt time.Time
}

// LogAction inserts an action log entry. source is "docker" or "systemd"; result is "success" or "error".
func (db *DB) LogAction(userID *int64, source, action, target, result, details string) error {
	_, err := db.Exec(
		`INSERT INTO ActionLog (user_id, source, action, target, result, details) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, source, action, target, result, details,
	)
	return err
}

// ListActionLogs returns the most recent action log entries (newest first).
func (db *DB) ListActionLogs(limit int) ([]ActionLog, error) {
	rows, err := db.Query(
		`SELECT id, user_id, source, action, target, result, details, created_at
		 FROM ActionLog ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	return scanActionLogs(rows)
}

// ListActionLogsBySource returns paginated logs filtered by source ("docker", "systemd", or "" for all).
func (db *DB) ListActionLogsBySource(source string, page, limit int) ([]ActionLog, error) {
	offset := page * limit
	var rows *sql.Rows
	var err error

	if source != "" {
		rows, err = db.Query(
			`SELECT id, user_id, source, action, target, result, details, created_at
			 FROM ActionLog WHERE source = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			source, limit, offset,
		)
	} else {
		rows, err = db.Query(
			`SELECT id, user_id, source, action, target, result, details, created_at
			 FROM ActionLog ORDER BY created_at DESC LIMIT ? OFFSET ?`,
			limit, offset,
		)
	}
	if err != nil {
		return nil, err
	}
	return scanActionLogs(rows)
}

func scanActionLogs(rows *sql.Rows) ([]ActionLog, error) {
	defer rows.Close()
	var logs []ActionLog
	for rows.Next() {
		var l ActionLog
		var details *string
		if err := rows.Scan(&l.ID, &l.UserID, &l.Source, &l.Action, &l.Target, &l.Result, &details, &l.CreatedAt); err != nil {
			return nil, err
		}
		if details != nil {
			l.Details = *details
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// PruneOldData deletes ActionLog and Alert records older than the given number
// of days. Returns the total number of rows deleted.
func (db *DB) PruneOldData(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)

	res1, err := db.Exec(`DELETE FROM ActionLog WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune action logs: %w", err)
	}
	n1, _ := res1.RowsAffected()

	res2, err := db.Exec(`DELETE FROM Alert WHERE created_at < ?`, cutoff)
	if err != nil {
		return n1, fmt.Errorf("prune alerts: %w", err)
	}
	n2, _ := res2.RowsAffected()

	return n1 + n2, nil
}
