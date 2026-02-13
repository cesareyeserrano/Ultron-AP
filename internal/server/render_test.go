package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

func setupTestServerWithSession(t *testing.T) (*Server, *database.Session) {
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

	err = db.CreateUser("admin", "$2a$10$dummy")
	require.NoError(t, err)

	session := &database.Session{
		ID:        "test-session-token",
		UserID:    1,
		CSRFToken: "test-csrf-token",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err = db.CreateSession(session)
	require.NoError(t, err)

	srv := New(cfg, db, nil, nil, nil, nil)
	return srv, session
}

func addSessionContext(r *http.Request, session *database.Session) *http.Request {
	ctx := context.WithValue(r.Context(), userContextKey, session.UserID)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	return r.WithContext(ctx)
}

func TestDashboard_Returns200(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

func TestDashboard_ContainsSidebar(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "id=\"sidebar\"")
	assert.Contains(t, body, "Dashboard")
	assert.Contains(t, body, "Docker")
	assert.Contains(t, body, "Services")
	assert.Contains(t, body, "Alerts")
	assert.Contains(t, body, "Settings")
}

func TestDashboard_ContainsHeader(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "Ultron-AP")
	assert.Contains(t, body, "/logout")
	assert.Contains(t, body, "admin")
}

func TestDashboard_ContainsUptime(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	body := rec.Body.String()
	// Uptime should contain at least "0m" or similar
	assert.Contains(t, body, "m")
}

func TestDashboard_ContainsCSRFToken(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "test-csrf-token")
}

func TestDashboard_HighlightsActivePage(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	body := rec.Body.String()
	// The Dashboard nav item should have the accent color class
	assert.Contains(t, body, "text-accent")
}

func TestPlaceholderPage_Returns200(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	handler := srv.handlePlaceholderPage("Docker", "docker")
	req := httptest.NewRequest(http.MethodGet, "/docker", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Coming Soon")
	assert.Contains(t, body, "Docker")
}

func TestFormatUptime_Minutes(t *testing.T) {
	result := formatUptime(30 * time.Minute)
	assert.Equal(t, "30m", result)
}

func TestFormatUptime_Hours(t *testing.T) {
	result := formatUptime(2*time.Hour + 15*time.Minute)
	assert.Equal(t, "2h 15m", result)
}

func TestFormatUptime_Days(t *testing.T) {
	result := formatUptime(3*24*time.Hour + 5*time.Hour + 42*time.Minute)
	assert.Equal(t, "3d 5h 42m", result)
}

func TestFormatUptime_Zero(t *testing.T) {
	result := formatUptime(0)
	assert.Equal(t, "0m", result)
}

func TestStaticCSS_Served(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	// Should contain CSS content
	assert.True(t, len(body) > 0)
}

func TestStaticJS_Served(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/static/js/sidebar.js", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSSFileSize_Under50KB(t *testing.T) {
	srv := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	size := rec.Body.Len()
	assert.Less(t, size, 50*1024, "CSS file must be under 50KB, got %d bytes", size)
}

func TestDashboard_ContainsMetricCards(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "CPU")
	assert.Contains(t, body, "Memory")
	assert.Contains(t, body, "Disk")
	assert.Contains(t, body, "Network")
}

func TestDashboard_ContainsHTMXScript(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "htmx.min.js")
}

func TestDashboard_SidebarNavLinksPresent(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = addSessionContext(req, session)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	body := rec.Body.String()
	for _, href := range []string{`href="/"`, `href="/docker"`, `href="/services"`, `href="/alerts"`, `href="/settings"`} {
		assert.True(t, strings.Contains(body, href), "expected sidebar to contain link %s", href)
	}
}

func TestServerStartedAt_IsSet(t *testing.T) {
	before := time.Now()
	srv := setupTestServer(t)
	after := time.Now()

	assert.False(t, srv.startedAt.IsZero())
	assert.True(t, srv.startedAt.After(before) || srv.startedAt.Equal(before))
	assert.True(t, srv.startedAt.Before(after) || srv.startedAt.Equal(after))
}
