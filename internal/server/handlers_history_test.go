package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTestActionLogs(t *testing.T, srv *Server) {
	t.Helper()
	uid := int64(1)
	entries := []struct {
		source, action, target, result, details string
	}{
		{"docker", "start", "web-app", "success", "Container web-app started"},
		{"docker", "stop", "db", "error", "Failed to stop db: timeout"},
		{"systemd", "restart", "nginx.service", "success", "Service nginx.service restarted"},
		{"systemd", "start", "ssh.service", "success", "Service ssh.service started"},
	}
	for _, e := range entries {
		require.NoError(t, srv.db.LogAction(&uid, e.source, e.action, e.target, e.result, e.details))
	}
}

func TestHistoryPage_RendersLogs(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestActionLogs(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Action History")
	assert.Contains(t, body, "web-app")
	assert.Contains(t, body, "nginx.service")
}

func TestHistoryPage_RequiresAuth(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

func TestHistoryPage_FilterByDocker(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestActionLogs(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/history?source=docker", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "web-app")
	assert.NotContains(t, body, "nginx.service")
}

func TestHistoryPage_FilterBySystemd(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestActionLogs(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/history?source=systemd", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "nginx.service")
	assert.NotContains(t, body, "web-app")
}

func TestHistoryPage_InvalidSourceShowsAll(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestActionLogs(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/history?source=invalid", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "web-app")
	assert.Contains(t, body, "nginx.service")
}

func TestHistoryPage_EmptyState(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "No actions recorded")
}

func TestHistoryPage_ErrorIndicators(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestActionLogs(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Contains(t, rec.Body.String(), "Failed to stop db: timeout")
}

func TestHistoryClear_All(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestActionLogs(t, srv)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/history/clear", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	logs, err := srv.db.ListActionLogs(100)
	require.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, "history", logs[0].Source)
	assert.Equal(t, "clear", logs[0].Action)
}

func TestHistoryClear_HTMX(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestActionLogs(t, srv)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/history/clear", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("HX-Trigger"), "Cleared")
	assert.Contains(t, rec.Header().Get("HX-Redirect"), "/history")
}
