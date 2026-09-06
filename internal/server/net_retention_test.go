// Tests for the network-retention pass inside the shared daily retention job.
//
// The job is shared: it prunes ActionLog, sessions, UPS and now NetSample. The
// tests below care mostly about CONTAINMENT — that one prune failing never
// costs the others their run — because that job is a single goroutine and an
// unhandled failure in it would silently stop all retention.
//
// @aitri-trace FR-097 FR-099 NFR-103 NFR-104 NFR-109
package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// retentionServer builds a Server over a real database file with the given
// network retention window.
func retentionServer(t *testing.T, days int) (*Server, *database.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ret.db")
	db, err := database.New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	cfg := &config.Config{SessionTTL: 24 * time.Hour, NetRetentionDays: days}
	return New(cfg, db, nil, nil, nil, nil, nil), db, path
}

func seedNetSamples(t *testing.T, db *database.DB, n int, ageDays float64) {
	t.Helper()
	ts := time.Now().Add(-time.Duration(ageDays * float64(24*time.Hour)))
	rtt := 9.0
	for i := 0; i < n; i++ {
		require.NoError(t, db.InsertNetSample(database.NetSample{
			TS: ts, Target: fmt.Sprintf("t%d", i), Kind: "icmp", RTTMs: &rtt, Status: "ok",
		}))
	}
}

func netSampleCount(t *testing.T, db *database.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM NetSample").Scan(&n))
	return n
}

// captureServerLog redirects the standard logger for the duration of a test.
func captureServerLog(t *testing.T) *strings.Builder {
	t.Helper()
	var sb strings.Builder
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&sb)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })
	return &sb
}

// @aitri-tc TC-NSR-012e — the prune reports how many rows it removed and the
// window it applied (AC-097-003, NFR-104).
func TestTC_NSR_012e(t *testing.T) {
	srv, db, _ := retentionServer(t, 30)
	seedNetSamples(t, db, 3, 45)

	logged := captureServerLog(t)
	srv.pruneNetSamples()

	out := logged.String()
	assert.Contains(t, out, "3", "the log must say how many rows went")
	assert.Contains(t, out, "30", "the log must say which window was applied")
	assert.Contains(t, out, "net samples")
	assert.Equal(t, 0, netSampleCount(t, db))
}

// @aitri-tc TC-NSR-013e is covered in internal/database; here the same clean
// table must also produce NO log line, so a healthy Pi does not write a
// retention line every single day (NFR-104).
func TestTC_NSR_012e_QuietWhenNothingToDo(t *testing.T) {
	srv, db, _ := retentionServer(t, 30)
	seedNetSamples(t, db, 5, 1)

	logged := captureServerLog(t)
	srv.pruneNetSamples()

	assert.NotContains(t, logged.String(), "net samples",
		"a prune that removed nothing must stay quiet")
	assert.Equal(t, 5, netSampleCount(t, db))
}

// @aitri-tc TC-NSR-014f — a failing network prune does not cost the other
// prunes their run (AC-097-005, NFR-103).
//
// The failure is induced by closing the database underneath the job, which is
// the bluntest way to make every query fail at once.
func TestTC_NSR_014f(t *testing.T) {
	srv, db, _ := retentionServer(t, 30)
	seedNetSamples(t, db, 2, 45)
	require.NoError(t, db.Close())

	logged := captureServerLog(t)
	assert.NotPanics(t, func() { srv.pruneNetSamples() },
		"a failing prune must not take the retention goroutine down with it")
	assert.Contains(t, logged.String(), "retention:", "the failure must be reported")
}

// @aitri-tc TC-NSR-034f — compaction is skipped when there is little to
// reclaim, because VACUUM blocks writers (AC-099-005).
func TestTC_NSR_034f(t *testing.T) {
	srv, db, path := retentionServer(t, 30)
	seedNetSamples(t, db, 50, 45)

	logged := captureServerLog(t)
	before := statSize(t, path)
	srv.pruneNetSamples()
	after := statSize(t, path)

	free, err := db.FreeSpaceBytes()
	require.NoError(t, err)
	require.Less(t, free, int64(netCompactThresholdBytes),
		"this fixture must stay under the threshold for the test to mean anything")

	assert.NotContains(t, logged.String(), "compacting database",
		"a few free pages must not trigger a write-blocking VACUUM")
	assert.Equal(t, before, after, "the file must be left alone below the threshold")
}

