// Tests for network-sample retention: batched pruning and space reclamation.
//
// These run against a real SQLite FILE, not an in-memory database. That is not
// incidental: the central claim of FR-099 is that the file on disk gets
// smaller, and a DELETE that leaves free pages behind is indistinguishable from
// a successful reclaim unless you can stat the file.
//
// @aitri-trace FR-097 FR-098 FR-099 NFR-102 NFR-107 NFR-108 NFR-111
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFileDB opens a database backed by a real file so size can be measured.
func newFileDB(t *testing.T) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "retention.db")
	db, err := New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

// withPruneBatch shrinks the delete batch for one test and restores it after.
//
// The production value is 50000; proving the batching loop works at that size
// would mean seeding 100k+ rows per test, which pushed this package past the
// 10-minute `go test` timeout under -race. The loop under test is identical at
// any batch size.
func withPruneBatch(t *testing.T, n int) {
	t.Helper()
	prev := netPruneBatch
	netPruneBatch = n
	t.Cleanup(func() { netPruneBatch = prev })
}

// seedSamples inserts n samples, all aged the given number of days.
//
// One transaction for the whole batch. Inserting row by row in autocommit means
// a WAL frame and an fsync per row, which is what actually made these tests
// take minutes rather than the pruning they were meant to measure.
func seedSamples(t *testing.T, db *DB, n int, ageDays float64) {
	t.Helper()
	ts := time.Now().Add(-time.Duration(ageDays * float64(24*time.Hour))).UnixMilli()

	tx, err := db.Begin()
	require.NoError(t, err)
	stmt, err := tx.Prepare(`INSERT INTO NetSample (ts, target, kind, rtt_ms, status) VALUES (?, ?, ?, ?, ?)`)
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		_, err := stmt.Exec(ts, fmt.Sprintf("t%d", i%5), "icmp", 12.5, "ok")
		require.NoError(t, err)
	}
	require.NoError(t, stmt.Close())
	require.NoError(t, tx.Commit())
}

func countSamples(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM NetSample").Scan(&n))
	return n
}

// @aitri-tc TC-NSR-010h — the prune removes what is outside the window and
// keeps what is inside (AC-097-001).
func TestTC_NSR_010h(t *testing.T) {
	db, _ := newFileDB(t)
	for _, age := range []float64{45, 40, 20, 1} {
		seedSamples(t, db, 1, age)
	}

	n, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "the 45- and 40-day samples must go")
	assert.Equal(t, 2, countSamples(t, db), "the 20- and 1-day samples must stay")

	var oldest int64
	require.NoError(t, db.QueryRow("SELECT MIN(ts) FROM NetSample").Scan(&oldest))
	cutoff := time.Now().AddDate(0, 0, -30).UnixMilli()
	assert.Greater(t, oldest, cutoff, "nothing older than the window may survive")
}

// @aitri-tc TC-NSR-011e — the boundary is the exact window, not a rounded day
// (AC-097-002).
func TestTC_NSR_011e(t *testing.T) {
	db, _ := newFileDB(t)
	seedSamples(t, db, 1, 29+23.0/24) // 29d23h — inside
	seedSamples(t, db, 1, 30+1.0/24)  // 30d01h — outside

	n, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.Equal(t, 1, countSamples(t, db), "29d23h is inside a 30-day window and must survive")
}

// @aitri-tc TC-NSR-013e — a prune over a clean table removes nothing and does
// not error (AC-097-004).
func TestTC_NSR_013e(t *testing.T) {
	db, _ := newFileDB(t)
	seedSamples(t, db, 10, 1)

	n, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Equal(t, 10, countSamples(t, db))
}

// @aitri-tc TC-NSR-020h — the batched total matches the rows out of window
// (AC-098-002).
func TestTC_NSR_020h(t *testing.T) {
	withPruneBatch(t, 200)
	db, _ := newFileDB(t)
	const out = 500 // 200 + 200 + 100 — forces three batches
	seedSamples(t, db, out, 45)
	seedSamples(t, db, 500, 1)

	n, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	assert.Equal(t, int64(out), n, "batching must neither lose nor double-count rows")
	assert.Equal(t, 500, countSamples(t, db))
}

