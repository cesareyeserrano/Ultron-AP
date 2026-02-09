package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

	return New(cfg, db)
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

	srv := New(cfg, db)
	assert.Equal(t, ":9090", srv.httpServer.Addr)
}
