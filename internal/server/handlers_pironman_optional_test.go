package server

import (
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
)

func setupPironmanFeatureServer(t *testing.T) (*Server, *database.Session) {
	t.Helper()

	cfg := &config.Config{
		Port:            8080,
		DBPath:          filepath.Join(t.TempDir(), "test.db"),
		LogLevel:        "info",
		AdminUser:       "admin",
		AdminPass:       "secret",
		SessionTTL:      24 * time.Hour,
		FeaturePironman: true,
	}

	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	err = db.CreateUser("admin", "$2a$10$dummy")
	require.NoError(t, err)

	session := &database.Session{
		ID:        "test-pironman-session",
		UserID:    1,
		CSRFToken: "test-csrf",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err = db.CreateSession(session)
	require.NoError(t, err)

	srv := New(cfg, db, nil, nil, nil, nil, nil)
	return srv, session
}

func TestPironmanPage_NotRegisteredWhenFeatureDisabled(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/integrations/pironman", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "Pironman Integration")
}

func TestPironmanPage_RegisteredWhenFeatureEnabled(t *testing.T) {
	srv, session := setupPironmanFeatureServer(t)

	req := httptest.NewRequest(http.MethodGet, "/integrations/pironman", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Pironman Integration")
}

func TestPironmanApply_RequiresCSRF(t *testing.T) {
	srv, session := setupPironmanFeatureServer(t)

	form := url.Values{
		"csrf_token": {"wrong-token"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/pironman/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestParsePironmanConfig_SanitizesInvalidValues(t *testing.T) {
	form := url.Values{
		"rgb_color":     {"#ZZZZZZ"},
		"rgb_style":     {"invalid"},
		"fan_led":       {"invalid"},
		"oled_rotation": {"90"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/pironman/apply", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cfg := parsePironmanConfig(req)
	assert.Equal(t, "000000", cfg.RGBColor)
	assert.Equal(t, "solid", cfg.RGBStyle)
	assert.Equal(t, "follow", cfg.FanLED)
	assert.Equal(t, 0, cfg.OLEDRotation)
}
