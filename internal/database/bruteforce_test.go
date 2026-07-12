// Tests for the SQLite-backed brute-force lockout store.
//
// @aitri-trace BG-022 BL-009
// TC-BG-022-001 .. 005
package database

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// TC-BG-022-001 — record on a fresh IP creates a count=1 row whose first_at
// is the supplied 'now'.
//
// @aitri-tc TC-BG-022-001
func TestBruteForce_RecordFailure_NewIP(t *testing.T) {
	db := newTestDB(t)
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

	count, firstAt, err := db.BruteForceRecordFailure("203.0.113.7", 15*time.Minute, now)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.True(t, firstAt.Equal(now), "first_at should equal supplied now; got %v", firstAt)

	c, fa, found, err := db.BruteForceLookup("203.0.113.7")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 1, c)
	assert.True(t, fa.Equal(now))
}

// TC-BG-022-002 — second failure within the window increments the count
// and preserves the original first_at.
//
// @aitri-tc TC-BG-022-002
func TestBruteForce_RecordFailure_IncrementsWithinWindow(t *testing.T) {
	db := newTestDB(t)
	t0 := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(5 * time.Minute) // still within 15-minute window

	_, _, err := db.BruteForceRecordFailure("203.0.113.7", 15*time.Minute, t0)
	require.NoError(t, err)
	count, firstAt, err := db.BruteForceRecordFailure("203.0.113.7", 15*time.Minute, t1)
	require.NoError(t, err)

	assert.Equal(t, 2, count, "count should increment within window")
	assert.True(t, firstAt.Equal(t0), "first_at should be preserved (got %v want %v)", firstAt, t0)
}

// TC-BG-022-003 — failure after the window has elapsed resets the count
// and rolls first_at forward to the new time.
//
// @aitri-tc TC-BG-022-003
func TestBruteForce_RecordFailure_RollsOverAfterWindow(t *testing.T) {
	db := newTestDB(t)
	t0 := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(20 * time.Minute) // past the 15-minute window

	_, _, err := db.BruteForceRecordFailure("203.0.113.7", 15*time.Minute, t0)
	require.NoError(t, err)
	count, firstAt, err := db.BruteForceRecordFailure("203.0.113.7", 15*time.Minute, t1)
	require.NoError(t, err)

	assert.Equal(t, 1, count, "count must reset to 1 after window expiry")
	assert.True(t, firstAt.Equal(t1), "first_at must roll forward; got %v want %v", firstAt, t1)
}

// TC-BG-022-004 — Lookup on an unknown IP returns found=false and no error.
//
// @aitri-tc TC-BG-022-004
func TestBruteForce_Lookup_NotFound(t *testing.T) {
	db := newTestDB(t)
	count, firstAt, found, err := db.BruteForceLookup("198.51.100.42")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, 0, count)
	assert.True(t, firstAt.IsZero())
}

// TC-BG-022-005 — Reset clears the row; PruneBefore deletes stale rows.
// State survives a re-open of the same DB file (simulating restart).
//
// @aitri-tc TC-BG-022-005
func TestBruteForce_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db1, err := New(dbPath)
	require.NoError(t, err)
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_, _, err := db1.BruteForceRecordFailure("203.0.113.7", 15*time.Minute, now)
		require.NoError(t, err)
	}
	require.NoError(t, db1.Close())

	// Re-open the same file — the count must still be 3, not zero.
	db2, err := New(dbPath)
	require.NoError(t, err)
	defer db2.Close()

	count, _, found, err := db2.BruteForceLookup("203.0.113.7")
	require.NoError(t, err)
	require.True(t, found, "row must survive db Close+reopen (BL-009 — restart-resistant lockout)")
	assert.Equal(t, 3, count)

	// Reset clears it.
	require.NoError(t, db2.BruteForceReset("203.0.113.7"))
	_, _, found, err = db2.BruteForceLookup("203.0.113.7")
	require.NoError(t, err)
	assert.False(t, found, "Reset should remove the row")
}

// TC-BG-022-006 — PruneBefore deletes stale rows and leaves recent ones.
//
// @aitri-tc TC-BG-022-006
func TestBruteForce_PruneBefore(t *testing.T) {
	db := newTestDB(t)
	t0 := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(20 * time.Minute)

	_, _, err := db.BruteForceRecordFailure("203.0.113.1", 15*time.Minute, t0)
	require.NoError(t, err)
	_, _, err = db.BruteForceRecordFailure("203.0.113.2", 15*time.Minute, t1)
	require.NoError(t, err)

	// Prune everything older than t1 — only the t0 row should disappear.
	removed, err := db.BruteForcePruneBefore(t1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)

	_, _, found1, err := db.BruteForceLookup("203.0.113.1")
	require.NoError(t, err)
	assert.False(t, found1, "stale row must be pruned")

	_, _, found2, err := db.BruteForceLookup("203.0.113.2")
	require.NoError(t, err)
	assert.True(t, found2, "recent row must survive prune")
}

// TC-A3 — concurrent failures for the same IP must not lose increments. The
// previous DEFERRED SELECT-then-UPSERT could drop counts under load (silently
// weakening the lockout). With the atomic UPSERT every failure is counted.
func TestBruteForce_RecordFailure_ConcurrentNoLostIncrements(t *testing.T) {
	db := newTestDB(t)
	const ip = "203.0.113.99"
	const n = 200
	window := 15 * time.Minute
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// All within the window so every call must increment.
			_, _, err := db.BruteForceRecordFailure(ip, window, base.Add(time.Duration(i)*time.Second))
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err, "no failure should error out")
	}

	c, _, found, err := db.BruteForceLookup(ip)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, n, c, "every concurrent failure must be counted (no lost increments)")
}