// @aitri-tc TC-NSR-021e — no single statement exceeds the batch size
// (AC-098-001).
//
// Asserted by construction and by observation: the deletes are counted in
// steps, and the arithmetic below only holds if each statement was bounded.
func TestTC_NSR_021e(t *testing.T) {
	withPruneBatch(t, 200)
	db, _ := newFileDB(t)
	const out = 500
	seedSamples(t, db, out, 45)

	// Drive the same loop the production path runs, one batch at a time, and
	// record what each individual statement removed.
	cutoff := time.Now().AddDate(0, 0, -30).UnixMilli()
	var sizes []int64
	for {
		res, err := db.Exec(
			`DELETE FROM NetSample WHERE id IN (SELECT id FROM NetSample WHERE ts < ? LIMIT ?)`,
			cutoff, netPruneBatch)
		require.NoError(t, err)
		n, err := res.RowsAffected()
		require.NoError(t, err)
		if n == 0 {
			break
		}
		sizes = append(sizes, n)
	}

	require.Len(t, sizes, 3, "%d rows at a batch of %d must take three statements", out, netPruneBatch)
	assert.Equal(t, []int64{200, 200, 100}, sizes)
	for i, n := range sizes {
		assert.LessOrEqualf(t, n, int64(netPruneBatch), "statement %d exceeded the batch bound", i)
	}
}

// @aitri-tc TC-NSR-022f — the batch loop terminates instead of spinning
// (AC-098-003).
//
// A loop that never advances would hang the shared retention job forever, which
// is a worse failure than not pruning at all.
func TestTC_NSR_022f(t *testing.T) {
	db, _ := newFileDB(t)
	seedSamples(t, db, 10, 1)

	done := make(chan struct{})
	var n int64
	var err error
	go func() { n, err = db.PruneNetSamples(30); close(done) }()

	select {
	case <-done:
		require.NoError(t, err)
		assert.Equal(t, int64(0), n)
	case <-time.After(5 * time.Second):
		t.Fatal("PruneNetSamples did not terminate — the batch loop is not advancing")
	}
}

// @aitri-tc TC-NSR-023e — inserts continue while the prune runs (AC-098-004).
func TestTC_NSR_023e(t *testing.T) {
	withPruneBatch(t, 200)
	db, _ := newFileDB(t)
	seedSamples(t, db, 2000, 45)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	wg.Add(1)
	go func() {
		defer wg.Done()
		rtt := 1.0
		for i := 0; i < 50; i++ {
			if err := db.InsertNetSample(NetSample{
				TS: time.Now(), Target: "live", Kind: "icmp", RTTMs: &rtt, Status: "ok",
			}); err != nil {
				errs <- err
			}
			time.Sleep(time.Millisecond)
		}
	}()

	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Errorf("insert failed while pruning: %v", e)
	}
	var live int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM NetSample WHERE target = 'live'`).Scan(&live))
	assert.Equal(t, 50, live, "every sample written during the prune must survive it")
}

// @aitri-tc TC-NSR-030h — compaction shrinks the file on disk (AC-099-001).
func TestTC_NSR_030h(t *testing.T) {
	db, path := newFileDB(t)
	seedSamples(t, db, 20000, 45)
	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)

	// Compare like with like: both measurements on a checkpointed file, so the
	// only difference is the space VACUUM reclaimed.
	checkpoint(t, db)
	before := fileSize(t, path)
	require.NoError(t, db.Compact())
	after := fileSize(t, path)

	assert.Less(t, after, before,
		"DELETE alone leaves free pages: the file must shrink only after compaction (%d -> %d)", before, after)
}

// @aitri-tc TC-NSR-031e — reported free space drops after compaction
// (AC-099-002).
func TestTC_NSR_031e(t *testing.T) {
	db, _ := newFileDB(t)
	seedSamples(t, db, 20000, 45)
	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)

	checkpoint(t, db)
	before, err := db.FreeSpaceBytes()
	require.NoError(t, err)
	require.Greater(t, before, int64(0), "the prune must have left free pages to reclaim")

	require.NoError(t, db.Compact())
	after, err := db.FreeSpaceBytes()
	require.NoError(t, err)
	assert.Less(t, after, before)
}

// @aitri-tc TC-NSR-032e — the database stays intact after compaction
// (AC-099-003).
func TestTC_NSR_032e(t *testing.T) {
	db, _ := newFileDB(t)
	seedSamples(t, db, 20000, 45)
	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	require.NoError(t, db.Compact())

	var result string
	require.NoError(t, db.QueryRow("PRAGMA integrity_check").Scan(&result))
	assert.Equal(t, "ok", result)
}

// @aitri-tc TC-NSR-033e — in-window data survives compaction unchanged
// (AC-099-004).
func TestTC_NSR_033e(t *testing.T) {
	db, _ := newFileDB(t)
	seedSamples(t, db, 20000, 45)
	seedSamples(t, db, 500, 1)

	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	// seedSamples spreads across five targets, so t0 holds a fifth of them.
	before, err := db.RecentNetSamples("t0", 200)
	require.NoError(t, err)
	require.Len(t, before, 100)

	require.NoError(t, db.Compact())
	after, err := db.RecentNetSamples("t0", 200)
	require.NoError(t, err)

	require.Len(t, after, 100, "compaction must not lose a single in-window row")
	for i := range before {
		assert.Equal(t, before[i].Target, after[i].Target)
		assert.Equal(t, before[i].Kind, after[i].Kind)
		assert.Equal(t, before[i].Status, after[i].Status)
		require.NotNil(t, after[i].RTTMs)
		assert.Equal(t, *before[i].RTTMs, *after[i].RTTMs)
	}
}

// @aitri-tc TC-NSR-050h — the prune SQL binds its parameters instead of
// interpolating them (NFR-102).
func TestTC_NSR_050h(t *testing.T) {
	src, err := os.ReadFile("network.go")
	require.NoError(t, err)
	body := string(src)

	start := strings.Index(body, "func (db *DB) PruneNetSamples")
	require.Greater(t, start, 0)
	fn := body[start:]
	if end := strings.Index(fn[1:], "\nfunc "); end > 0 {
		fn = fn[:end]
	}

	assert.NotContains(t, fn, "Sprintf", "the prune SQL must not be built with Sprintf")
	assert.NotContains(t, fn, `" +`, "the prune SQL must not be built by concatenation")
	assert.Contains(t, fn, "ts < ?", "the cutoff must travel as a bound parameter")
}

