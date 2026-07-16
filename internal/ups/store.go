// Module:       internal/ups
// Purpose:      SQLite persistence for UPS samples + outage events (FR-019/FR-020).
// Dependencies: database/sql (tables created by internal/database migrations).
package ups

import (
	"database/sql"
	"fmt"
	"time"
)

// Store persists UPS samples and outage events. The tables (ups_samples,
// ups_events) are created by the internal/database schema; this store only
// reads and writes them, mirroring the network module's persistence.
type Store struct {
	db *sql.DB
}

// NewStore wraps the shared database handle.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Sample is one persisted UPS observation.
type Sample struct {
	TS         time.Time
	Status     string
	State      State
	LoadPct    *float64
	InputV     *float64
	InputFreq  *float64
	BatteryV   *float64
	BattPctEst *float64
}

// OutageEvent is a mains-outage record: opened on the transition to on-battery,
// closed on the return to mains.
type OutageEvent struct {
	ID        int64
	Start     time.Time
	End       *time.Time // nil while open
	DurationS *int64     // nil while open
	Kind      string
}

// WriteSample persists one snapshot as a sample row, timestamped at the
// snapshot's last-good time (or now if unset).
// @aitri-trace FR-019 US-019 AC-019-001 TC-UPS-013h
func (s *Store) WriteSample(snap Snapshot) error {
	ts := snap.LastGood
	if ts.IsZero() {
		ts = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO ups_samples (ts, status, state, load_pct, input_v, input_freq, battery_v, batt_pct_est)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ts.UnixMilli(), snap.RawStatus, string(snap.State),
		nullF(snap.LoadPct), nullF(snap.InputV), nullF(snap.InputHz), nullF(snap.BatteryV), nullF(snap.BattPctEst),
	)
	if err != nil {
		return fmt.Errorf("ups: insert sample: %w", err)
	}
	return nil
}

// Series returns the samples with ts in [from, to], oldest first — the data
// behind the 24h/7d charts. An empty range returns an empty slice and nil error
// (the chart shows an empty state, not an error).
func (s *Store) Series(from, to time.Time) ([]Sample, error) {
	rows, err := s.db.Query(
		`SELECT ts, status, state, load_pct, input_v, input_freq, battery_v, batt_pct_est
		 FROM ups_samples WHERE ts >= ? AND ts <= ? ORDER BY ts ASC`,
		from.UnixMilli(), to.UnixMilli(),
	)
	if err != nil {
		return nil, fmt.Errorf("ups: query series: %w", err)
	}
	defer rows.Close()
	out := []Sample{}
	for rows.Next() {
		var (
			ts                           int64
			status, state                string
			load, input, freq, batt, pct sql.NullFloat64
		)
		if err := rows.Scan(&ts, &status, &state, &load, &input, &freq, &batt, &pct); err != nil {
			return nil, fmt.Errorf("ups: scan sample: %w", err)
		}
		out = append(out, Sample{
			TS: time.UnixMilli(ts), Status: status, State: State(state),
			LoadPct: fromNull(load), InputV: fromNull(input), InputFreq: fromNull(freq),
			BatteryV: fromNull(batt), BattPctEst: fromNull(pct),
		})
	}
	return out, rows.Err()
}

