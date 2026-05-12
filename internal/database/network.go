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

// RecentNetSamplesByKind returns up to limit most-recent samples for a probe
// kind, newest first. It is used by alert rules that aggregate DNS resolver
// health across all configured resolver targets.
//
// @aitri-trace FR-ID: FR-073, US-ID: US-073, AC-ID: AC-073-001, TC-ID: TC-NA-073h
func (db *DB) RecentNetSamplesByKind(kind string, limit int) ([]NetSample, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(
		`SELECT id, ts, target, kind, rtt_ms, status
		 FROM NetSample WHERE kind = ? ORDER BY ts DESC LIMIT ?`,
		kind, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent net samples by kind: %w", err)
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

// NetEvent is one structured network event row (e.g. WAN outage transitions,
// path changes, public IP changes — for now only WAN up/down).
type NetEvent struct {
	ID     int64
	TS     time.Time
	Kind   string // "wan_down" | "wan_up"
	Detail string
}

// InsertNetEvent appends an event row.
func (db *DB) InsertNetEvent(e NetEvent) error {
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	_, err := db.Exec(
		`INSERT INTO NetEvent (ts, kind, detail) VALUES (?, ?, ?)`,
		e.TS.UnixMilli(), e.Kind, e.Detail,
	)
	if err != nil {
		return fmt.Errorf("insert net event: %w", err)
	}
	return nil
}

// RecentNetEvents returns up to limit most-recent events, newest first.
func (db *DB) RecentNetEvents(limit int) ([]NetEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(
		`SELECT id, ts, kind, detail FROM NetEvent ORDER BY ts DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent net events: %w", err)
	}
	defer rows.Close()
	var out []NetEvent
	for rows.Next() {
		var e NetEvent
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Kind, &e.Detail); err != nil {
			return nil, err
		}
		e.TS = time.UnixMilli(ts)
		out = append(out, e)
	}
	return out, rows.Err()
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
