package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DigestDateLayout is the calendar-date key the digest de-duplicates on
// (FR-080 / AC-080-002: at most one digest per calendar day).
const DigestDateLayout = "2006-01-02"

// DigestLastSentDate returns the local calendar date of the last digest
// attempt, or "" when none was ever made. A corrupt/unparseable value is
// reported as "" (never sent) rather than an error, so a bad marker can never
// permanently suppress the digest — the failure mode of a spurious extra
// digest is trivially better than one that silently stops arriving (NFR-090).
func (db *DB) DigestLastSentDate() (string, error) {
	var date string
	err := db.QueryRow(`SELECT last_sent_date FROM DigestState WHERE id = 1`).Scan(&date)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read digest state: %w", err)
	}
	if date == "" {
		return "", nil
	}
	if _, parseErr := time.Parse(DigestDateLayout, date); parseErr != nil {
		return "", nil // corrupt marker == never sent
	}
	return date, nil
}

// MarkDigestSent records that today's digest was attempted.
//
// It is called on COMPLETION — success or failure. Marking only on success
// would make a broken SMTP relay retry every scheduler tick for the whole hour
// (~60 sends at a dead relay); the failure is surfaced in ActionLog and the
// journal instead (ADR-3 / NFR-091).
func (db *DB) MarkDigestSent(date string) error {
	_, err := db.Exec(`
		INSERT INTO DigestState (id, last_sent_date, updated_at)
		VALUES (1, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			last_sent_date = excluded.last_sent_date,
			updated_at     = CURRENT_TIMESTAMP`, date)
	if err != nil {
		return fmt.Errorf("mark digest sent: %w", err)
	}
	return nil
}

// AlertsSince returns every alert created at or after t, newest first. It
// powers the 24h digest summary. The Alert table is retention-pruned, so the
// result set is bounded without an explicit LIMIT.
func (db *DB) AlertsSince(t time.Time) ([]Alert, error) {
	rows, err := db.Query(`
		SELECT id, config_id, severity, message, source, value, acknowledged, created_at
		FROM Alert
		WHERE created_at >= ?
		ORDER BY created_at DESC`, t.UTC())
	if err != nil {
		return nil, fmt.Errorf("query alerts since %s: %w", t.Format(time.RFC3339), err)
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		var ack int
		if err := rows.Scan(&a.ID, &a.ConfigID, &a.Severity, &a.Message, &a.Source, &a.Value, &ack, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		a.Acknowledged = ack == 1
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}
