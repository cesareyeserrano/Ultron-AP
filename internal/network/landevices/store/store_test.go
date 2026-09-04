package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

func newTestStore(t *testing.T) (*Store, *database.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return New(db.DB), db
}

func newTestStoreThreshold(t *testing.T, threshold int) (*Store, *database.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewWithThreshold(db.DB, threshold), db
}

// TC-LD-005h
//
// @aitri-tc TC-LD-005h
func TestTC_LD_005h_ApplySweep_FirstObservation_InsertsWithFirstEqualsLast(t *testing.T) {
	// @aitri-tc TC-LD-005h
	s, _ := newTestStore(t)
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

	err := s.ApplySweep(now, []Observation{
		{MAC: "aa:bb:cc:11:22:33", IP: "192.168.1.50", Vendor: "Acme Corp"},
	})
	require.NoError(t, err)

	d, err := s.Get("aa:bb:cc:11:22:33")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.50", d.IP)
	assert.Equal(t, "Acme Corp", d.Vendor)
	assert.True(t, d.Online)
	assert.Equal(t, 0, d.MissedSweeps)
	assert.WithinDuration(t, d.FirstSeen, d.LastSeen, time.Millisecond)
}

// TC-LD-005f — DHCP renewal: same MAC, new IP → ip updates, first_seen preserved
//
// @aitri-tc TC-LD-005f
func TestTC_LD_005f_ApplySweep_DHCPRenewal_PreservesFirstSeen(t *testing.T) {
	// @aitri-tc TC-LD-005f
	s, _ := newTestStore(t)
	t0 := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)

	require.NoError(t, s.ApplySweep(t0, []Observation{
		{MAC: "aa:bb:cc:11:22:33", IP: "192.168.1.50", Vendor: "Acme Corp"},
	}))
	require.NoError(t, s.ApplySweep(t1, []Observation{
		{MAC: "aa:bb:cc:11:22:33", IP: "192.168.1.99", Vendor: "Acme Corp"},
	}))

	d, err := s.Get("aa:bb:cc:11:22:33")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.99", d.IP)
	assert.WithinDuration(t, t1, d.LastSeen, time.Millisecond)
	assert.WithinDuration(t, t0, d.FirstSeen, time.Millisecond)
}

// TC-LD-005e — first_seen survives DB close + re-open
//
// @aitri-tc TC-LD-005e
func TestTC_LD_005e_ApplySweep_FirstSeenSurvivesRestart(t *testing.T) {
	// @aitri-tc TC-LD-005e
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	require.NoError(t, err)
	s := New(db.DB)

	t0 := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC) // 3 days before "now"
	require.NoError(t, s.ApplySweep(t0, []Observation{
		{MAC: "aa:bb:cc:11:22:33", IP: "192.168.1.50", Vendor: "Acme"},
	}))
	require.NoError(t, db.Close())

	db2, err := database.New(dbPath)
	require.NoError(t, err)
	defer db2.Close()
	s2 := New(db2.DB)

	d, err := s2.Get("aa:bb:cc:11:22:33")
	require.NoError(t, err)
	assert.WithinDuration(t, t0, d.FirstSeen, time.Millisecond)
}

// TC-LD-006h — single miss tolerated; device stays online
//
// @aitri-tc TC-LD-006h
func TestTC_LD_006h_ApplySweep_SingleMiss_StaysOnline(t *testing.T) {
	// @aitri-tc TC-LD-006h
	s, _ := newTestStoreThreshold(t, 3)
	t0 := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	mac := "aa:bb:cc:11:22:33"
	obs := Observation{MAC: mac, IP: "192.168.1.50", Vendor: "Acme"}

	require.NoError(t, s.ApplySweep(t0, []Observation{obs}))                            // sweep 1
	require.NoError(t, s.ApplySweep(t0.Add(5*time.Minute), []Observation{obs}))         // sweep 2
	require.NoError(t, s.ApplySweep(t0.Add(10*time.Minute), nil))                       // sweep 3 — miss
	require.NoError(t, s.ApplySweep(t0.Add(15*time.Minute), []Observation{obs}))        // sweep 4 — back

	d, err := s.Get(mac)
	require.NoError(t, err)
	assert.True(t, d.Online)
	assert.Equal(t, 0, d.MissedSweeps)
	assert.WithinDuration(t, t0.Add(15*time.Minute), d.LastSeen, time.Millisecond)
}

