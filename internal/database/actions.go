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

// ActionLogEntry holds the values captured by an audit log row. Used by
// WithAuditTx to populate the audit record alongside a database mutation
// in the same transaction.
//
// @aitri-trace BG-024 BL-010
type ActionLogEntry struct {
	UserID  *int64
	Source  string
	Action  string
	Target  string
	Result  string
	Details string
}

// LogActionTx is the in-transaction variant of LogAction. Pass a *sql.Tx
// to record the audit entry inside an open transaction so it commits or
// rolls back together with the surrounding database mutation. Direct
// callers that don't need atomicity should keep using LogAction.
//
// @aitri-trace BG-024 BL-010
func (db *DB) LogActionTx(tx *sql.Tx, e ActionLogEntry) error {
	_, err := tx.Exec(
		`INSERT INTO ActionLog (user_id, source, action, target, result, details) VALUES (?, ?, ?, ?, ?, ?)`,
		e.UserID, e.Source, e.Action, e.Target, e.Result, e.Details,
	)
	return err
}

// WithAuditTx runs fn inside a database transaction. fn receives the tx
// and a pointer to an ActionLogEntry which it populates with the audit
// metadata for whatever it just did. If fn returns nil, the audit entry
// is inserted via the same transaction and the transaction commits; if
// fn returns an error or audit-insertion fails, both the database
// mutation and the audit record roll back so neither lands without the
// other (FR-006 audit-trail integrity, BG-024 / BL-010).
//
// Use this for handlers where a SQLite mutation must not commit without
// its audit record (alerts clear, alert-rule mutations, etc.). External
// operations (Docker start/stop, systemctl, OS reboot) cannot be wrapped
// because they are not database state — keep using the non-atomic
// LogAction for those.
//
// @aitri-trace BG-024 BL-010
func (db *DB) WithAuditTx(fn func(*sql.Tx, *ActionLogEntry) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("audit tx: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck — no-op after Commit

	var entry ActionLogEntry
	if err := fn(tx, &entry); err != nil {
		return err
	}
	if err := db.LogActionTx(tx, entry); err != nil {
		return fmt.Errorf("audit tx: log action: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("audit tx: commit: %w", err)
	}
	return nil
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

// DeleteActionLogs removes action logs. If source is empty, removes all.
func (db *DB) DeleteActionLogs(source string) (int64, error) {
	var (
		res sql.Result
		err error
	)
	if source == "" {
		res, err = db.Exec("DELETE FROM ActionLog")
	} else {
		res, err = db.Exec("DELETE FROM ActionLog WHERE source = ?", source)
	}
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
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