// PruneSamples deletes samples older than the retention window (days) and
// returns the number of rows removed.
func (s *Store) PruneSamples(days int) (int64, error) {
	if days <= 0 {
		days = defaultRetentionDays
	}
	cutoff := time.Now().AddDate(0, 0, -days).UnixMilli()
	res, err := s.db.Exec(`DELETE FROM ups_samples WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("ups: prune samples: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// PruneEvents deletes closed outage events older than the retention window.
// Open events are never pruned.
func (s *Store) PruneEvents(days int) (int64, error) {
	if days <= 0 {
		days = defaultRetentionDays
	}
	cutoff := time.Now().AddDate(0, 0, -days).UnixMilli()
	res, err := s.db.Exec(`DELETE FROM ups_events WHERE end_ts IS NOT NULL AND end_ts < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("ups: prune events: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// HasOpenEvent reports whether an outage event is currently open.
func (s *Store) HasOpenEvent() (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ups_events WHERE end_ts IS NULL`).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("ups: count open events: %w", err)
	}
	return n > 0, nil
}

// OpenEvent opens a new outage event at ts. It is idempotent: if an outage is
// already open, it does nothing (so a restart mid-outage cannot double-count —
// FR-020 AC-020-003).
// @aitri-trace FR-020 US-020 AC-020-001 TC-UPS-016h
func (s *Store) OpenEvent(ts time.Time) error {
	open, err := s.HasOpenEvent()
	if err != nil {
		return err
	}
	if open {
		return nil
	}
	_, err = s.db.Exec(
		`INSERT INTO ups_events (start_ts, kind) VALUES (?, 'outage')`,
		ts.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("ups: open event: %w", err)
	}
	return nil
}

// CloseOpenEvent closes the currently-open outage event at ts, setting its end
// timestamp and duration. Returns the outage duration; a zero duration and nil
// error if no event was open.
func (s *Store) CloseOpenEvent(ts time.Time) (time.Duration, error) {
	var id, startMs int64
	err := s.db.QueryRow(
		`SELECT id, start_ts FROM ups_events WHERE end_ts IS NULL ORDER BY start_ts ASC LIMIT 1`,
	).Scan(&id, &startMs)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("ups: find open event: %w", err)
	}
	dur := ts.Sub(time.UnixMilli(startMs))
	if dur < 0 {
		dur = 0
	}
	_, err = s.db.Exec(
		`UPDATE ups_events SET end_ts = ?, duration_s = ? WHERE id = ?`,
		ts.UnixMilli(), int64(dur.Seconds()), id,
	)
	if err != nil {
		return 0, fmt.Errorf("ups: close event: %w", err)
	}
	return dur, nil
}

// ReconcileOpenOnBoot is called at startup. It leaves any open outage event
// open (it is a genuine ongoing outage) but returns whether one exists so the
// poller can seed its state and not open a duplicate on the next poll.
func (s *Store) ReconcileOpenOnBoot() (bool, error) {
	return s.HasOpenEvent()
}

// LastOnlineSince returns the moment mains was last restored (end of the most
// recent closed outage), falling back to the first recorded sample when no
// outage was ever recorded. Zero time when there is no history at all.
func (s *Store) LastOnlineSince() (time.Time, error) {
	var ms sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(end_ts) FROM ups_events`).Scan(&ms); err != nil {
		return time.Time{}, fmt.Errorf("ups: last restore: %w", err)
	}
	if ms.Valid {
		return time.UnixMilli(ms.Int64), nil
	}
	if err := s.db.QueryRow(`SELECT MIN(ts) FROM ups_samples`).Scan(&ms); err != nil {
		return time.Time{}, fmt.Errorf("ups: first sample: %w", err)
	}
	if ms.Valid {
		return time.UnixMilli(ms.Int64), nil
	}
	return time.Time{}, nil
}

// RecentEvents returns up to limit most-recent outage events, newest first.
func (s *Store) RecentEvents(limit int) ([]OutageEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, start_ts, end_ts, duration_s, kind FROM ups_events ORDER BY start_ts DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ups: query events: %w", err)
	}
	defer rows.Close()
	out := []OutageEvent{}
	for rows.Next() {
		var (
			e       OutageEvent
			startMs int64
			endMs   sql.NullInt64
			dur     sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &startMs, &endMs, &dur, &e.Kind); err != nil {
			return nil, fmt.Errorf("ups: scan event: %w", err)
		}
		e.Start = time.UnixMilli(startMs)
		if endMs.Valid {
			t := time.UnixMilli(endMs.Int64)
			e.End = &t
		}
		if dur.Valid {
			d := dur.Int64
			e.DurationS = &d
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountEvents returns the total number of outage event rows (test/insights helper).
func (s *Store) CountEvents() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ups_events`).Scan(&n)
	return n, err
}

func nullF(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func fromNull(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}
