package alerts

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/stretchr/testify/require"
)

// Feature: sustained-alert-window-fix — span-based, jitter-tolerant sustained
// breach confirmation in sustainedWindow.add. See features/sustained-alert-window-fix.

// swBase is the fixed clock origin for all timestamps in these tests.
var swBase = time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)

// swAt returns swBase + offsetSeconds.
func swAt(offsetSec int) time.Time {
	return swBase.Add(time.Duration(offsetSec) * time.Second)
}

// ---- FR-016: jitter-tolerant span-based confirmation -------------------------

// @aitri-tc TC-SAW-016h
func TestTC_SAW_016h(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	// Jittered breaching samples; every gap <= 2*interval (20s) so no reset.
	offsets := []int{0, 9, 21, 29, 38, 47, 58, 63}
	for _, o := range offsets[:len(offsets)-1] {
		require.Falsef(t, w.add(1, swAt(o), true), "span %ds < 60s must not confirm", o)
	}
	// First sample whose span (63s) reaches the 60s duration confirms.
	require.True(t, w.add(1, swAt(63), true), "span 63s >= 60s must confirm under jitter")
}

// @aitri-tc TC-SAW-016e
func TestTC_SAW_016e(t *testing.T) {
	// Knife-edge: the offset-60 sample is missing (jitter); the confirming
	// sample lands at 63s. The previous cutoff-equality code never confirmed
	// this because it trimmed the run-start sample.
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50, 57} {
		require.Falsef(t, w.add(1, swAt(o), true), "span %ds < 60s must not confirm", o)
	}
	require.True(t, w.add(1, swAt(63), true), "span 63s must confirm even with the 60s sample missing")
}

// @aitri-tc TC-SAW-016b
func TestTC_SAW_016b(t *testing.T) {
	// Boundary: a single sample (span 0) never confirms; span exactly D does.
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	require.False(t, w.add(1, swAt(0), true), "single sample (span 0) must not confirm")
	for _, o := range []int{10, 20, 30, 40, 50} {
		require.Falsef(t, w.add(1, swAt(o), true), "span %ds < 60s must not confirm", o)
	}
	require.True(t, w.add(1, swAt(60), true), "span exactly 60s must confirm (>= is inclusive)")
}

// @aitri-tc TC-SAW-016f
func TestTC_SAW_016f(t *testing.T) {
	// Negative: span stays below D, so the window must never confirm early.
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50} {
		require.Falsef(t, w.add(1, swAt(o), true), "span %ds < 60s must not confirm", o)
	}
}

// ---- FR-017: bounded retention ----------------------------------------------

// @aitri-tc TC-SAW-017h
func TestTC_SAW_017h(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	// Establish a run so subsequent adds overwrite the boundary sample.
	w.add(1, swAt(0), true)
	w.add(1, swAt(10), true)
	i := 1
	allocs := testing.AllocsPerRun(1000, func() {
		i++
		w.add(1, swAt(i*10), true)
	})
	require.Equal(t, 0.0, allocs, "an established sustained breach must allocate nothing per add")
}

// @aitri-tc TC-SAW-017e
func TestTC_SAW_017e(t *testing.T) {
	// A breach lasting 100x the duration must not grow the retained state.
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for k := 0; k < 600; k++ { // 600 * 10s = 6000s = 100 * 60s
		w.add(1, swAt(k*10), true)
	}
	require.LessOrEqual(t, len(w.samples), 2, "retained samples must stay bounded across a long breach")
}

// @aitri-tc TC-SAW-017f
func TestTC_SAW_017f(t *testing.T) {
	// Across a very long breach, retention must never drop the run-start, so
	// confirmation keeps firing from the moment the span reaches D.
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for k := 0; k < 10000; k++ {
		offset := k * 10
		got := w.add(1, swAt(offset), true)
		if offset >= 60 {
			require.Truef(t, got, "span %ds >= 60s must confirm", offset)
		} else {
			require.Falsef(t, got, "span %ds < 60s must not confirm", offset)
		}
	}
}

// ---- NFR-005: non-breach resets the window ----------------------------------

// @aitri-tc TC-SAW-005h
func TestTC_SAW_005h(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50} {
		require.False(t, w.add(1, swAt(o), true))
	}
	require.False(t, w.add(1, swAt(55), false), "non-breach must return false")
	// Fresh run measured from offset 60, sampled every 10s (no gap reset).
	for _, o := range []int{60, 70, 80, 90, 100, 110} {
		require.Falsef(t, w.add(1, swAt(o), true), "fresh run span %ds < 60s", o-60)
	}
	require.True(t, w.add(1, swAt(120), true), "fresh run span 60s confirms")
}

// @aitri-tc TC-SAW-005e
func TestTC_SAW_005e(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	require.False(t, w.add(1, swAt(0), false), "non-breach on an empty window is a safe no-op")
	require.Len(t, w.samples, 0)
}

// @aitri-tc TC-SAW-005f
func TestTC_SAW_005f(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50} {
		require.False(t, w.add(1, swAt(o), true))
	}
	require.False(t, w.add(1, swAt(55), false), "reset")
	// After reset the run starts at 60; offset 65 has span 5s, NOT 65s. A
	// failure to reset would (wrongly) confirm here.
	require.False(t, w.add(1, swAt(60), true), "fresh run span 0")
	require.False(t, w.add(1, swAt(65), true), "span measured from 60 (5s), not from stale 0 (65s)")
}

// ---- NFR-006: gap > 2*interval resets the window ----------------------------

// @aitri-tc TC-SAW-006h
func TestTC_SAW_006h(t *testing.T) {
	// A gap of exactly 2*interval (20s) does NOT reset the run.
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40} {
		require.False(t, w.add(1, swAt(o), true))
	}
	require.True(t, w.add(1, swAt(60), true), "gap 20s == 2*interval keeps the run; span 60s confirms")
}

