package ups

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// newTestStore opens a real temp SQLite DB (with the ups tables) and returns a
// store plus the db path so a test can simulate a restart by reopening it.
func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ups.db")
	db, err := database.New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db.DB), path
}

func fp(v float64) *float64 { return &v }

// TC-UPS-013h (FR-019): a ups_samples row survives a process restart.
func TestTC_UPS_013h_SampleSurvivesRestart(t *testing.T) {
	// @aitri-tc TC-UPS-013h
	path := filepath.Join(t.TempDir(), "ups.db")
	db, err := database.New(path)
	require.NoError(t, err)
	st := NewStore(db.DB)

	snap := Snapshot{
		State: StateOnline, RawStatus: "OL", Reachable: true,
		LoadPct: fp(2), InputV: fp(122.0), BatteryV: fp(27.1), BattPctEst: fp(96),
		LastGood: time.Unix(1000, 0),
	}
	require.NoError(t, st.WriteSample(snap))
	require.NoError(t, db.Close()) // simulate process exit

	db2, err := database.New(path) // reopen the same file
	require.NoError(t, err)
	t.Cleanup(func() { _ = db2.Close() })
	series, err := NewStore(db2.DB).Series(time.Unix(0, 0), time.Unix(2000, 0))
	require.NoError(t, err)
	require.Len(t, series, 1)
	assert.Equal(t, StateOnline, series[0].State)
	assert.InDelta(t, 27.1, *series[0].BatteryV, 0.001)
	assert.InDelta(t, 96, *series[0].BattPctEst, 0.001)
}

// TC-UPS-014e (FR-019): an empty chart series returns no error.
func TestTC_UPS_014e_EmptySeriesNoError(t *testing.T) {
	// @aitri-tc TC-UPS-014e
	st, _ := newTestStore(t)
	series, err := st.Series(time.Now().Add(-24*time.Hour), time.Now())
	require.NoError(t, err)
	assert.Empty(t, series, "no history yet must return an empty slice, not an error")
}

// TC-UPS-015f (FR-019): purge removes samples older than retention only.
func TestTC_UPS_015f_PruneRetentionBoundary(t *testing.T) {
	// @aitri-tc TC-UPS-015f
	st, _ := newTestStore(t)
	old := Snapshot{State: StateOnline, RawStatus: "OL", Reachable: true, BatteryV: fp(27.0), LastGood: time.Now().AddDate(0, 0, -31)}
	fresh := Snapshot{State: StateOnline, RawStatus: "OL", Reachable: true, BatteryV: fp(27.1), LastGood: time.Now().AddDate(0, 0, -1)}
	require.NoError(t, st.WriteSample(old))
	require.NoError(t, st.WriteSample(fresh))

	removed, err := st.PruneSamples(30)
	require.NoError(t, err)
	assert.EqualValues(t, 1, removed, "only the 31-day-old row is pruned")

	series, err := st.Series(time.Now().AddDate(0, 0, -40), time.Now())
	require.NoError(t, err)
	require.Len(t, series, 1, "the within-retention row remains")
	assert.InDelta(t, 27.1, *series[0].BatteryV, 0.001)
}

// TC-UPS-016h (FR-020): OL→OB opens an outage event.
func TestTC_UPS_016h_OpenOutage(t *testing.T) {
	// @aitri-tc TC-UPS-016h
	st, _ := newTestStore(t)
	require.NoError(t, st.OpenEvent(time.Unix(5000, 0)))
	events, err := st.RecentEvents(10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, time.Unix(5000, 0).UnixMilli(), events[0].Start.UnixMilli())
	assert.Nil(t, events[0].End, "a freshly opened outage has no end")
	assert.Nil(t, events[0].DurationS)
}

// TC-UPS-017e (FR-020): OB→OL closes the event with duration.
func TestTC_UPS_017e_CloseOutage(t *testing.T) {
	// @aitri-tc TC-UPS-017e
	st, _ := newTestStore(t)
	require.NoError(t, st.OpenEvent(time.Unix(5000, 0)))
	dur, err := st.CloseOpenEvent(time.Unix(5300, 0))
	require.NoError(t, err)
	assert.EqualValues(t, 300*time.Second, dur)

	events, err := st.RecentEvents(10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.NotNil(t, events[0].End)
	assert.Equal(t, time.Unix(5300, 0).UnixMilli(), events[0].End.UnixMilli())
	require.NotNil(t, events[0].DurationS)
	assert.EqualValues(t, 300, *events[0].DurationS)

	open, err := st.HasOpenEvent()
	require.NoError(t, err)
	assert.False(t, open, "no event remains open after close")
}

// TC-UPS-018f (FR-020): restart with an open event does not double-count on recovery.
func TestTC_UPS_018f_NoDoubleCountOnRestart(t *testing.T) {
	// @aitri-tc TC-UPS-018f
	st, _ := newTestStore(t)
	// Outage opened before the "restart".
	require.NoError(t, st.OpenEvent(time.Unix(5000, 0)))

	// Restart: reconcile finds the open event.
	open, err := st.ReconcileOpenOnBoot()
	require.NoError(t, err)
	assert.True(t, open)

	// The poller, seeing OB again after boot, calls OpenEvent — it must be a
	// no-op because an outage is already open (no duplicate).
	require.NoError(t, st.OpenEvent(time.Unix(5100, 0)))

	// Power returns.
	dur, err := st.CloseOpenEvent(time.Unix(5300, 0))
	require.NoError(t, err)
	assert.EqualValues(t, 300*time.Second, dur, "duration measured from the original start")

	count, err := st.CountEvents()
	require.NoError(t, err)
	assert.Equal(t, 1, count, "exactly one outage event, no orphaned duplicate")
}

// @aitri-tc TC-NSR-062e — the UPS prune keeps working and stays in its own
// lane: it removes its own old rows and never touches NetSample.
//
// This is the regression the network-retention feature has to protect. Both
// tables are pruned from the same daily job now, and the cheapest way to break
// UPS retention would be to widen a WHERE clause by accident.
//
// @aitri-trace NFR-106
func TestTC_NSR_062e(t *testing.T) {
	st, path := newTestStore(t)

	old := Snapshot{State: StateOnline, RawStatus: "OL", Reachable: true, BatteryV: fp(27.0), LastGood: time.Now().AddDate(0, 0, -45)}
	fresh := Snapshot{State: StateOnline, RawStatus: "OL", Reachable: true, BatteryV: fp(27.2), LastGood: time.Now().AddDate(0, 0, -1)}
	require.NoError(t, st.WriteSample(old))
	require.NoError(t, st.WriteSample(fresh))

	// A network sample in the same database, also outside a 30-day window.
	db, err := database.New(path)
	require.NoError(t, err)
	defer db.Close()
	rtt := 5.0
	require.NoError(t, db.InsertNetSample(database.NetSample{
		TS: time.Now().AddDate(0, 0, -45), Target: "gateway", Kind: "icmp", RTTMs: &rtt, Status: "ok",
	}))

	removed, err := st.PruneSamples(30)
	require.NoError(t, err)
	assert.EqualValues(t, 1, removed, "the UPS prune removes its own 45-day row")

	var nets int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM NetSample").Scan(&nets))
	assert.Equal(t, 1, nets, "the UPS prune must not reach into NetSample")

	series, err := st.Series(time.Now().AddDate(0, 0, -60), time.Now())
	require.NoError(t, err)
	require.Len(t, series, 1, "the in-window UPS row survives")
}
