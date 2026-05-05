package landevices

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices/store"
)

// helper: build an orchestrator with an injectable clock and an empty store.
func newTestOrch(t *testing.T, cadence time.Duration, clock func() time.Time) *Orchestrator {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	o := New(Config{
		Store:   store.New(db.DB),
		Cadence: cadence,
		Now:     clock,
	})
	return o
}

// TC-LD-009h — two consecutive over-budget cycles trigger 2× cadence and
// SelfThrottled flag becomes true.
//
// @aitri-tc TC-LD-009h
func TestTC_LD_009h_AdjustCadence_TwoOverBudgetCyclesTriggerThrottle(t *testing.T) {
	// @aitri-tc TC-LD-009h
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	o := newTestOrch(t, 5*time.Minute, func() time.Time { return now })

	// First over-budget cycle: streak=1, no throttle yet.
	o.adjustCadence(4 * time.Second)
	assert.False(t, o.Status().SelfThrottled)

	// Second over-budget cycle: streak=2 (>= ThrottleStreak), throttle activates.
	o.adjustCadence(4 * time.Second)
	st := o.Status()
	assert.True(t, st.SelfThrottled)
	assert.Equal(t, 10*time.Minute, st.CurrentCadence) // doubled from 5 min
}

// TC-LD-009f — RestoreWindow of in-budget cycles after throttling restores
// the configured cadence and clears the flag.
//
// @aitri-tc TC-LD-009f
func TestTC_LD_009f_AdjustCadence_RestoresAfter30MinInBudget(t *testing.T) {
	// @aitri-tc TC-LD-009f
	clock := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	o := newTestOrch(t, 5*time.Minute, func() time.Time { return clock })

	// Throttle into 2× cadence.
	o.adjustCadence(4 * time.Second)
	o.adjustCadence(4 * time.Second)
	require.True(t, o.Status().SelfThrottled)
	require.Equal(t, 10*time.Minute, o.Status().CurrentCadence)

	// First in-budget cycle: marks inBudgetSince, no restore yet.
	clock = clock.Add(1 * time.Minute)
	o.adjustCadence(1 * time.Second)
	assert.True(t, o.Status().SelfThrottled, "still throttled after 1 in-budget cycle")

	// Advance past the 30-min window.
	clock = clock.Add(31 * time.Minute)
	o.adjustCadence(1 * time.Second)

	st := o.Status()
	assert.False(t, st.SelfThrottled)
	assert.Equal(t, 5*time.Minute, st.CurrentCadence)
}

// TC-LD-009e — cadence cap at MaxCadence even after repeated throttle events.
//
// @aitri-tc TC-LD-009e
func TestTC_LD_009e_AdjustCadence_CapsAtMaxCadence(t *testing.T) {
	// @aitri-tc TC-LD-009e
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	o := newTestOrch(t, 20*time.Minute, func() time.Time { return now })

	// First throttle pair: 20 min × 2 = 40 min → clamped to MaxCadence (30 min).
	o.adjustCadence(4 * time.Second)
	o.adjustCadence(4 * time.Second)
	st := o.Status()
	require.True(t, st.SelfThrottled)
	assert.Equal(t, MaxCadence, st.CurrentCadence)

	// Second over-budget pair: 30 min × 2 = 60 min → still clamped to 30 min.
	o.adjustCadence(4 * time.Second)
	o.adjustCadence(4 * time.Second)
	assert.Equal(t, MaxCadence, o.Status().CurrentCadence, "cadence must never exceed MaxCadence")
}

// TC-LD-002f — overrun counter increments when a cycle is in flight and
// the cadence ticker fires.
//
// @aitri-tc TC-LD-002f
func TestTC_LD_002f_TryCycle_OverrunIncrementsCounter(t *testing.T) {
	// @aitri-tc TC-LD-002f
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	o := newTestOrch(t, 1*time.Second, func() time.Time { return now })

	// Manually mark a cycle in flight, then fire tryCycle — it should
	// observe the in-flight flag and increment the overrun counter.
	o.mu.Lock()
	o.cycleInFlight = true
	o.mu.Unlock()

	for i := 0; i < 3; i++ {
		o.tryCycle(nil) //nolint:staticcheck — no real sweep is run because we short-circuit on cycleInFlight
	}
	assert.Equal(t, 3, o.Status().OverrunCount)
}

// Single in-budget cycle on a non-throttled orchestrator must NOT change
// state — the inBudgetSince counter only matters once throttled.
func TestAdjustCadence_InBudgetWhenHealthy_NoChange(t *testing.T) {
	now := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	o := newTestOrch(t, 5*time.Minute, func() time.Time { return now })
	for i := 0; i < 10; i++ {
		o.adjustCadence(500 * time.Millisecond)
	}
	st := o.Status()
	assert.False(t, st.SelfThrottled)
	assert.Equal(t, 5*time.Minute, st.CurrentCadence)
}
