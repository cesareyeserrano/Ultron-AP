package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