// TC-LD-006f — N consecutive misses flip offline; last_seen freezes
//
// @aitri-tc TC-LD-006f
func TestTC_LD_006f_ApplySweep_ThreeConsecutiveMisses_FlipsOffline(t *testing.T) {
	// @aitri-tc TC-LD-006f
	s, _ := newTestStoreThreshold(t, 3)
	t0 := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	mac := "aa:bb:cc:11:22:33"
	obs := Observation{MAC: mac, IP: "192.168.1.50", Vendor: "Acme"}

	require.NoError(t, s.ApplySweep(t0, []Observation{obs}))
	require.NoError(t, s.ApplySweep(t0.Add(5*time.Minute), nil))
	require.NoError(t, s.ApplySweep(t0.Add(10*time.Minute), nil))
	require.NoError(t, s.ApplySweep(t0.Add(15*time.Minute), nil))

	d, err := s.Get(mac)
	require.NoError(t, err)
	assert.False(t, d.Online)
	assert.Equal(t, 3, d.MissedSweeps)
	assert.WithinDuration(t, t0, d.LastSeen, time.Millisecond, "last_seen should freeze at sweep 1")
}

// TC-LD-006e — recovery from offline
//
// @aitri-tc TC-LD-006e
func TestTC_LD_006e_ApplySweep_OfflineDeviceRecoversOnReappearance(t *testing.T) {
	// @aitri-tc TC-LD-006e
	s, _ := newTestStoreThreshold(t, 3)
	t0 := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	mac := "aa:bb:cc:11:22:33"
	obs := Observation{MAC: mac, IP: "192.168.1.50", Vendor: "Acme"}

	require.NoError(t, s.ApplySweep(t0, []Observation{obs}))
	for i := 1; i <= 3; i++ {
		require.NoError(t, s.ApplySweep(t0.Add(time.Duration(i)*5*time.Minute), nil))
	}
	d, err := s.Get(mac)
	require.NoError(t, err)
	require.False(t, d.Online, "precondition: device should be offline")
	originalFirst := d.FirstSeen

	t5 := t0.Add(20 * time.Minute)
	require.NoError(t, s.ApplySweep(t5, []Observation{obs}))

	d, err = s.Get(mac)
	require.NoError(t, err)
	assert.True(t, d.Online)
	assert.Equal(t, 0, d.MissedSweeps)
	assert.WithinDuration(t, t5, d.LastSeen, time.Millisecond)
	assert.WithinDuration(t, originalFirst, d.FirstSeen, time.Millisecond)
}

// TC-LD-007h — List returns online first, then last_seen DESC
//
// @aitri-tc TC-LD-007h
func TestTC_LD_007h_List_OrderingOnlineFirstThenLastSeenDesc(t *testing.T) {
	// @aitri-tc TC-LD-007h
	s, _ := newTestStoreThreshold(t, 3)
	t0 := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)

	a := Observation{MAC: "aa:aa:aa:00:00:01", IP: "192.168.1.10", Vendor: "A"}
	b := Observation{MAC: "bb:bb:bb:00:00:02", IP: "192.168.1.11", Vendor: "B"}
	c := Observation{MAC: "cc:cc:cc:00:00:03", IP: "192.168.1.12", Vendor: "C"}

	require.NoError(t, s.ApplySweep(t0, []Observation{a, b, c}))                  // all online
	require.NoError(t, s.ApplySweep(t0.Add(5*time.Minute), []Observation{a, b})) // c misses
	require.NoError(t, s.ApplySweep(t0.Add(10*time.Minute), []Observation{a, b}))
	require.NoError(t, s.ApplySweep(t0.Add(15*time.Minute), []Observation{a, b})) // c crosses threshold → offline

	// b's last_seen is t0+15m, a's last_seen is t0+15m, c's last_seen is t0.
	// We expect online ones first (a, b in some order by last_seen DESC), then c.
	devices, err := s.List()
	require.NoError(t, err)
	require.Len(t, devices, 3)
	assert.True(t, devices[0].Online)
	assert.True(t, devices[1].Online)
	assert.False(t, devices[2].Online)
	assert.Equal(t, "cc:cc:cc:00:00:03", devices[2].MAC)
}

