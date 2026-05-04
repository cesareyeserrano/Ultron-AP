package database

import (
	"database/sql"
	"fmt"
	"time"
)

// NetSample is one observation from a network probe (FR-021 minimum slice —
// no per-target catalog yet, target is stored as a free-form identifier such
// as the gateway IP).
type NetSample struct {
	ID     int64
	TS     time.Time
	Target string
	Kind   string   // "icmp" | "dns" | future: "udp"
	RTTMs  *float64 // nil on failure
	Status string   // "ok" | "timeout" | "error" | "no-gateway" | future: "servfail" | "nxdomain"
}

// InsertNetSample appends one sample row. Idempotent only at the row-level
// (each call inserts a new row). Failure to insert is the caller's concern —
// the probe must keep running.
func (db *DB) InsertNetSample(s NetSample) error {
	var rtt sql.NullFloat64
	if s.RTTMs != nil {
		rtt = sql.NullFloat64{Float64: *s.RTTMs, Valid: true}
	}
	if s.TS.IsZero() {
		s.TS = time.Now()
	}
	_, err := db.Exec(
		`INSERT INTO NetSample (ts, target, kind, rtt_ms, status) VALUES (?, ?, ?, ?, ?)`,
		s.TS.UnixMilli(), s.Target, s.Kind, rtt, s.Status,
	)
	if err != nil {
		return fmt.Errorf("insert net sample: %w", err)
	}
	return nil
}

// RecentNetSamples returns up to limit most-recent samples for a target,
// newest first. Used by the network dashboard chart (BG-014, future).
func (db *DB) RecentNetSamples(target string, limit int) ([]NetSample, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(
		`SELECT id, ts, target, kind, rtt_ms, status
		 FROM NetSample WHERE target = ? ORDER BY ts DESC LIMIT ?`,
		target, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent net samples: %w", err)
	}
	return scanNetSamples(rows)
}

// PruneNetSamples deletes NetSample rows older than days. Returns rows removed.
func (db *DB) PruneNetSamples(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).UnixMilli()
	res, err := db.Exec(`DELETE FROM NetSample WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune net samples: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func scanNetSamples(rows *sql.Rows) ([]NetSample, error) {
	defer rows.Close()
	var out []NetSample
	for rows.Next() {
		var s NetSample
		var ts int64
		var rtt sql.NullFloat64
		if err := rows.Scan(&s.ID, &ts, &s.Target, &s.Kind, &rtt, &s.Status); err != nil {
			return nil, err
		}
		s.TS = time.UnixMilli(ts)
		if rtt.Valid {
			v := rtt.Float64
			s.RTTMs = &v
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
