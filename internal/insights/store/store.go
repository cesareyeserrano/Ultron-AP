// Package store owns all reads and writes against the rules and rule_state
// SQLite tables. It seeds the rules table from the bundled set on first boot,
// preserves the operator-mutable enabled flag across upgrades, and exposes a
// small CRUD surface used by the engine.
//
// FR-045 persistence semantics:
//   - First startup with a fresh DB: every bundled rule is inserted with
//     enabled=1 and source='bundled'.
//   - Subsequent startups: bundled rules are upserted (definition refreshed,
//     enabled flag preserved) while orphan rows (rules removed in a later
//     build) remain in the table with their state untouched.
//   - rule_state rows are bounded by rule count (NFR-017).
//
// @aitri-trace FR-045 NFR-017 US-045 TC-IE-007h TC-IE-007f TC-IE-007e
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Severity is the wire string for the severity column.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityCritical Severity = "critical"
)

// Rule is the read model returned by Load. Its shape is the on-disk schema —
// the engine compiles ConditionJSON via the lang package separately.
type Rule struct {
	ID             string
	Title          string
	ConditionJSON  json.RawMessage
	Severity       Severity
	Verdict        string
	Recommendation string
	Links          []string
	Enabled        bool
	Source         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// State is the per-rule hysteresis and timing state persisted between ticks.
// FR-046 transition counters live here; FR-042 first_emitted_at is per-rule
// in-memory state managed by the engine (not persisted).
type State struct {
	RuleID              string
	LastEvaluatedAt     time.Time
	LastValue           bool
	LastChangeAt        time.Time
	TransitionsInWindow int
}

// Store wraps a parent *sql.DB. It does not own the DB lifecycle.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by db.
func New(db *sql.DB) *Store { return &Store{db: db} }

// SeedRule inserts or updates a bundled rule definition. The enabled flag is
// preserved on upsert so an operator-disabled rule stays disabled after a
// binary upgrade refreshes the rule definitions.
func (s *Store) SeedRule(r Rule) error {
	if r.ID == "" {
		return fmt.Errorf("seed: empty rule id")
	}
	links := r.Links
	if links == nil {
		links = []string{}
	}
	linksJSON, err := json.Marshal(links)
	if err != nil {
		return fmt.Errorf("seed %s: marshal links: %w", r.ID, err)
	}
	now := time.Now().UnixMilli()
	src := r.Source
	if src == "" {
		src = "bundled"
	}
	_, err = s.db.Exec(
		`INSERT INTO rules(id, title, condition_json, severity, verdict, recommendation,
		                    links_json, enabled, source, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title          = excluded.title,
		   condition_json = excluded.condition_json,
		   severity       = excluded.severity,
		   verdict        = excluded.verdict,
		   recommendation = excluded.recommendation,
		   links_json     = excluded.links_json,
		   source         = excluded.source,
		   updated_at     = excluded.updated_at`,
		r.ID, r.Title, string(r.ConditionJSON), string(r.Severity), r.Verdict,
		r.Recommendation, string(linksJSON), src, now, now,
	)
	if err != nil {
		return fmt.Errorf("seed %s: %w", r.ID, err)
	}
	return nil
}

// LoadAll returns every rule row.
func (s *Store) LoadAll() ([]Rule, error) {
	rows, err := s.db.Query(
		`SELECT id, title, condition_json, severity, verdict, recommendation,
		        links_json, enabled, source, created_at, updated_at
		 FROM rules
		 ORDER BY
		   CASE severity WHEN 'critical' THEN 0 WHEN 'warn' THEN 1 ELSE 2 END,
		   id`,
	)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var r Rule
		var condStr, linksStr, sev, src string
		var enabledInt int
		var createdMS, updatedMS int64
		if err := rows.Scan(
			&r.ID, &r.Title, &condStr, &sev, &r.Verdict, &r.Recommendation,
			&linksStr, &enabledInt, &src, &createdMS, &updatedMS,
		); err != nil {
			return nil, err
		}
		r.ConditionJSON = json.RawMessage(condStr)
		r.Severity = Severity(sev)
		var links []string
		if err := json.Unmarshal([]byte(linksStr), &links); err != nil || links == nil {
			links = []string{}
		}
		r.Links = links
		r.Enabled = enabledInt == 1
		r.Source = src
		r.CreatedAt = time.UnixMilli(createdMS).UTC()
		r.UpdatedAt = time.UnixMilli(updatedMS).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// LoadEnabled returns only the rules where enabled=1.
func (s *Store) LoadEnabled() ([]Rule, error) {
	all, err := s.LoadAll()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, r := range all {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

// SetEnabled flips the enabled flag for one rule. Returns sql.ErrNoRows if
// the rule does not exist. The function does not delete rule rows even if
// enabled is set to false — disabling is reversible (FR-045 / TC-IE-007f).
func (s *Store) SetEnabled(id string, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	res, err := s.db.Exec(
		`UPDATE rules SET enabled = ?, updated_at = ? WHERE id = ?`,
		val, time.Now().UnixMilli(), id,
	)
	if err != nil {
		return fmt.Errorf("setenabled %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// LoadState returns every rule_state row keyed by rule_id. Missing rows are
// initialised lazily by PersistState; callers should treat absence as "fresh".
func (s *Store) LoadState() (map[string]State, error) {
	rows, err := s.db.Query(
		`SELECT rule_id, last_evaluated_at, last_value, last_change_at, transitions_in_window
		 FROM rule_state`,
	)
	if err != nil {
		return nil, fmt.Errorf("load rule_state: %w", err)
	}
	defer rows.Close()
	out := map[string]State{}
	for rows.Next() {
		var st State
		var evalMS, changeMS int64
		var lastVal int
		if err := rows.Scan(&st.RuleID, &evalMS, &lastVal, &changeMS, &st.TransitionsInWindow); err != nil {
			return nil, err
		}
		st.LastEvaluatedAt = time.UnixMilli(evalMS).UTC()
		st.LastChangeAt = time.UnixMilli(changeMS).UTC()
		st.LastValue = lastVal == 1
		out[st.RuleID] = st
	}
	return out, rows.Err()
}

// PersistState upserts the rule_state row for one rule. The engine debounces
// these writes (NFR-017): per-tick state lives in memory; periodic flushes
// keep the table bounded.
func (s *Store) PersistState(st State) error {
	v := 0
	if st.LastValue {
		v = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO rule_state(rule_id, last_evaluated_at, last_value, last_change_at, transitions_in_window)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(rule_id) DO UPDATE SET
		   last_evaluated_at     = excluded.last_evaluated_at,
		   last_value            = excluded.last_value,
		   last_change_at        = excluded.last_change_at,
		   transitions_in_window = excluded.transitions_in_window`,
		st.RuleID, st.LastEvaluatedAt.UnixMilli(), v,
		st.LastChangeAt.UnixMilli(), st.TransitionsInWindow,
	)
	if err != nil {
		return fmt.Errorf("persist rule_state %s: %w", st.RuleID, err)
	}
	return nil
}

// CountRules returns the number of rule rows (used by the migration test).
func (s *Store) CountRules() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM rules`).Scan(&n)
	return n, err
}

// CountEnabled returns the number of rules with enabled=1.
func (s *Store) CountEnabled() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM rules WHERE enabled = 1`).Scan(&n)
	return n, err
}
