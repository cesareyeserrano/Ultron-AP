package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cesareyeserrano/ultron-ap/internal/pironman"
	"github.com/stretchr/testify/assert"
)

func TestParseHardwareConfig_CheckboxOffByDefault(t *testing.T) {
	form := url.Values{
		"rgb_color":      {"#00ff00"},
		"rgb_brightness": {"75"},
		"rgb_style":      {"solid"},
		"rgb_speed":      {"20"},
		"fan_mode":       {"2"},
		"fan_led":        {"follow"},
		"oled_rotation":  {"180"},
		"oled_sleep":     {"30"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hardware/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cfg := parseHardwareConfig(req)
	assert.False(t, cfg.RGBEnable)
	assert.False(t, cfg.OLEDEnable)
	assert.Equal(t, "00ff00", cfg.RGBColor)
	assert.Equal(t, 180, cfg.OLEDRotation)
}

func TestParseHardwareConfig_CheckboxOn(t *testing.T) {
	form := url.Values{
		"rgb_enable":  {"on"},
		"oled_enable": {"on"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hardware/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cfg := parseHardwareConfig(req)
	assert.True(t, cfg.RGBEnable)
	assert.True(t, cfg.OLEDEnable)
}

func TestHardwareApply_RequiresCSRF(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {"wrong-token"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hardware/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHardwareApply_AcceptsValidCSRFThenChecksAvailability(t *testing.T) {
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token": {session.CSRFToken},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hardware/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	// If CSRF passes, next gate in this environment is pironman availability.
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestRenderHardwareContent_IncludesCSRFToken(t *testing.T) {
	srv := setupTestServer(t)
	html := srv.renderHardwareContent(&pironman.Config{RGBColor: "00ff00"}, "csrf-123")
	assert.Contains(t, html, `name="csrf_token" value="csrf-123"`)
}
