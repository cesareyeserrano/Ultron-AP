package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	dockerpkg "github.com/cesareyeserrano/ultron-ap/internal/docker"
)

// mockDockerClient is a test double for docker.DockerClient.
type mockDockerClient struct {
	containers []types.Container
	startErr   error
	stopErr    error
	restartErr error
}

func (m *mockDockerClient) Ping(_ context.Context) (types.Ping, error) {
	return types.Ping{}, nil
}

func (m *mockDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
	return m.containers, nil
}

func (m *mockDockerClient) ContainerStats(_ context.Context, _ string, _ bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func (m *mockDockerClient) ContainerInspect(_ context.Context, _ string) (types.ContainerJSON, error) {
	return types.ContainerJSON{}, nil
}

func (m *mockDockerClient) ContainerStart(_ context.Context, _ string, _ container.StartOptions) error {
	return m.startErr
}

func (m *mockDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return m.stopErr
}

func (m *mockDockerClient) ContainerRestart(_ context.Context, _ string, _ container.StopOptions) error {
	return m.restartErr
}

func (m *mockDockerClient) ContainerLogs(_ context.Context, _ string, _ container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("sample logs")), nil
}

func (m *mockDockerClient) Close() error { return nil }

// setupDockerTestServer creates a test server with a mock Docker monitor seeded with containers.
func setupDockerTestServer(t *testing.T, client *mockDockerClient) (*Server, *database.Session) {
	t.Helper()

	cfg := &config.Config{
		Port:       8080,
		DBPath:     filepath.Join(t.TempDir(), "test.db"),
		LogLevel:   "info",
		AdminUser:  "admin",
		AdminPass:  "secret",
		SessionTTL: 24 * time.Hour,
	}

	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	require.NoError(t, db.CreateUser("admin", "$2a$10$dummy"))

	session := &database.Session{
		ID:        "docker-test-session",
		UserID:    1,
		CSRFToken: "test-csrf",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.CreateSession(session))

	dockerMon := dockerpkg.NewMonitorWithClient(client)
	// Start the monitor and wait for first refresh to populate the container cache.
	ctx, cancel := context.WithCancel(context.Background())
	dockerMon.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	t.Cleanup(func() { cancel(); dockerMon.Stop() })

	srv := New(cfg, db, nil, nil, dockerMon, nil, nil)
	return srv, session
}

func sampleDockerContainers() []types.Container {
	return []types.Container{
		{
			ID:    "abc123def456789012345678",
			Names: []string{"/web-app"},
			Image: "nginx:latest",
			State: "running",
		},
		{
			ID:    "def456ghi789012345678901",
			Names: []string{"/db"},
			Image: "postgres:16",
			State: "exited",
		},
	}
}

// --- Docker Page Tests ---

func TestDockerPage_RendersContainers(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, session := setupDockerTestServer(t, mock)

	req := httptest.NewRequest(http.MethodGet, "/docker", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Docker")
	assert.Contains(t, body, "web-app")
}

func TestDockerPage_RequiresAuth(t *testing.T) {
	mock := &mockDockerClient{}
	srv, _ := setupDockerTestServer(t, mock)

	req := httptest.NewRequest(http.MethodGet, "/docker", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

// --- Control Handler Tests ---

func TestDockerStart_RequiresCSRF(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, session := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {"wrong-token"}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/abc123def456789012345678/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestDockerStart_RequiresAuth(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, _ := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {"test-csrf"}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/abc123def456789012345678/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	// API endpoints return 401 (not a redirect) for unauthenticated requests
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestDockerStart_Success(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, session := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/def456ghi789012345678901/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDockerStart_LogsAction(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, session := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/def456ghi789012345678901/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	logs, err := srv.db.ListActionLogs(10)
	require.NoError(t, err)
	require.NotEmpty(t, logs)
	assert.Equal(t, "start", logs[0].Action)
	assert.Equal(t, "success", logs[0].Result)
}

func TestDockerStop_Success(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, session := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/abc123def456789012345678/stop", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDockerStop_RequiresCSRF(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, session := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {"bad-token"}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/abc123def456789012345678/stop", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestDockerStop_LogsAction(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, session := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/abc123def456789012345678/stop", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	srv.httpServer.Handler.ServeHTTP(httptest.NewRecorder(), req)

	logs, _ := srv.db.ListActionLogs(10)
	require.NotEmpty(t, logs)
	assert.Equal(t, "stop", logs[0].Action)
}

func TestDockerRestart_Success(t *testing.T) {
	mock := &mockDockerClient{containers: sampleDockerContainers()}
	srv, session := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/abc123def456789012345678/restart", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDockerStart_ErrorLogged(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleDockerContainers(),
		startErr:   fmt.Errorf("permission denied"),
	}
	srv, session := setupDockerTestServer(t, mock)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/docker/def456ghi789012345678901/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	logs, _ := srv.db.ListActionLogs(10)
	require.NotEmpty(t, logs)
	assert.Equal(t, "error", logs[0].Result)
}
