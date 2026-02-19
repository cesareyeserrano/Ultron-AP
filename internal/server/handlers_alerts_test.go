package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

func seedTestAlerts(t *testing.T, db *database.DB) {
	t.Helper()
	alerts := []database.Alert{
		{Severity: "critical", Message: "CPU at 95%", Source: "cpu", Acknowledged: false},
		{Severity: "warning", Message: "RAM at 87%", Source: "ram", Acknowledged: false},
		{Severity: "info", Message: "Disk check OK", Source: "disk", Acknowledged: true},
	}
	for i := range alerts {
		v := 95.0 - float64(i)*8
		alerts[i].Value = &v
		require.NoError(t, db.CreateAlert(&alerts[i]))
	}
}

func TestAlertsPage_RendersPage(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestAlerts(t, srv.db)

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Alerts")
	assert.Contains(t, body, "CPU at 95%")
	assert.Contains(t, body, "RAM at 87%")
	assert.Contains(t, body, "2 unacknowledged")
}

func TestAlertsPage_RequiresAuth(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

func TestAlertsPage_FilterBySeverity(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestAlerts(t, srv.db)

	req := httptest.NewRequest(http.MethodGet, "/alerts?severity=critical", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "CPU at 95%")
	assert.NotContains(t, body, "RAM at 87%")
}

func TestAlertsPage_InvalidSeverityShowsAll(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestAlerts(t, srv.db)

	req := httptest.NewRequest(http.MethodGet, "/alerts?severity=invalid", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "CPU at 95%")
	assert.Contains(t, body, "RAM at 87%")
}

func TestAlertsPage_EmptyState(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "No alerts")
}

func TestAlertAcknowledge(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestAlerts(t, srv.db)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/1/acknowledge", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify alert is acknowledged
	alerts, _ := srv.db.ListAlerts(100)
	for _, a := range alerts {
		if a.ID == 1 {
			assert.True(t, a.Acknowledged)
		}
	}
}

func TestAlertAcknowledge_RequiresCSRF(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestAlerts(t, srv.db)

	form := url.Values{"csrf_token": {"wrong-token"}}
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/1/acknowledge", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestAlertAcknowledge_InvalidID(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/abc/acknowledge", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAlertAcknowledge_DecreasesUnackCount(t *testing.T) {
	srv, session := setupSSETestServer(t)
	seedTestAlerts(t, srv.db)

	// Initial count should be 2
	count, _ := srv.db.UnacknowledgedAlertCount()
	assert.Equal(t, 2, count)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/1/acknowledge", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Count should now be 1
	count, _ = srv.db.UnacknowledgedAlertCount()
	assert.Equal(t, 1, count)
}
