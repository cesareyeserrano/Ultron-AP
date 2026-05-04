package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestNetDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertNetSample_OK(t *testing.T) {
	db := newTestNetDB(t)
	rtt := 1.23
	err := db.InsertNetSample(NetSample{
		TS:     time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
		Target: "192.168.1.1",
		Kind:   "icmp",
		RTTMs:  &rtt,
		Status: "ok",
	})
	require.NoError(t, err)

	got, err := db.RecentNetSamples("192.168.1.1", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "192.168.1.1", got[0].Target)
	assert.Equal(t, "icmp", got[0].Kind)
	assert.Equal(t, "ok", got[0].Status)
	require.NotNil(t, got[0].RTTMs)
	assert.InDelta(t, 1.23, *got[0].RTTMs, 0.001)
}

func TestInsertNetSample_NilRTT(t *testing.T) {
	db := newTestNetDB(t)
	err := db.InsertNetSample(NetSample{
		Target: "192.168.1.1",
		Kind:   "icmp",
		Status: "timeout",
	})
	require.NoError(t, err)

	got, err := db.RecentNetSamples("192.168.1.1", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "timeout", got[0].Status)
	assert.Nil(t, got[0].RTTMs)
	assert.False(t, got[0].TS.IsZero(), "missing ts should default to now")
}

func TestRecentNetSamples_OrderingAndLimit(t *testing.T) {
	db := newTestNetDB(t)
	base := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		rtt := float64(i)
		require.NoError(t, db.InsertNetSample(NetSample{
			TS:     base.Add(time.Duration(i) * time.Second),
			Target: "192.168.1.1",
			Kind:   "icmp",
			RTTMs:  &rtt,
			Status: "ok",
		}))
	}

	got, err := db.RecentNetSamples("192.168.1.1", 3)
	require.NoError(t, err)
	require.Len(t, got, 3)
	// newest first
	require.NotNil(t, got[0].RTTMs)
	require.NotNil(t, got[2].RTTMs)
	assert.InDelta(t, 4.0, *got[0].RTTMs, 0.001)
	assert.InDelta(t, 2.0, *got[2].RTTMs, 0.001)
}

func TestRecentNetSamples_FilterByTarget(t *testing.T) {
	db := newTestNetDB(t)
	rtt := 1.0
	require.NoError(t, db.InsertNetSample(NetSample{Target: "192.168.1.1", Kind: "icmp", RTTMs: &rtt, Status: "ok"}))
	require.NoError(t, db.InsertNetSample(NetSample{Target: "1.1.1.1", Kind: "icmp", RTTMs: &rtt, Status: "ok"}))

	got, err := db.RecentNetSamples("1.1.1.1", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "1.1.1.1", got[0].Target)
}

func TestPruneNetSamples(t *testing.T) {
	db := newTestNetDB(t)
	rtt := 1.0
	old := time.Now().AddDate(0, 0, -10)
	fresh := time.Now().Add(-1 * time.Minute)
	require.NoError(t, db.InsertNetSample(NetSample{TS: old, Target: "192.168.1.1", Kind: "icmp", RTTMs: &rtt, Status: "ok"}))
	require.NoError(t, db.InsertNetSample(NetSample{TS: fresh, Target: "192.168.1.1", Kind: "icmp", RTTMs: &rtt, Status: "ok"}))

	n, err := db.PruneNetSamples(7)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	got, err := db.RecentNetSamples("192.168.1.1", 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
}