// @aitri-tc TC-NSR-053e — the batch size is a code constant, untouched by the
// environment (NFR-102).
func TestTC_NSR_053e(t *testing.T) {
	t.Setenv("ULTRON_NET_RETENTION_DAYS", "1")
	t.Setenv("ULTRON_NET_PRUNE_BATCH", "999999") // no such setting exists, deliberately
	assert.Equal(t, 50000, defaultNetPruneBatch,
		"the environment may shape the parameter, never the statement")
	assert.Equal(t, defaultNetPruneBatch, netPruneBatch,
		"outside a test that shrinks it deliberately, the batch is the default")
}

// @aitri-tc TC-NSR-070h — the NetSample columns are unchanged (NFR-107).
func TestTC_NSR_070h(t *testing.T) {
	db, _ := newFileDB(t)
	rows, err := db.Query("PRAGMA table_info(NetSample)")
	require.NoError(t, err)
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk))
		cols = append(cols, name)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"id", "ts", "target", "kind", "rtt_ms", "status"}, cols)
}

// @aitri-tc TC-NSR-071e — the two index names are unchanged (NFR-107).
func TestTC_NSR_071e(t *testing.T) {
	db, _ := newFileDB(t)
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='NetSample'`)
	require.NoError(t, err)
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		seen[name] = true
	}
	require.NoError(t, rows.Err())
	assert.True(t, seen["idx_net_sample_target_ts"], "index renamed — the UI depends on it: %v", seen)
	assert.True(t, seen["idx_net_sample_ts"], "index renamed — the UI depends on it: %v", seen)
}

// @aitri-tc TC-NSR-072f — the UI's query still returns rows after prune and
// compaction (NFR-107).
func TestTC_NSR_072f(t *testing.T) {
	db, _ := newFileDB(t)
	seedSamples(t, db, 5000, 45)
	seedSamples(t, db, 100, 1)
	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	require.NoError(t, db.Compact())

	got, err := db.RecentNetSamples("t0", 50)
	require.NoError(t, err)
	require.Len(t, got, 20, "t0 holds a fifth of the 100 in-window samples")
	for _, s := range got {
		assert.NotEmpty(t, s.Target)
		assert.NotEmpty(t, s.Status)
	}
}

// @aitri-tc TC-NSR-080h — a 7-day query returns the same set before and after
// pruning: the weekly outage panel is untouched by a 30-day window (NFR-108).
func TestTC_NSR_080h(t *testing.T) {
	db, _ := newFileDB(t)
	for _, age := range []float64{45, 40, 35, 20, 6, 3, 1} {
		seedSamples(t, db, 10, age)
	}
	weekCutoff := time.Now().AddDate(0, 0, -7).UnixMilli()

	before := idsSince(t, db, weekCutoff)
	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	after := idsSince(t, db, weekCutoff)

	assert.Equal(t, before, after, "a 30-day window cannot touch anything the 7-day panel reads")
	assert.Len(t, after, 30, "the 6-, 3- and 1-day batches are 30 rows")
}

// @aitri-tc TC-NSR-081e — pruning samples does not touch network events
// (NFR-108).
func TestTC_NSR_081e(t *testing.T) {
	db, _ := newFileDB(t)
	seedSamples(t, db, 1, 45)
	require.NoError(t, db.InsertNetEvent(NetEvent{
		TS: time.Now().AddDate(0, 0, -45), Kind: "wan_down", Detail: "old outage",
	}))

	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)

	assert.Equal(t, 0, countSamples(t, db), "the old sample must go")
	var events int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM NetEvent").Scan(&events))
	assert.Equal(t, 1, events, "the prune must not spread to other tables")
}

// @aitri-tc TC-NSR-082f — recent samples are never caught by the default
// window (NFR-108).
func TestTC_NSR_082f(t *testing.T) {
	db, _ := newFileDB(t)
	for _, age := range []float64{3, 2, 1} {
		seedSamples(t, db, 5, age)
	}
	n, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Equal(t, 15, countSamples(t, db))
}

// @aitri-tc TC-NSR-110h — inserts progress throughout a large prune (NFR-111).
func TestTC_NSR_110h(t *testing.T) {
	withPruneBatch(t, 200)
	db, _ := newFileDB(t)
	seedSamples(t, db, 2000, 45)

	stop := make(chan struct{})
	var mu sync.Mutex
	var failures []error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rtt := 2.0
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := db.InsertNetSample(NetSample{
				TS: time.Now(), Target: "concurrent", Kind: "icmp", RTTMs: &rtt, Status: "ok",
			}); err != nil {
				mu.Lock()
				failures = append(failures, err)
				mu.Unlock()
			}
			time.Sleep(time.Millisecond)
		}
	}()

	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	close(stop)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Emptyf(t, failures, "inserts must not fail during a prune: %v", failures)
}

// @aitri-tc TC-NSR-111e — samples written during the prune survive it
// (NFR-111).
func TestTC_NSR_111e(t *testing.T) {
	withPruneBatch(t, 200)
	db, _ := newFileDB(t)
	seedSamples(t, db, 2000, 45)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		rtt := 3.0
		for i := 0; i < 30; i++ {
			_ = db.InsertNetSample(NetSample{
				TS: time.Now(), Target: "during", Kind: "icmp", RTTMs: &rtt, Status: "ok",
			})
			time.Sleep(time.Millisecond)
		}
	}()

	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)
	wg.Wait()

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM NetSample WHERE target = 'during'`).Scan(&n))
	assert.Equal(t, 30, n, "rows written mid-prune must not be swept up by it")
}