// TC-LD-012e — table size grows with distinct MACs, not sweep count
//
// @aitri-tc TC-LD-012e
func TestTC_LD_012e_ApplySweep_RowsBoundedByDistinctMACs(t *testing.T) {
	// @aitri-tc TC-LD-012e
	s, _ := newTestStoreThreshold(t, 100) // never flip offline within the test
	t0 := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	five := []Observation{
		{MAC: "aa:00:00:00:00:01", IP: "192.168.1.1", Vendor: "A"},
		{MAC: "aa:00:00:00:00:02", IP: "192.168.1.2", Vendor: "A"},
		{MAC: "aa:00:00:00:00:03", IP: "192.168.1.3", Vendor: "A"},
		{MAC: "aa:00:00:00:00:04", IP: "192.168.1.4", Vendor: "A"},
		{MAC: "aa:00:00:00:00:05", IP: "192.168.1.5", Vendor: "A"},
	}
	for i := 0; i < 100; i++ {
		require.NoError(t, s.ApplySweep(t0.Add(time.Duration(i)*time.Minute), five))
	}
	n, err := s.Count()
	require.NoError(t, err)
	assert.Equal(t, 5, n)
}

// TC-LD-012h — schema is idempotent and lan_devices columns match expected
//
// @aitri-tc TC-LD-012h
func TestTC_LD_012h_Schema_IdempotentAndColumnsMatch(t *testing.T) {
	// @aitri-tc TC-LD-012h
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db1, err := database.New(dbPath)
	require.NoError(t, err)
	require.NoError(t, db1.Close())

	db2, err := database.New(dbPath)
	require.NoError(t, err)
	defer db2.Close()

	rows, err := db2.Query(`PRAGMA table_info(lan_devices)`)
	require.NoError(t, err)
	defer rows.Close()
	cols := []string{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql_NullString
		require.NoError(t, rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk))
		cols = append(cols, name)
	}
	assert.Equal(t,
		[]string{"mac", "ip", "vendor", "first_seen", "last_seen", "online", "missed_sweeps"},
		cols,
	)
}

// sql_NullString avoids importing database/sql just for this scan target.
type sql_NullString struct {
	String string
	Valid  bool
}

func (s *sql_NullString) Scan(v interface{}) error {
	if v == nil {
		s.Valid = false
		return nil
	}
	s.Valid = true
	switch x := v.(type) {
	case string:
		s.String = x
	case []byte:
		s.String = string(x)
	}
	return nil
}

// M3 — offline devices older than the retention window are pruned so a
// spoofed-MAC flood cannot grow the table without bound. A recently-seen
// device must survive the same sweep.
func TestApplySweep_PrunesStaleOfflineDevices(t *testing.T) {
	s, _ := newTestStoreThreshold(t, 1) // flip offline after a single miss
	t0 := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

	require.NoError(t, s.ApplySweep(t0, []Observation{{MAC: "aa:bb:cc:00:00:01", IP: "192.168.1.10"}}))
	// One empty sweep flips it offline (threshold=1).
	require.NoError(t, s.ApplySweep(t0.Add(time.Minute), nil))

	// A fresh device seen just before the pruning sweep must be kept.
	late := t0.Add(defaultPruneRetention + time.Hour)
	require.NoError(t, s.ApplySweep(late, []Observation{{MAC: "aa:bb:cc:00:00:02", IP: "192.168.1.11"}}))

	devices, err := s.List()
	require.NoError(t, err)
	macs := map[string]bool{}
	for _, d := range devices {
		macs[d.MAC] = true
	}
	assert.False(t, macs["aa:bb:cc:00:00:01"], "stale offline device should be pruned")
	assert.True(t, macs["aa:bb:cc:00:00:02"], "recently-seen device must be kept")
}
