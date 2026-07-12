package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func setupTestServer(t *testing.T) *Server {
	t.Helper()

	cfg := &config.Config{
		Port:     8080,
		DBPath:   filepath.Join(t.TempDir(), "test.db"),
		LogLevel: "info",
	}

	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return New(cfg, db, nil, nil, nil, nil, nil)
}

func TestHealthEndpoint_Returns200(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHealthEndpoint_ReturnsJSON(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

func TestHealthEndpoint_SetsSecurityHeaders(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
	assert.NotEmpty(t, rec.Header().Get("Content-Security-Policy-Report-Only"))
}

func TestHealthEndpoint_PostNotAllowed(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestNewServer_SetsAddr(t *testing.T) {
	cfg := &config.Config{
		Port:     9090,
		DBPath:   filepath.Join(t.TempDir(), "test.db"),
		LogLevel: "info",
	}

	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	defer db.Close()

	srv := New(cfg, db, nil, nil, nil, nil, nil)
	assert.Equal(t, ":9090", srv.httpServer.Addr)
}

func TestRecordBackupOutcome_LogsAction(t *testing.T) {
	srv := setupTestServer(t)

	srv.recordBackupOutcome(errors.New("telegram send failed"))
	srv.recordBackupOutcome(nil)

	logs, err := srv.db.ListActionLogs(10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(logs), 2)

	assert.Equal(t, "backup", logs[0].Source)
	assert.Equal(t, "automated", logs[0].Action)
	assert.Contains(t, []string{"success", "error"}, logs[0].Result)
}

func TestPerformAutomatedBackup_RetentionRunsOnTelegramError(t *testing.T) {
	srv := setupTestServer(t)
	srv.ApplyBackupConfig(database.BackupConfig{
		Enabled:         true,
		IntervalHours:   24,
		RetentionCount:  7,
		DestinationMode: "local_plus_telegram",
	})

	backupDir := filepath.Join(filepath.Dir(srv.cfg.DBPath), "backups")
	require.NoError(t, os.MkdirAll(backupDir, 0o755))
	for i := 0; i < 10; i++ {
		p := filepath.Join(backupDir, fmt.Sprintf("ultron-20000101-0000%02d.db", i))
		require.NoError(t, os.WriteFile(p, []byte("old"), 0o644))
	}

	err := srv.performAutomatedBackup()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "telegram"))

	files, readErr := os.ReadDir(backupDir)
	require.NoError(t, readErr)
	assert.LessOrEqual(t, len(files), 7)
}

func TestNextBackupDelay_Daily(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 15, 0, 0, time.UTC)
	delay := nextBackupDelay(now, true, "daily", 24, 11, 0)
	assert.Equal(t, 45*time.Minute, delay)
}

func TestNextBackupDelay_Weekly(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 15, 0, 0, time.UTC)
	delay := nextBackupDelay(now, true, "weekly", 24, 9, 0)
	assert.Equal(t, 6*24*time.Hour+22*time.Hour+45*time.Minute, delay)
}

func TestNextBackupDelay_Biweekly(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 15, 0, 0, time.UTC)
	delay := nextBackupDelay(now, true, "biweekly", 24, 10, 15)
	assert.Equal(t, 14*24*time.Hour, delay)
}

func TestApplyBackupConfig_RequestsReschedule(t *testing.T) {
	srv := &Server{
		backupRescheduleCh: make(chan struct{}, 1),
	}
	cfg := database.DefaultBackupConfig()
	cfg.ScheduleMode = "daily"
	cfg.ScheduleHour = 22
	cfg.ScheduleMinute = 30

	srv.ApplyBackupConfig(cfg)

	select {
	case <-srv.backupRescheduleCh:
		// expected: config apply should ask scheduler to recompute immediately.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected backup reschedule signal after ApplyBackupConfig")
	}
}

func TestRequestBackupReschedule_DoesNotBlockWhenChannelFull(t *testing.T) {
	srv := &Server{
		backupRescheduleCh: make(chan struct{}, 1),
	}
	// Fill the channel so the next request takes the default non-blocking path.
	srv.backupRescheduleCh <- struct{}{}

	done := make(chan struct{})
	go func() {
		srv.requestBackupReschedule()
		close(done)
	}()

	select {
	case <-done:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("requestBackupReschedule blocked with full channel")
	}
}

// D1 — ApplyBackupConfig swaps a whole immutable snapshot; readers see a
// consistent set, and the "keep old value on invalid input" cases preserve
// prior values rather than resetting.
func TestApplyBackupConfig_AtomicSnapshotAndPreserve(t *testing.T) {
	srv := setupTestServer(t)

	srv.ApplyBackupConfig(database.BackupConfig{
		Enabled:          true,
		IntervalHours:    12,
		RetentionCount:   5,
		DestinationMode:  "local_plus_telegram",
		LocalPath:        "/srv/backups",
		ScheduleMode:     "daily",
		ScheduleHour:     2,
		ScheduleMinute:   30,
		EncryptEnabled:   true,
		EncryptionKeyRef: "env:K",
		UploadTimeoutSec: 45,
		MaxUploadSizeMB:  80,
	})

	bs := srv.currentBackupSettings()
	assert.True(t, bs.enabled)
	assert.Equal(t, 12, bs.intervalHours)
	assert.Equal(t, 5, bs.retention)
	assert.Equal(t, "local_plus_telegram", bs.destination)
	assert.Equal(t, "/srv/backups", bs.localPath)
	assert.True(t, bs.encrypt)
	assert.Equal(t, "env:K", bs.keyRef)
	assert.Equal(t, 45, bs.uploadTimeout)
	assert.Equal(t, 80, bs.maxUploadMB)

	// Invalid interval/retention (< 1) must preserve the prior values.
	srv.ApplyBackupConfig(database.BackupConfig{
		Enabled:        true,
		IntervalHours:  0,
		RetentionCount: 0,
		ScheduleMode:   "interval",
	})
	bs2 := srv.currentBackupSettings()
	assert.Equal(t, 12, bs2.intervalHours, "invalid interval must preserve prior value")
	assert.Equal(t, 5, bs2.retention, "invalid retention must preserve prior value")
	assert.Equal(t, "interval", bs2.scheduleMode)
}
