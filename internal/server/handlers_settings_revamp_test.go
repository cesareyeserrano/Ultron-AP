// @aitri-tc TC-SR-057f, TC-SR-057f2, TC-SR-064h, TC-SR-064e, TC-SR-064f,
// TC-SR-NFR029h, TC-SR-NFR029e, TC-SR-NFR029f
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

// TC-SR-057f — POSTing dashboard_refresh_sec=999 returns 400 with the same
// hint string used in the visible label.
func TestPerformanceSave_OutOfRange_Returns400WithHint(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token":       {session.CSRFToken},
		"sse_interval_sec": {"999"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/performance", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "2–60 sec", "error body must contain the same range string used in the label")
	assert.Contains(t, body, "Dashboard refresh", "error body must reference the field by its visible label")
}

// TC-SR-057f2 — value below min returns 400 with the same hint.
func TestPerformanceSave_BelowMin_Returns400WithHint(t *testing.T) {
	srv, session := setupSSETestServer(t)
	form := url.Values{
		"csrf_token":       {session.CSRFToken},
		"sse_interval_sec": {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/performance", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "2–60 sec")
}

// Boundary OK — valid min and valid max accepted.
func TestPerformanceSave_BoundaryValues_Accepted(t *testing.T) {
	for _, val := range []string{"2", "60"} {
		t.Run(val, func(t *testing.T) {
			srv, session := setupSSETestServer(t)
			form := url.Values{
				"csrf_token":       {session.CSRFToken},
				"sse_interval_sec": {val},
			}
			req := httptest.NewRequest(http.MethodPost, "/api/performance", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
			rec := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, "value %s on boundary must be accepted", val)
		})
	}
}

// TC-SR-064h — backup config accepts the new `time=HH:MM` form.
func TestBackupConfigSave_NewTimeForm_Accepted(t *testing.T) {
	srv, session := setupSSETestServer(t)
	form := url.Values{
		"csrf_token": {session.CSRFToken},
		"enabled":    {"on"},
		"time":       {"02:30"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	cfg, err := srv.db.GetBackupConfig()
	require.NoError(t, err)
	assert.Equal(t, 2, cfg.ScheduleHour)
	assert.Equal(t, 30, cfg.ScheduleMinute)
}

// TC-SR-064e — when both `time=HH:MM` and legacy `hour/minute` are present,
// the new format wins.
func TestBackupConfigSave_BothForms_NewWins(t *testing.T) {
	srv, session := setupSSETestServer(t)
	form := url.Values{
		"csrf_token":      {session.CSRFToken},
		"enabled":         {"on"},
		"time":            {"04:30"},
		"schedule_hour":   {"9"},
		"schedule_minute": {"15"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	cfg, err := srv.db.GetBackupConfig()
	require.NoError(t, err)
	assert.Equal(t, 4, cfg.ScheduleHour, "new time= must win")
	assert.Equal(t, 30, cfg.ScheduleMinute)
}

// TC-SR-064f — POST `time=25:00` returns 400 with the canonical message.
func TestBackupConfigSave_OutOfRangeTime_Returns400(t *testing.T) {
	srv, session := setupSSETestServer(t)
	form := url.Values{
		"csrf_token": {session.CSRFToken},
		"enabled":    {"on"},
		"time":       {"25:00"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "time must be HH:MM in 00:00..23:59")
}

// TC-SR-NFR029f — legacy `hour=24` returns 400 (out-of-range still rejected).
func TestBackupConfigSave_LegacyHourOutOfRange_Returns400(t *testing.T) {
	srv, session := setupSSETestServer(t)
	form := url.Values{
		"csrf_token":    {session.CSRFToken},
		"enabled":       {"on"},
		"schedule_hour": {"24"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/backup/config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "hour must be 0..23")
}
