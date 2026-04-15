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

func TestNotificationSave_RejectsOriginMismatch(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {session.CSRFToken},
		"bot_token":  {"token"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/notifications/telegram", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://evil.local")
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

func TestIsSameOriginRequest_AllowsWhenHeadersMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/performance", nil)
	req.Host = "example.local"
	assert.True(t, isSameOriginRequest(req))
}

func TestIsSameOriginRequest_AllowsMatchingOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/performance", nil)
	req.Host = "example.local"
	req.Header.Set("Origin", "http://example.local")
	assert.True(t, isSameOriginRequest(req))
}

func TestIsSameOriginRequest_RejectsMismatchedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/performance", nil)
	req.Host = "example.local"
	req.Header.Set("Origin", "http://evil.local")
	assert.False(t, isSameOriginRequest(req))
}

func TestIsSameOriginRequest_AllowsProxyHTTPSOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/performance", nil)
	req.Host = "example.local"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://example.local")
	assert.True(t, isSameOriginRequest(req))
}

func TestBackupConfigSave_RequiresCSRF(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {"wrong-token"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestBackupConfigSave_PersistsAndApplies(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token":         {session.CSRFToken},
		"enabled":            {"on"},
		"interval_hours":     {"12"},
		"retention_count":    {"9"},
		"schedule_mode":      {"weekly"},
		"schedule_hour":      {"2"},
		"schedule_minute":    {"30"},
		"destination_mode":   {"local_plus_telegram"},
		"local_path":         {"/tmp/ultron-backups"},
		"encrypt_enabled":    {"on"},
		"encryption_key_ref": {"env:ULTRON_BACKUP_KEY"},
		"upload_timeout_sec": {"45"},
		"max_upload_size_mb": {"80"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	cfg, err := srv.db.GetBackupConfig()
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 12, cfg.IntervalHours)
	assert.Equal(t, 9, cfg.RetentionCount)
	assert.Equal(t, "weekly", cfg.ScheduleMode)
	assert.Equal(t, 2, cfg.ScheduleHour)
	assert.Equal(t, 30, cfg.ScheduleMinute)
	assert.Equal(t, "local_plus_telegram", cfg.DestinationMode)
	assert.Equal(t, "/tmp/ultron-backups", cfg.LocalPath)
	assert.True(t, cfg.EncryptEnabled)
	assert.Equal(t, "env:ULTRON_BACKUP_KEY", cfg.EncryptionKeyRef)
	assert.Equal(t, 45, cfg.UploadTimeoutSec)
	assert.Equal(t, 80, cfg.MaxUploadSizeMB)
}

func TestBackupConfigSave_EnabledWithoutOtherFields(t *testing.T) {
	// Regression: checkbox-only submit (no other fields) must persist enabled=true.
	// The bug was that setFormBusy() disabled checkboxes before HTMX serialized the
	// form, causing the "enabled" field to be absent and saved as false.
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {session.CSRFToken},
		"enabled":    {"on"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	cfg, err := srv.db.GetBackupConfig()
	require.NoError(t, err)
	assert.True(t, cfg.Enabled)
}

func TestBackupConfigSave_DisabledPersists(t *testing.T) {
	// Regression: submitting without "enabled" (unchecked checkbox) must save false.
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {session.CSRFToken},
		// "enabled" intentionally absent — unchecked checkbox
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	cfg, err := srv.db.GetBackupConfig()
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
}

func TestRuntimeDiagnosticsRouteNotFound(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/diagnostics", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestLegacyIntegrationDiagnosticsRouteNotFound(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings/integrations/diagnostics", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
