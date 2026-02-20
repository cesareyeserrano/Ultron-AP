package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	systemdpkg "github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

// mockCommandRunner is a test double for systemd.CommandRunner.
type mockCommandRunner struct {
	output []byte
	err    error
}

func (m *mockCommandRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	// For list-units, return a minimal valid output so refresh works.
	if len(args) > 0 && args[0] == "list-units" {
		return m.output, nil
	}
	return m.output, m.err
}

// setupServiceTestServer creates a test server with a mock systemd runner.
func setupServiceTestServer(t *testing.T, runner *mockCommandRunner) (*Server, *database.Session) {
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
		ID:        "svc-test-session",
		UserID:    1,
		CSRFToken: "test-csrf",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.CreateSession(session))

	// Pre-seed runner output with sample services so refresh populates cache.
	systemdMon := systemdpkg.NewMonitorWithRunner(runner)
	ctx, cancel := context.WithCancel(context.Background())
	systemdMon.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	t.Cleanup(func() { cancel(); systemdMon.Stop() })

	srv := New(cfg, db, nil, nil, nil, systemdMon, nil)
	return srv, session
}

// listUnitsOutput returns a minimal systemctl list-units output for two services.
func listUnitsOutput() []byte {
	return []byte("" +
		"nginx.service              loaded active running nginx web server\n" +
		"ssh.service                loaded inactive dead   openssh server daemon\n" +
		"fail2ban.service           loaded failed  failed  fail2ban service\n",
	)
}

// --- Services Page Tests ---

func TestServicesPage_RendersServices(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Systemd Services")
}

func TestServicesPage_RequiresAuth(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, _ := setupServiceTestServer(t, runner)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

// --- Control Handler Tests ---

func TestServiceStart_RequiresCSRF(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {"wrong-token"}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/ssh.service/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestServiceStart_RequiresAuth(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, _ := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {"test-csrf"}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/ssh.service/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestServiceStart_Success(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/ssh.service/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestServiceStart_LogsAction(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/ssh.service/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	srv.httpServer.Handler.ServeHTTP(httptest.NewRecorder(), req)

	logs, err := srv.db.ListActionLogs(10)
	require.NoError(t, err)
	require.NotEmpty(t, logs)
	assert.Equal(t, "start", logs[0].Action)
	assert.Equal(t, "success", logs[0].Result)
}

func TestServiceStop_Success(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/nginx.service/stop", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestServiceRestart_Success(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/nginx.service/restart", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestServiceStart_InvalidNameRejected(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	// Name contains semicolon — invalid per validServiceName regex
	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/bad%3Bname.service/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	// Errors return 200 with an inline error banner (HTMX swaps normally)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid service name")

	logs, _ := srv.db.ListActionLogs(10)
	require.NotEmpty(t, logs)
	assert.Equal(t, "error", logs[0].Result)
}

func TestServiceStart_ErrorLogged(t *testing.T) {
	runner := &mockCommandRunner{
		output: listUnitsOutput(),
		err:    fmt.Errorf("Failed to start ssh.service: Unit not found"),
	}
	srv, session := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/ssh.service/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	// Errors return 200 with an inline error banner (HTMX swaps normally)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Failed to start")

	logs, _ := srv.db.ListActionLogs(10)
	require.NotEmpty(t, logs)
	assert.Equal(t, "error", logs[0].Result)
}
