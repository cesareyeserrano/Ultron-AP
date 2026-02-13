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

func TestSettings_RendersPage(t *testing.T) {
	srv, session := setupSSETestServer(t)

	// Seed some rules
	srv.db.SeedDefaultAlertConfigs()

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Settings")
	assert.Contains(t, body, "Alert Rules")
	assert.Contains(t, body, "High CPU")
	assert.Contains(t, body, "Telegram")
}

func TestSettings_RequiresAuth(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusSeeOther, rec.Code) // Redirects to login
}

func TestAlertRuleCreate(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {session.CSRFToken},
		"name":       {"Test Rule"},
		"metric":     {"cpu"},
		"operator":   {">"},
		"threshold":  {"85"},
		"severity":   {"warning"},
		"cooldown":   {"10"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/rules", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	rules, _ := srv.db.ListAlertConfigs()
	assert.Len(t, rules, 1)
	assert.Equal(t, "Test Rule", rules[0].Name)
	assert.Equal(t, 85.0, rules[0].Threshold)
}

func TestAlertRuleCreate_InvalidThreshold(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {session.CSRFToken},
		"metric":     {"cpu"},
		"operator":   {">"},
		"threshold":  {"not-a-number"},
		"severity":   {"warning"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/rules", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAlertRuleCreate_InvalidMetric(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {session.CSRFToken},
		"metric":     {"invalid"},
		"operator":   {">"},
		"threshold":  {"90"},
		"severity":   {"warning"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/rules", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAlertRuleToggle(t *testing.T) {
	srv, session := setupSSETestServer(t)

	ac := &database.AlertConfig{Name: "Test", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, srv.db.CreateAlertConfig(ac))

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/rules/1/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	got, _ := srv.db.GetAlertConfig(1)
	assert.False(t, got.Enabled)
}

func TestAlertRuleDelete(t *testing.T) {
	srv, session := setupSSETestServer(t)

	ac := &database.AlertConfig{Name: "Test", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, srv.db.CreateAlertConfig(ac))

	req := httptest.NewRequest(http.MethodDelete, "/api/alerts/rules/1", nil)
	req.Header.Set("X-CSRF-Token", session.CSRFToken)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	got, _ := srv.db.GetAlertConfig(1)
	assert.Nil(t, got)
}

func TestNotificationSave_Telegram(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {session.CSRFToken},
		"bot_token":  {"123456:ABC-DEF"},
		"chat_id":    {"789"},
		"enabled":    {"on"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/telegram", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Saved successfully")

	got, _ := srv.db.GetNotificationConfig("telegram")
	require.NotNil(t, got)
	assert.True(t, got.Enabled)
	assert.Contains(t, got.Config, "123456:ABC-DEF")
}

func TestNotificationSave_InvalidChannel(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/notifications/invalid", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestNotificationSave_RequiresCSRF(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {"wrong-token"},
		"bot_token":  {"test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/telegram", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestMaskNotifConfig_MasksSensitiveFields(t *testing.T) {
	nc := &database.NotificationConfig{
		Channel: "telegram",
		Enabled: true,
		Config:  `{"bot_token":"123456789:ABCdefGHIjklMNO","chat_id":"12345"}`,
	}

	nd := maskNotifConfig(nc, "telegram")
	assert.True(t, nd.Enabled)
	assert.Equal(t, "12345", nd.Fields["chat_id"]) // Not sensitive
	assert.NotEqual(t, "123456789:ABCdefGHIjklMNO", nd.Fields["bot_token"])
	assert.True(t, strings.HasSuffix(nd.Fields["bot_token"], "lMNO")) // Last 4 visible
}

func TestValidation_Helpers(t *testing.T) {
	assert.True(t, isValidMetric("cpu"))
	assert.True(t, isValidMetric("ram"))
	assert.True(t, isValidMetric("disk"))
	assert.True(t, isValidMetric("temp"))
	assert.False(t, isValidMetric("network"))

	assert.True(t, isValidOperator(">"))
	assert.True(t, isValidOperator(">="))
	assert.False(t, isValidOperator("!="))

	assert.True(t, isValidSeverity("critical"))
	assert.True(t, isValidSeverity("warning"))
	assert.True(t, isValidSeverity("info"))
	assert.False(t, isValidSeverity("high"))
}
