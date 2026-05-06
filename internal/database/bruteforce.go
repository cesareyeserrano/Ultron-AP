// Persistent brute-force lockout store. The methods here are the database
// surface the auth.BruteForceTracker uses when configured for persistence.
// Every call is idempotent and safe to drive from concurrent HTTP handlers.
//
// Method signatures use primitive return tuples (no struct types crossing
// the boundary) so the auth package never imports internal/database — the
// dependency direction stays one-way through internal/server's wiring.
//
// @aitri-trace BG-022 BL-009
package database

import (
	"database/sql"
	"errors"
	"time"
)

// BruteForceLookup returns the attempt record for ip. found is false when
// no row exists; in that case count, firstAt and err are zero values.
func (db *DB) BruteForceLookup(ip string) (count int, firstAt time.Time, found bool, err error) {
	var firstAtNs int64
	scanErr := db.QueryRow(
		`SELECT count, first_at FROM brute_force_attempts WHERE ip = ?`,
		ip,
	).Scan(&count, &firstAtNs)
	if errors.Is(scanErr, sql.ErrNoRows) {
		return 0, time.Time{}, false, nil
	}
	if scanErr != nil {
		return 0, time.Time{}, false, scanErr
	}
	return count, time.Unix(0, firstAtNs), true, nil
}

// BruteForceRecordFailure increments the count for ip atomically (with
// window rollover) and returns the post-update record. If no prior row
// exists or the previous row is older than window, the count resets to 1.
func (db *DB) BruteForceRecordFailure(ip string, window time.Duration, now time.Time) (count int, firstAt time.Time, err error) {
	tx, txErr := db.Begin()
	if txErr != nil {
		return 0, time.Time{}, txErr
	}
	defer tx.Rollback() //nolint:errcheck — rollback is a no-op after commit

	var (
		existingCount     int
		existingFirstAtNs int64
	)
	scanErr := tx.QueryRow(
		`SELECT count, first_at FROM brute_force_attempts WHERE ip = ?`,
		ip,
	).Scan(&existingCount, &existingFirstAtNs)

	switch {
	case errors.Is(scanErr, sql.ErrNoRows):
		count = 1
		firstAt = now
	case scanErr != nil:
		return 0, time.Time{}, scanErr
	default:
		existingFirstAt := time.Unix(0, existingFirstAtNs)
		if now.Sub(existingFirstAt) > window {
			count = 1
			firstAt = now
		} else {
			count = existingCount + 1
			firstAt = existingFirstAt
		}
	}

	if _, err := tx.Exec(`INSERT INTO brute_force_attempts(ip, count, first_at)
		VALUES(?, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET count = excluded.count, first_at = excluded.first_at`,
		ip, count, firstAt.UnixNano(),
	); err != nil {
		return 0, time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return 0, time.Time{}, err
	}

	return count, firstAt, nil
}

// BruteForceReset deletes the attempt record for ip. Called on a successful
// login so a legitimate user is not held by their own earlier typos.
func (db *DB) BruteForceReset(ip string) error {
	_, err := db.Exec(`DELETE FROM brute_force_attempts WHERE ip = ?`, ip)
	return err
}

// BruteForcePruneBefore deletes every record whose first_at predates cutoff.
// Returns the number of rows removed.
func (db *DB) BruteForcePruneBefore(cutoff time.Time) (int64, error) {
	res, err := db.Exec(`DELETE FROM brute_force_attempts WHERE first_at < ?`, cutoff.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
