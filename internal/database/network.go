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

// netPruneBatch bounds how many rows a single DELETE may remove.
//
// It is a constant, never configurable: the environment may influence the
// PARAMETER of the statement (the cutoff) but never its shape (NFR-102).
const netPruneBatch = 50000

// PruneNetSamples deletes NetSample rows older than days, in bounded batches.
// Returns the total rows removed.
//
// Batching is the point. The first run in production has to remove ~6.3M rows
// from a 719 MB database on a Raspberry Pi, and a single DELETE of that size
// holds one transaction open for the whole delete, inflates the WAL, and blocks
// writers — and the writer here is the network probe, inserting every few
// seconds. With bounded batches each transaction lasts tens of milliseconds and
// the probe finds the database free in between (NFR-111).
//
// The cutoff is computed ONCE, before the loop: recomputing it per batch would
// walk the boundary forward while the delete is in progress.
//
// SQLite's DELETE ... LIMIT needs SQLITE_ENABLE_UPDATE_DELETE_LIMIT at compile
// time and the Go driver does not guarantee it, so the bound is expressed as a
// subquery over the primary key instead (ADR-003).
//
// Params:
//   - days: retention window; the caller is responsible for it being >= 1,
//     which config.Load guarantees.
//
// Returns rows removed so far and the first error, if any — a partial delete is
// valid work and the next daily pass continues from where it stopped.
//
// @aitri-trace FR-097, FR-098, AC-098-001, AC-098-002, TC-NSR-020h, TC-NSR-021e
func (db *DB) PruneNetSamples(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).UnixMilli()

	var total int64
	for {
		res, err := db.Exec(
			`DELETE FROM NetSample WHERE id IN (
			     SELECT id FROM NetSample WHERE ts < ? LIMIT ?
			 )`, cutoff, netPruneBatch)
		if err != nil {
			return total, fmt.Errorf("prune net samples: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("prune net samples: rows affected: %w", err)
		}
		total += n
		// A batch that removed nothing means there is nothing left in range.
		// This is what terminates the loop; without it a non-advancing query
		// would spin forever inside the retention job (TC-NSR-022f).
		if n == 0 {
			return total, nil
		}
	}
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
