// Package store owns all reads and writes against the lan_devices SQLite table.
// It also implements the online/offline state machine (FR-035): a sweep cycle
// classifies each existing row as either "responded this sweep" or "missed
// this sweep", and the store advances last_seen / online / missed_sweeps
// accordingly in a single transaction.
//
// @aitri-trace FR-034 FR-035 US-034 US-035 TC-LD-005h TC-LD-005f TC-LD-005e TC-LD-006h TC-LD-006f TC-LD-006e
package store

import (
	"database/sql"
	"fmt"
	"time"
)

// DefaultMissThreshold — number of consecutive missed sweeps before a device
// flips to offline. Configurable per-orchestrator via NewWithThreshold.
const DefaultMissThreshold = 3

// Device is the read model returned by List.
type Device struct {
	MAC          string
	IP           string
	Vendor       string
	FirstSeen    time.Time
	LastSeen     time.Time
	Online       bool
	MissedSweeps int
}

// Observation is one (mac, ip, vendor) triple coming out of a sweep cycle.
// Callers pass the full set seen during the cycle to ApplySweep.
type Observation struct {
	MAC    string
	IP     string
	Vendor string
}

// Store is the lan_devices CRUD surface.
type Store struct {
	db             *sql.DB
	missThreshold  int
}

// New returns a Store using the default miss threshold.
func New(db *sql.DB) *Store { return NewWithThreshold(db, DefaultMissThreshold) }

// NewWithThreshold lets tests / configuration override the offline threshold.
func NewWithThreshold(db *sql.DB, threshold int) *Store {
	if threshold < 1 {
		threshold = 1
	}
	return &Store{db: db, missThreshold: threshold}
}

// MissThreshold returns the configured offline threshold.
func (s *Store) MissThreshold() int { return s.missThreshold }

// ApplySweep is the single write path for a sweep cycle. It applies all
// observations in one transaction:
//
//   - Each Observation upserts the corresponding row: existing rows have
//     ip/vendor refreshed, last_seen advanced, online=1, missed_sweeps=0;
//     first_seen is preserved (FR-034 AC-002, AC-003).
//   - Every row whose MAC is NOT in the observation set has its
//     missed_sweeps incremented; if it crosses the configured threshold,
//     online flips to 0 (FR-035 AC-002). last_seen freezes by virtue of not
//     being touched.
func (s *Store) ApplySweep(now time.Time, observations []Observation) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin sweep tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	nowMS := now.UnixMilli()

	seen := make(map[string]struct{}, len(observations))
	for _, obs := range observations {
		if obs.MAC == "" {
			continue
		}
		seen[obs.MAC] = struct{}{}
		// Upsert: insert if new (preserving first_seen=now), refresh otherwise.
		if _, err := tx.Exec(
			`INSERT INTO lan_devices(mac, ip, vendor, first_seen, last_seen, online, missed_sweeps)
			 VALUES(?, ?, ?, ?, ?, 1, 0)
			 ON CONFLICT(mac) DO UPDATE SET
			   ip = excluded.ip,
			   vendor = excluded.vendor,
			   last_seen = excluded.last_seen,
			   online = 1,
			   missed_sweeps = 0`,
			obs.MAC, obs.IP, obs.Vendor, nowMS, nowMS,
		); err != nil {
			return fmt.Errorf("upsert observation %s: %w", obs.MAC, err)
		}
	}

	// Increment missed_sweeps for rows not in the observation set, and flip
	// online to 0 once the count crosses the threshold.
	if len(seen) == 0 {
		// No observations this cycle — every existing row is a miss.
		if _, err := tx.Exec(
			`UPDATE lan_devices
			 SET missed_sweeps = missed_sweeps + 1,
			     online = CASE WHEN missed_sweeps + 1 >= ? THEN 0 ELSE online END`,
			s.missThreshold,
		); err != nil {
			return fmt.Errorf("apply misses (empty): %w", err)
		}
	} else {
		// Build a "NOT IN (?,?,?)" clause inline. Bound by /24 size so safe.
		args := make([]interface{}, 0, len(seen)+1)
		args = append(args, s.missThreshold)
		placeholders := ""
		for mac := range seen {
			if placeholders != "" {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, mac)
		}
		query := fmt.Sprintf(
			`UPDATE lan_devices
			 SET missed_sweeps = missed_sweeps + 1,
			     online = CASE WHEN missed_sweeps + 1 >= ? THEN 0 ELSE online END
			 WHERE mac NOT IN (%s)`, placeholders,
		)
		if _, err := tx.Exec(query, args...); err != nil {
			return fmt.Errorf("apply misses: %w", err)
		}
	}

	return tx.Commit()
}

// List returns the device list ordered for the API: online first, then by
// last_seen DESC (matches the index on (online DESC, last_seen DESC)).
func (s *Store) List() ([]Device, error) {
	rows, err := s.db.Query(
		`SELECT mac, ip, vendor, first_seen, last_seen, online, missed_sweeps
		 FROM lan_devices
		 ORDER BY online DESC, last_seen DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list lan_devices: %w", err)
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var firstMS, lastMS int64
		var onlineInt int
		if err := rows.Scan(&d.MAC, &d.IP, &d.Vendor, &firstMS, &lastMS, &onlineInt, &d.MissedSweeps); err != nil {
			return nil, err
		}
		d.FirstSeen = time.UnixMilli(firstMS).UTC()
		d.LastSeen = time.UnixMilli(lastMS).UTC()
		d.Online = onlineInt == 1
		out = append(out, d)
	}
	return out, rows.Err()
}

// Get fetches a single row by MAC. Returns sql.ErrNoRows if absent.
func (s *Store) Get(mac string) (Device, error) {
	var d Device
	var firstMS, lastMS int64
	var onlineInt int
	err := s.db.QueryRow(
		`SELECT mac, ip, vendor, first_seen, last_seen, online, missed_sweeps
		 FROM lan_devices WHERE mac = ?`, mac,
	).Scan(&d.MAC, &d.IP, &d.Vendor, &firstMS, &lastMS, &onlineInt, &d.MissedSweeps)
	if err != nil {
		return Device{}, err
	}
	d.FirstSeen = time.UnixMilli(firstMS).UTC()
	d.LastSeen = time.UnixMilli(lastMS).UTC()
	d.Online = onlineInt == 1
	return d, nil
}

// Count returns the number of rows in lan_devices.
func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM lan_devices`).Scan(&n)
	return n, err
}
