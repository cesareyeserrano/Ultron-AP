package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MuteHoursAllowed is the closed set of mute durations the admin can pick
// (FR-079). Anything else is a client error, not a clamped value — a mute is a
// deliberate act and a silently-corrected duration would surprise the admin.
var MuteHoursAllowed = []int{1, 4, 24}

// ErrInvalidMuteHours is returned for a duration outside MuteHoursAllowed.
var ErrInvalidMuteHours = errors.New("mute duration must be 1, 4 or 24 hours")

// SetNotificationMute opens (or replaces) the Telegram mute window and returns
// the instant it expires. FR-079.
func (db *DB) SetNotificationMute(hours int, now time.Time) (time.Time, error) {
	if !validMuteHours(hours) {
		return time.Time{}, ErrInvalidMuteHours
	}
	expiresAt := now.Add(time.Duration(hours) * time.Hour).UTC()

	_, err := db.Exec(`
		INSERT INTO NotificationMute (id, expires_at, hours, created_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			expires_at = excluded.expires_at,
			hours      = excluded.hours,
			created_at = excluded.created_at`,
		expiresAt, hours, now.UTC())
	if err != nil {
		return time.Time{}, fmt.Errorf("set notification mute: %w", err)
	}
	return expiresAt, nil
}

// ClearNotificationMute closes an open mute window. Clearing a window that is
// not open is not an error — the caller's intent (be un-muted) is satisfied.
func (db *DB) ClearNotificationMute() error {
	if _, err := db.Exec(`DELETE FROM NotificationMute WHERE id = 1`); err != nil {
		return fmt.Errorf("clear notification mute: %w", err)
	}
	return nil
}

// NotificationMuteUntil reports whether a Telegram mute window is currently
// open, and when it expires.
//
// It FAILS OPEN: any error — no row, a corrupt timestamp, an unreadable
// database — returns muted=false alongside the error. The send path must
// deliver an alert it cannot prove is muted, because the failure mode of
// failing closed (silently swallowing a critical alert) is far worse than the
// failure mode of failing open (one notification the admin wanted muted).
// NFR-090.
func (db *DB) NotificationMuteUntil(now time.Time) (expiresAt time.Time, muted bool, err error) {
	var raw sql.NullTime
	scanErr := db.QueryRow(`SELECT expires_at FROM NotificationMute WHERE id = 1`).Scan(&raw)
	switch {
	case errors.Is(scanErr, sql.ErrNoRows):
		return time.Time{}, false, nil // not muted — the normal case
	case scanErr != nil:
		return time.Time{}, false, fmt.Errorf("read notification mute: %w", scanErr)
	case !raw.Valid:
		return time.Time{}, false, errors.New("notification mute: expires_at is NULL")
	}

	expiresAt = raw.Time.UTC()
	return expiresAt, now.UTC().Before(expiresAt), nil
}

// MuteHours returns the duration the admin picked for the open window, so the
// UI can label it ("Muted for 4h"). Zero when no window is open.
func (db *DB) MuteHours() (int, error) {
	var hours int
	err := db.QueryRow(`SELECT hours FROM NotificationMute WHERE id = 1`).Scan(&hours)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read mute hours: %w", err)
	}
	return hours, nil
}

func validMuteHours(h int) bool {
	for _, allowed := range MuteHoursAllowed {
		if h == allowed {
			return true
		}
	}
	return false
}
