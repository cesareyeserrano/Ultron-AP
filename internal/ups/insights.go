// Module:       internal/ups
// Purpose:      UPS-derived insights: outage counts + battery degradation (FR-022).
// Dependencies: database/sql (via Store).
//
// Design note (build-time correction): the bundled insights engine is locked at
// exactly 10 rules by the parent FR-047, and it emits STATIC verdict text (it
// cannot interpolate a value — adversarial finding #3). Rather than break the
// FR-047 contract or ship a number-less verdict, UPS insights are produced here
// as a dedicated producer, which lets them render the REAL count/drop. Declared
// as technical_debt vs the "feed the bundled engine via EvalWithVars" design.
package ups

import (
	"fmt"
	"time"
)

// minRestingSamples is the history required before claiming battery degradation
// — a single sample (or a handful) is never enough to assert a trend (FR-022
// AC-022-002).
const minRestingSamples = 10

// Insight is one human-readable UPS observation for the dashboard.
type Insight struct {
	Severity Severity
	Title    string
	Text     string
}

// InsightVars returns the raw UPS insight signals. Keys:
//   - ups_outages_7d: closed outage events in the last 7 days
//   - ups_batt_drop_v: resting battery-voltage drop across history (only present
//     when there is enough history to make the claim honestly)
func (s *Store) InsightVars(now time.Time) (map[string]float64, error) {
	vars := map[string]float64{}

	var outages int
	weekAgo := now.AddDate(0, 0, -7).UnixMilli()
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ups_events WHERE end_ts IS NOT NULL AND start_ts >= ?`, weekAgo,
	).Scan(&outages); err != nil {
		return nil, fmt.Errorf("ups: count weekly outages: %w", err)
	}
	vars["ups_outages_7d"] = float64(outages)

	if drop, ok, err := s.restingVoltageDrop(now); err != nil {
		return nil, err
	} else if ok {
		vars["ups_batt_drop_v"] = drop
	}
	return vars, nil
}

// Insights turns the raw signals into rendered observations with real numbers.
// @aitri-trace FR-022 US-022 AC-022-001 TC-UPS-024h
func (s *Store) Insights(now time.Time) ([]Insight, error) {
	vars, err := s.InsightVars(now)
	if err != nil {
		return nil, err
	}
	out := []Insight{}
	if n := int(vars["ups_outages_7d"]); n >= 1 {
		noun := "corte"
		if n != 1 {
			noun = "cortes"
		}
		out = append(out, Insight{
			Severity: SevInfo,
			Title:    "Cortes de red esta semana",
			Text:     fmt.Sprintf("La red eléctrica tuvo %d %s esta semana.", n, noun),
		})
	}
	if drop, ok := vars["ups_batt_drop_v"]; ok && drop >= 0.5 {
		out = append(out, Insight{
			Severity: SevWarning,
			Title:    "Batería degradándose",
			Text:     fmt.Sprintf("El voltaje de batería en reposo bajó %.1f V — la batería podría estar degradándose.", drop),
		})
	}
	return out, nil
}

// restingVoltageDrop computes the drop in resting (on-mains) battery voltage
// between the earliest and latest history. It reports ok=false when there is
// insufficient history to make the claim, so a single sample never yields a
// degradation insight (FR-022 AC-022-002).
//
// Bounded by design: it counts resting samples, then reads only the first and
// last decile (LIMIT) rather than loading the whole series — cheap even at
// 30-day retention, and it uses the idx_ups_samples_state_ts index.
func (s *Store) restingVoltageDrop(now time.Time) (float64, bool, error) {
	var n int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ups_samples WHERE state = ? AND battery_v IS NOT NULL`,
		string(StateOnline),
	).Scan(&n); err != nil {
		return 0, false, fmt.Errorf("ups: count resting voltages: %w", err)
	}
	if n < minRestingSamples {
		return 0, false, nil // not enough history to claim a trend
	}
	k := n / 10
	if k < 1 {
		k = 1
	}
	early, err := s.avgRestingV(k, "ASC")
	if err != nil {
		return 0, false, err
	}
	late, err := s.avgRestingV(k, "DESC")
	if err != nil {
		return 0, false, err
	}
	return early - late, true, nil
}

// avgRestingV averages the battery voltage of the first (ASC) or last (DESC)
// `limit` resting samples. order is a trusted internal constant, never input.
func (s *Store) avgRestingV(limit int, order string) (float64, error) {
	rows, err := s.db.Query(
		//nolint:gosec // order is a hardcoded "ASC"/"DESC" constant, not user input
		fmt.Sprintf(`SELECT battery_v FROM ups_samples WHERE state = ? AND battery_v IS NOT NULL ORDER BY ts %s LIMIT ?`, order),
		string(StateOnline), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("ups: query resting voltages: %w", err)
	}
	defer rows.Close()
	var vs []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return 0, err
		}
		vs = append(vs, v)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(vs) == 0 {
		return 0, nil
	}
	return avg(vs), nil
}

func avg(vs []float64) float64 {
	var sum float64
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}
