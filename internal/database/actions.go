package database

import "time"

// ActionLog represents a single audited action entry.
type ActionLog struct {
	ID        int64
	UserID    *int64
	Action    string
	Target    string
	Result    string
	Details   string
	CreatedAt time.Time
}

// LogAction inserts an action log entry. result should be "success" or "error".
func (db *DB) LogAction(userID *int64, action, target, result, details string) error {
	_, err := db.Exec(
		`INSERT INTO ActionLog (user_id, action, target, result, details) VALUES (?, ?, ?, ?, ?)`,
		userID, action, target, result, details,
	)
	return err
}

// ListActionLogs returns the most recent action log entries (newest first).
func (db *DB) ListActionLogs(limit int) ([]ActionLog, error) {
	rows, err := db.Query(
		`SELECT id, user_id, action, target, result, details, created_at
		 FROM ActionLog ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []ActionLog
	for rows.Next() {
		var l ActionLog
		var details *string
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.Target, &l.Result, &details, &l.CreatedAt); err != nil {
			return nil, err
		}
		if details != nil {
			l.Details = *details
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