// @aitri-tc TC-SAW-006e
func TestTC_SAW_006e(t *testing.T) {
	// A gap just over 2*interval (21s) resets the run.
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50} {
		require.False(t, w.add(1, swAt(o), true))
	}
	require.False(t, w.add(1, swAt(71), true), "gap 21s > 2*interval resets; new run span 0")
}

// @aitri-tc TC-SAW-006f
func TestTC_SAW_006f(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50} {
		require.False(t, w.add(1, swAt(o), true))
	}
	require.False(t, w.add(1, swAt(75), true), "gap 25s > 20s resets the run")
	// Fresh run from offset 75, sampled every 10s (no further gap reset).
	for _, o := range []int{85, 95, 105, 115, 125} {
		require.Falsef(t, w.add(1, swAt(o), true), "fresh run span %ds < 60s", o-75)
	}
	require.True(t, w.add(1, swAt(135), true), "fresh run span 60s confirms (not from stale pre-gap samples)")
}

// ---- NFR-007: duration <= 0 short-circuits ----------------------------------

// @aitri-tc TC-SAW-007h
func TestTC_SAW_007h(t *testing.T) {
	w := &sustainedWindow{duration: 0, interval: 10 * time.Second}
	require.True(t, w.add(1, swAt(0), true), "duration 0 returns the breaching flag verbatim")
	require.Len(t, w.samples, 0, "short-circuit must not mutate state")
}

// @aitri-tc TC-SAW-007e
func TestTC_SAW_007e(t *testing.T) {
	w := &sustainedWindow{duration: -5 * time.Second, interval: 10 * time.Second}
	require.True(t, w.add(1, swAt(0), true), "negative duration mirrors breaching=true")
	require.False(t, w.add(1, swAt(10), false), "negative duration mirrors breaching=false")
}

// @aitri-tc TC-SAW-007f
func TestTC_SAW_007f(t *testing.T) {
	w := &sustainedWindow{duration: 0, interval: 10 * time.Second}
	require.False(t, w.add(1, swAt(0), false), "duration 0 returns breaching=false verbatim")
}

// ---- NFR-008: interval-aligned confirmation preserved (TC-NA-076e style) -----

// @aitri-tc TC-SAW-008h
func TestTC_SAW_008h(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50} {
		require.False(t, w.add(1, swAt(o), true))
	}
	require.True(t, w.add(1, swAt(60), true), "aligned confirming sample exactly D after first confirms")
}

// @aitri-tc TC-SAW-008e
func TestTC_SAW_008e(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50, 60} {
		w.add(1, swAt(o), true)
	}
	require.True(t, w.add(1, swAt(70), true), "confirmation stays true on the sample after D")
}

// @aitri-tc TC-SAW-008f
func TestTC_SAW_008f(t *testing.T) {
	w := &sustainedWindow{duration: 60 * time.Second, interval: 10 * time.Second}
	for _, o := range []int{0, 10, 20, 30, 40, 50} {
		require.Falsef(t, w.add(1, swAt(o), true), "aligned but span %ds < 60s", o)
	}
}

// ---- NFR-009: cooldown / dispatch unchanged ---------------------------------

func swMetricRule(t *testing.T) (*database.DB, *Engine, *database.AlertConfig, *metrics.Snapshot) {
	t.Helper()
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "CPU", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))
	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 95}}
	return db, eng, ac, snap
}

// @aitri-tc TC-SAW-009h
func TestTC_SAW_009h(t *testing.T) {
	db, eng, ac, snap := swMetricRule(t)
	eng.evaluateMetricRule(*ac, snap)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1, "a confirmed breach fires exactly once")
}

// @aitri-tc TC-SAW-009e
func TestTC_SAW_009e(t *testing.T) {
	db, eng, ac, snap := swMetricRule(t)
	eng.evaluateMetricRule(*ac, snap)
	// Simulate the 15-minute cooldown elapsing.
	key := fmt.Sprintf("metric:%d", ac.ID)
	eng.mu.Lock()
	eng.cooldowns[key] = time.Now().Add(-16 * time.Minute)
	eng.mu.Unlock()
	eng.evaluateMetricRule(*ac, snap)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 2, "a continuing breach re-fires once the cooldown elapses")
}

// @aitri-tc TC-SAW-009f
func TestTC_SAW_009f(t *testing.T) {
	db, eng, ac, snap := swMetricRule(t)
	eng.evaluateMetricRule(*ac, snap)
	eng.evaluateMetricRule(*ac, snap) // within cooldown
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1, "no duplicate fire inside the cooldown window")
}

// ---- NFR-010: CI runs the suite on push to main -----------------------------

// ciWorkflow reads the existing CI workflow that runs the Go test suite.
func ciWorkflow(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../.github/workflows/security-gate.yml")
	require.NoError(t, err, "CI workflow must exist")
	return string(b)
}

// @aitri-tc TC-SAW-010h
func TestTC_SAW_010h(t *testing.T) {
	c := ciWorkflow(t)
	require.Contains(t, c, "go test ./...", "CI must run the full Go test suite")
	require.Contains(t, c, "branches: [ main ]", "CI must trigger on push to main")
}

// @aitri-tc TC-SAW-010e
func TestTC_SAW_010e(t *testing.T) {
	c := ciWorkflow(t)
	require.Contains(t, c, "./...", "CI must run the whole module so internal/alerts is included")
}

// @aitri-tc TC-SAW-010f
func TestTC_SAW_010f(t *testing.T) {
	c := ciWorkflow(t)
	require.NotContains(t, c, "continue-on-error", "a failing test must fail the CI job, not be masked")
	require.NotContains(t, c, "|| true", "the test step must not swallow failures")
}