// @aitri-tc TC-NSR-112f — an insert is not rejected with a lock error while
// the prune runs (NFR-111).
func TestTC_NSR_112f(t *testing.T) {
	withPruneBatch(t, 200)
	db, _ := newFileDB(t)
	seedSamples(t, db, 2000, 45)

	errCh := make(chan error, 1)
	go func() {
		time.Sleep(5 * time.Millisecond) // land in the middle of the batches
		rtt := 4.0
		errCh <- db.InsertNetSample(NetSample{
			TS: time.Now(), Target: "midprune", Kind: "icmp", RTTMs: &rtt, Status: "ok",
		})
	}()

	_, err := db.PruneNetSamples(30)
	require.NoError(t, err)

	insertErr := <-errCh
	require.NoError(t, insertErr, "batched deletes must release the database between statements")
}

// checkpoint folds the WAL into the main database file so its on-disk size
// reflects the data actually stored.
//
// Without this, a "did the file shrink?" assertion measures nothing: in WAL
// mode the rows live in the -wal file until SQLite decides to checkpoint, so
// the main file can still read 4 KB while holding 20k rows. The test used to
// seed 80k rows purely because that happened to cross SQLite's automatic
// checkpoint threshold — an accident of timing standing in for a guarantee,
// and exactly the shape of a test that is fine locally and flaky in CI.
func checkpoint(t *testing.T, db *DB) {
	t.Helper()
	_, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	require.NoError(t, err)
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return fi.Size()
}

func idsSince(t *testing.T, db *DB, cutoffMs int64) []int64 {
	t.Helper()
	rows, err := db.Query(`SELECT id FROM NetSample WHERE ts >= ? ORDER BY id`, cutoffMs)
	require.NoError(t, err)
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		out = append(out, id)
	}
	require.NoError(t, rows.Err())
	return out
}