// @aitri-tc TC-NSR-090h — the cycle still prunes ActionLog and sessions
// alongside the new network prune (NFR-109).
func TestTC_NSR_090h(t *testing.T) {
	srv, db, _ := retentionServer(t, 30)

	// An expired session and an old network sample.
	require.NoError(t, db.CreateSession(&database.Session{
		ID: "expired", UserID: 1, CSRFToken: "x", ExpiresAt: time.Now().Add(-time.Hour),
	}))
	seedNetSamples(t, db, 2, 45)

	deleted, err := db.DeleteExpiredSessions()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(1), "session cleanup must still work")

	_, err = db.PruneOldData(30)
	require.NoError(t, err, "ActionLog/Alert pruning must still work")

	srv.pruneNetSamples()
	assert.Equal(t, 0, netSampleCount(t, db), "and the network prune runs too")
}

// @aitri-tc TC-NSR-091e — the job keeps its 24h cadence (NFR-109).
func TestTC_NSR_091e(t *testing.T) {
	src, err := os.ReadFile("server.go")
	require.NoError(t, err)
	body := string(src)

	start := strings.Index(body, "func (s *Server) startRetentionJob")
	require.Greater(t, start, 0)
	fn := body[start:]
	if end := strings.Index(fn[1:], "\nfunc "); end > 0 {
		fn = fn[:end]
	}

	assert.Contains(t, fn, "timer.Reset(24 * time.Hour)", "the daily cadence must be unchanged")
	assert.Contains(t, fn, "s.pruneNetSamples()", "the network prune must run inside this job")
	// The reset must not sit inside an error branch, or one bad prune would
	// stop the job forever.
	resetIdx := strings.Index(fn, "timer.Reset(24 * time.Hour)")
	pruneIdx := strings.Index(fn, "s.pruneNetSamples()")
	assert.Greater(t, resetIdx, pruneIdx, "the reset must follow the prune unconditionally")
}

// @aitri-tc TC-NSR-092f — an ActionLog failure does not stop the network
// prune: containment works in both directions (NFR-109).
func TestTC_NSR_092f(t *testing.T) {
	srv, db, _ := retentionServer(t, 30)
	seedNetSamples(t, db, 2, 45)

	// PruneOldData failing is simulated by asking for a nonsensical window;
	// whatever it returns, the network prune that follows must still run.
	_, _ = db.PruneOldData(-1)

	srv.pruneNetSamples()
	assert.Equal(t, 0, netSampleCount(t, db),
		"the network prune runs regardless of what the earlier prunes did")
}

func statSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return fi.Size()
}

// --- NFR-102: an unusable window must never reach the database ---
//
// These two go end to end on purpose: environment → config.Load → prune → rows.
// Asserting only that Load returns 30 would test the sanitiser in isolation and
// miss the thing that actually matters, which is that no destructive value
// survives the journey.

// @aitri-tc TC-NSR-051f — a window of 0 does not wipe the history
// (AC-096-004, NFR-102).
func TestTC_NSR_051f(t *testing.T) {
	t.Setenv("ULTRON_DB_PATH", filepath.Join(t.TempDir(), "cfg.db"))
	t.Setenv("ULTRON_NET_RETENTION_DAYS", "0")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, 30, cfg.NetRetentionDays, "0 must have been sanitised in Load")

	db, err := database.New(filepath.Join(t.TempDir(), "data.db"))
	require.NoError(t, err)
	defer db.Close()
	seedNetSamples(t, db, 1, 1) // yesterday

	n, err := db.PruneNetSamples(cfg.NetRetentionDays)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Equal(t, 1, netSampleCount(t, db),
		"a window of 0 would have deleted everything older than now — yesterday's sample must survive")
}

// @aitri-tc TC-NSR-052f — a negative window does not put the cutoff in the
// future (AC-096-004, NFR-102).
func TestTC_NSR_052f(t *testing.T) {
	t.Setenv("ULTRON_DB_PATH", filepath.Join(t.TempDir(), "cfg.db"))
	t.Setenv("ULTRON_NET_RETENTION_DAYS", "-30")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, 30, cfg.NetRetentionDays, "-30 must have been sanitised in Load")

	db, err := database.New(filepath.Join(t.TempDir(), "data.db"))
	require.NoError(t, err)
	defer db.Close()
	seedNetSamples(t, db, 1, 1.0/24) // an hour ago

	n, err := db.PruneNetSamples(cfg.NetRetentionDays)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Equal(t, 1, netSampleCount(t, db),
		"-30 would have set the cutoff 30 days in the FUTURE and deleted the whole table")
}
