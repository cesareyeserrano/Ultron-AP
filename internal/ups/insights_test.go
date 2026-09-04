package ups

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedOutage records one closed outage [start, start+dur].
func seedOutage(t *testing.T, st *Store, start time.Time, dur time.Duration) {
	t.Helper()
	require.NoError(t, st.OpenEvent(start))
	_, err := st.CloseOpenEvent(start.Add(dur))
	require.NoError(t, err)
}

// TC-UPS-024h (FR-022): weekly outage insight fires with ≥1 closed event.
func TestTC_UPS_024h_WeeklyOutageInsight(t *testing.T) {
	// @aitri-tc TC-UPS-024h
	st, _ := newTestStore(t)
	now := time.Unix(2_000_000, 0)
	seedOutage(t, st, now.Add(-2*time.Hour), 5*time.Minute)

	vars, err := st.InsightVars(now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, vars["ups_outages_7d"])

	insights, err := st.Insights(now)
	require.NoError(t, err)
	require.NotEmpty(t, insights, "an outage insight must be produced")
	assert.Contains(t, insights[0].Text, "1 corte")
}

// TC-UPS-025e (FR-022): outage insight renders with real data (count = 3), not empty state.
func TestTC_UPS_025e_OutageInsightRealCount(t *testing.T) {
	// @aitri-tc TC-UPS-025e
	st, _ := newTestStore(t)
	now := time.Unix(2_000_000, 0)
	seedOutage(t, st, now.Add(-3*24*time.Hour), 2*time.Minute)
	seedOutage(t, st, now.Add(-2*24*time.Hour), 3*time.Minute)
	seedOutage(t, st, now.Add(-1*24*time.Hour), 1*time.Minute)

	vars, err := st.InsightVars(now)
	require.NoError(t, err)
	assert.EqualValues(t, 3, vars["ups_outages_7d"], "the real trigger value must be 3, not an empty state")

	insights, err := st.Insights(now)
	require.NoError(t, err)
	require.NotEmpty(t, insights)
	assert.Contains(t, insights[0].Text, "3 cortes", "the insight renders the real count")
}

// Extra coverage (not a TC): with enough resting samples on a descending trend,
// the battery-degradation insight fires with the real voltage drop.
func TestDegradationInsightFiresOnTrend(t *testing.T) {
	st, _ := newTestStore(t)
	now := time.Unix(2_000_000, 0)
	// 40 resting (on-mains) samples descending from ~27.4 V to ~26.1 V.
	for i := 0; i < 40; i++ {
		v := 27.4 - float64(i)*0.033 // ~1.3 V total drop
		require.NoError(t, st.WriteSample(Snapshot{
			State: StateOnline, RawStatus: "OL", Reachable: true,
			BatteryV: fp(v), LastGood: now.Add(time.Duration(i-40) * time.Hour),
		}))
	}
	vars, err := st.InsightVars(now)
	require.NoError(t, err)
	drop, ok := vars["ups_batt_drop_v"]
	require.True(t, ok, "enough history must yield a battery-drop signal")
	assert.Greater(t, drop, 0.5, "resting voltage clearly dropped")

	insights, err := st.Insights(now)
	require.NoError(t, err)
	found := false
	for _, in := range insights {
		if in.Title == "Batería degradándose" {
			found = true
			assert.Contains(t, in.Text, "V")
		}
	}
	assert.True(t, found, "a battery-degradation insight must be produced on a clear downward trend")
}

// TC-UPS-026f (FR-022): no degradation insight from a single sample.
func TestTC_UPS_026f_NoDegradationFromSingleSample(t *testing.T) {
	// @aitri-tc TC-UPS-026f
	st, _ := newTestStore(t)
	now := time.Unix(2_000_000, 0)
	// A single resting sample — nowhere near enough to claim a trend.
	require.NoError(t, st.WriteSample(Snapshot{
		State: StateOnline, RawStatus: "OL", Reachable: true, BatteryV: fp(27.0), LastGood: now.Add(-1 * time.Hour),
	}))

	vars, err := st.InsightVars(now)
	require.NoError(t, err)
	_, hasDrop := vars["ups_batt_drop_v"]
	assert.False(t, hasDrop, "insufficient history must not yield a battery-drop signal")

	insights, err := st.Insights(now)
	require.NoError(t, err)
	for _, in := range insights {
		assert.NotContains(t, in.Title, "degrad", "no battery-degradation claim from a single sample")
	}
}
