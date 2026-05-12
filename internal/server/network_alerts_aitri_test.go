package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func postAlertRuleForm(t *testing.T, srv *Server, sessionID string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/rules", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func TestTC_NA_071f(t *testing.T) {
	// @aitri-tc TC-NA-071f
	srv, session := setupSSETestServer(t)
	form := url.Values{
		"csrf_token":         {session.CSRFToken},
		"metric":             {"latency"},
		"target":             {"8.8.8.8; rm -rf /"},
		"operator":           {">"},
		"threshold":          {"100"},
		"sustained_duration": {"120"},
		"severity":           {"warning"},
		"cooldown":           {"15"},
	}
	rec := postAlertRuleForm(t, srv, session.ID, form)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "Invalid target\n", rec.Body.String())
	rules, err := srv.db.ListAlertConfigs()
	require.NoError(t, err)
	require.Empty(t, rules)
}

func TestTC_NA_072f(t *testing.T) {
	// @aitri-tc TC-NA-072f
	srv, session := setupSSETestServer(t)
	form := url.Values{
		"csrf_token":         {session.CSRFToken},
		"metric":             {"loss"},
		"target":             {""},
		"operator":           {">"},
		"threshold":          {"5"},
		"sustained_duration": {"60"},
		"severity":           {"warning"},
		"cooldown":           {"15"},
	}
	rec := postAlertRuleForm(t, srv, session.ID, form)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "Invalid target\n", rec.Body.String())
}

func TestTC_NA_077h(t *testing.T) {
	// @aitri-tc TC-NA-077h
	srv, session := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	require.Equal(t, http.StatusOK, rec.Code)
	values := []string{"cpu", "ram", "disk", "temp", "latency", "loss", "dns_failure_rate", "wan_outage", "public_ip_change"}
	last := -1
	for _, v := range values {
		idx := strings.Index(body, `<option value="`+v+`">`)
		require.NotEqual(t, -1, idx, v)
		require.Greater(t, idx, last)
		last = idx
	}
}

func TestTC_NA_077e(t *testing.T) {
	// @aitri-tc TC-NA-077e
	srv, session := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	require.Contains(t, body, `value="wan_outage"`)
	require.Contains(t, body, `data-rule-field="threshold"`)
	require.Contains(t, body, `setSeverity('critical', true)`)
	require.Contains(t, body, `setField(sustained, !transition)`)
	require.Contains(t, body, `name="cooldown"`)
}

func TestTC_NA_077f(t *testing.T) {
	// @aitri-tc TC-NA-077f
	srv, session := setupSSETestServer(t)
	form := url.Values{
		"csrf_token":         {session.CSRFToken},
		"name":               {"Gateway latency"},
		"metric":             {"latency"},
		"target":             {"gateway"},
		"operator":           {">"},
		"threshold":          {"100"},
		"sustained_duration": {"120"},
		"severity":           {"warning"},
		"cooldown":           {"15"},
	}
	rec := postAlertRuleForm(t, srv, session.ID, form)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "latency")
	require.Contains(t, body, "gateway")
	require.Contains(t, body, "100.0")
	require.Contains(t, body, "120s")
	require.Contains(t, body, "warning")
	require.Contains(t, body, "15m")
}
