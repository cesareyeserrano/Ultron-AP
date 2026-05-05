// Tests for the insights-engine HTTP surface.
//
// @aitri-trace FR-043 FR-044 NFR-019 US-043 US-044
// TC-IE-005h TC-IE-005f TC-IE-005e TC-IE-006h TC-IE-006f TC-IE-006e TC-IE-012h TC-IE-012f
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/insights"
	"github.com/cesareyeserrano/ultron-ap/internal/insights/lang"
	insightsstore "github.com/cesareyeserrano/ultron-ap/internal/insights/store"
)

// newTestInsights wires a fresh insights.Service onto srv backed by srv's DB.
// Returns the service so the test can drive ticks before issuing requests.
func newTestInsights(t *testing.T, srv *Server) *insights.Service {
	t.Helper()
	st := insightsstore.New(srv.db.DB)
	svc := insights.New(insights.Config{Store: st})
	require.NoError(t, svc.LoadBundled())
	srv.SetInsights(svc)
	return svc
}

// idleVars returns a healthy snapshot — all bundled rules false.
func idleVars() map[string]lang.Value {
	return map[string]lang.Value{
		"cpu_pct":                  lang.Number(10),
		"ram_pct":                  lang.Number(30),
		"swap_pct":                 lang.Number(0),
		"temp_c":                   lang.Number(45),
		"disk_root_pct":            lang.Number(20),
		"services_failed":          lang.Number(0),
		"containers_failed":        lang.Number(0),
		"wan_gateway_ok":           lang.Number(1),
		"wan_cloudflare_ok":        lang.Number(1),
		"loss_pct":                 lang.Number(0),
		"lan_device_offline_count": lang.Number(0),
	}
}

// TC-IE-005h
// SSE broker emits a 'verdicts' event with HTML fragment when active set
// changes (asserted via the renderer used by the broker).
//
// @aitri-tc TC-IE-005h
func TestTC_IE_005h_VerdictsFragmentRendersActiveSet(t *testing.T) {
	// @aitri-tc TC-IE-005h
	srv, _ := setupSSETestServer(t)
	svc := newTestInsights(t, srv)

	// Drive one tick that fires disk_critical (single comparison rule).
	vars := idleVars()
	vars["disk_root_pct"] = lang.Number(98)
	svc.EvalWithVars(time.Now(), vars)

	html := srv.renderVerdictsFragment(time.Now())
	assert.Contains(t, html, "Disk critical", "fragment must include the rule title")
	assert.Contains(t, html, "CRITICAL", "explicit severity label must be present (NFR-020)")
	assert.Contains(t, html, "operational-indicators")
}

// TC-IE-005f
// Empty active set still emits an empty 'verdicts' fragment containing the
// 'All operational indicators clear' copy.
//
// @aitri-tc TC-IE-005f
func TestTC_IE_005f_EmptyActiveSetShowsClearMessage(t *testing.T) {
	// @aitri-tc TC-IE-005f
	srv, _ := setupSSETestServer(t)
	svc := newTestInsights(t, srv)

	svc.EvalWithVars(time.Now(), idleVars())
	html := srv.renderVerdictsFragment(time.Now())
	assert.Contains(t, html, "All operational indicators clear")
	assert.NotContains(t, html, "verdict-card", "no card divs in empty state")
}

// TC-IE-005e
// SSE fragment HTML-escapes verdict_text and recommendation (XSS defence).
//
// @aitri-tc TC-IE-005e
func TestTC_IE_005e_VerdictsFragmentHTMLEscapes(t *testing.T) {
	// @aitri-tc TC-IE-005e
	srv, _ := setupSSETestServer(t)

	// Build a custom service with a single hostile rule.
	st := insightsstore.New(srv.db.DB)
	require.NoError(t, st.SeedRule(insightsstore.Rule{
		ID:             "xss_test",
		Title:          "XSS Test",
		ConditionJSON:  json.RawMessage(`{"op":"gt","left":{"var":"cpu_pct"},"right":{"const":1}}`),
		Severity:       insightsstore.SeverityCritical,
		Verdict:        `<script>alert(1)</script>`,
		Recommendation: `"><img src=x onerror=alert(1)>`,
		Links:          []string{},
	}))
	svc := insights.New(insights.Config{Store: st})
	require.NoError(t, svc.RefreshFromStore())
	srv.SetInsights(svc)

	vars := idleVars()
	vars["cpu_pct"] = lang.Number(95)
	svc.EvalWithVars(time.Now(), vars)

	html := srv.renderVerdictsFragment(time.Now())
	assert.Contains(t, html, "&lt;script&gt;", "must HTML-escape the script tag")
	assert.False(t, bytes.Contains([]byte(html), []byte("<script>")),
		"raw <script> tag must NOT appear in the rendered fragment")
}

// TC-IE-006h
// Operational Indicators fragment renders verdicts sorted critical → warn → info,
// newest-first within severity.
//
// @aitri-tc TC-IE-006h
func TestTC_IE_006h_FragmentSortsCriticalWarnInfo(t *testing.T) {
	// @aitri-tc TC-IE-006h
	srv, _ := setupSSETestServer(t)
	st := insightsstore.New(srv.db.DB)
	// Three custom rules: one per severity, distinct ids.
	require.NoError(t, st.SeedRule(insightsstore.Rule{
		ID: "test_critical", Title: "TestCritical",
		ConditionJSON:  json.RawMessage(`{"op":"gt","left":{"var":"x"},"right":{"const":1}}`),
		Severity:       insightsstore.SeverityCritical,
		Verdict:        "v", Recommendation: "r", Links: []string{},
	}))
	require.NoError(t, st.SeedRule(insightsstore.Rule{
		ID: "test_warn", Title: "TestWarn",
		ConditionJSON:  json.RawMessage(`{"op":"gt","left":{"var":"x"},"right":{"const":1}}`),
		Severity:       insightsstore.SeverityWarn,
		Verdict:        "v", Recommendation: "r", Links: []string{},
	}))
	require.NoError(t, st.SeedRule(insightsstore.Rule{
		ID: "test_info", Title: "TestInfo",
		ConditionJSON:  json.RawMessage(`{"op":"gt","left":{"var":"x"},"right":{"const":1}}`),
		Severity:       insightsstore.SeverityInfo,
		Verdict:        "v", Recommendation: "r", Links: []string{},
	}))
	svc := insights.New(insights.Config{Store: st})
	require.NoError(t, svc.RefreshFromStore())
	srv.SetInsights(svc)

	svc.EvalWithVars(time.Now(), map[string]lang.Value{"x": lang.Number(99)})
	html := srv.renderVerdictsFragment(time.Now())

	// Document order: critical position < warn position < info position.
	pCrit := strings.Index(html, "TestCritical")
	pWarn := strings.Index(html, "TestWarn")
	pInfo := strings.Index(html, "TestInfo")
	require.True(t, pCrit >= 0 && pWarn >= 0 && pInfo >= 0, "all three rules must render")
	assert.Less(t, pCrit, pWarn, "critical must precede warn in document order")
	assert.Less(t, pWarn, pInfo, "warn must precede info in document order")
}

// TC-IE-006f
// Empty active verdict set renders the 'All operational indicators clear'
// message; section root is still present.
//
// @aitri-tc TC-IE-006f
func TestTC_IE_006f_EmptyStateRendersClearCopyAndSectionRoot(t *testing.T) {
	// @aitri-tc TC-IE-006f
	srv, _ := setupSSETestServer(t)
	svc := newTestInsights(t, srv)
	svc.EvalWithVars(time.Now(), idleVars())

	html := srv.renderVerdictsFragment(time.Now())
	assert.Contains(t, html, "All operational indicators clear")
	assert.Contains(t, html, `id="operational-indicators"`)
	assert.NotContains(t, html, "verdict-card")
}

// TC-IE-006e
// 100 verdicts render in a single fragment within bounded time and produce
// 100 cards.
//
// @aitri-tc TC-IE-006e
func TestTC_IE_006e_HundredVerdictsRenderUnderBudget(t *testing.T) {
	// @aitri-tc TC-IE-006e
	srv, _ := setupSSETestServer(t)
	st := insightsstore.New(srv.db.DB)
	for i := 0; i < 100; i++ {
		sev := insightsstore.SeverityWarn
		if i%3 == 0 {
			sev = insightsstore.SeverityCritical
		} else if i%3 == 2 {
			sev = insightsstore.SeverityInfo
		}
		require.NoError(t, st.SeedRule(insightsstore.Rule{
			ID:             "stress_" + intToStrLocal(i),
			Title:          "Stress " + intToStrLocal(i),
			ConditionJSON:  json.RawMessage(`{"op":"gt","left":{"var":"x"},"right":{"const":1}}`),
			Severity:       sev,
			Verdict:        "v", Recommendation: "r", Links: []string{},
		}))
	}
	svc := insights.New(insights.Config{Store: st})
	require.NoError(t, svc.RefreshFromStore())
	srv.SetInsights(svc)

	svc.EvalWithVars(time.Now(), map[string]lang.Value{"x": lang.Number(99)})

	start := time.Now()
	html := srv.renderVerdictsFragment(time.Now())
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 50*time.Millisecond, "100-verdict render must complete <50ms")
	count := strings.Count(html, "verdict-card")
	assert.Equal(t, 100, count, "fragment must contain 100 .verdict-card elements")
}

// TC-IE-012h
// GET /api/insights/verdicts authenticated — returns 200 with JSON array
// sorted critical→warn→info.
//
// @aitri-tc TC-IE-012h
func TestTC_IE_012h_VerdictsAPIReturnsSortedJSON(t *testing.T) {
	// @aitri-tc TC-IE-012h
	srv, session := setupSSETestServer(t)
	st := insightsstore.New(srv.db.DB)
	for _, sev := range []insightsstore.Severity{
		insightsstore.SeverityCritical,
		insightsstore.SeverityWarn,
		insightsstore.SeverityInfo,
	} {
		require.NoError(t, st.SeedRule(insightsstore.Rule{
			ID:             "sort_" + string(sev),
			Title:          "Sort " + string(sev),
			ConditionJSON:  json.RawMessage(`{"op":"gt","left":{"var":"x"},"right":{"const":1}}`),
			Severity:       sev,
			Verdict:        "v", Recommendation: "r", Links: []string{},
		}))
	}
	svc := insights.New(insights.Config{Store: st})
	require.NoError(t, svc.RefreshFromStore())
	srv.SetInsights(svc)

	svc.EvalWithVars(time.Now(), map[string]lang.Value{"x": lang.Number(99)})

	req := httptest.NewRequest(http.MethodGet, "/api/insights/verdicts", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var out []verdictJSON
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out, 3)
	assert.Equal(t, "critical", out[0].Severity)
	assert.Equal(t, "warn", out[1].Severity)
	assert.Equal(t, "info", out[2].Severity)
	for _, v := range out {
		assert.NotNil(t, v.Links, "links must be [] not null")
		_, err := time.Parse(time.RFC3339, v.FirstEmittedAt)
		assert.NoError(t, err, "first_emitted_at must parse as RFC3339: %s", v.FirstEmittedAt)
		_, err = time.Parse(time.RFC3339, v.LastEvaluatedAt)
		assert.NoError(t, err, "last_evaluated_at must parse as RFC3339: %s", v.LastEvaluatedAt)
	}
}

// TC-IE-012f
// GET /api/insights/verdicts unauthenticated — returns 401, no verdict data
// leaked.
//
// @aitri-tc TC-IE-012f
func TestTC_IE_012f_VerdictsAPIUnauthenticatedReturns401(t *testing.T) {
	// @aitri-tc TC-IE-012f
	srv, _ := setupSSETestServer(t)
	svc := newTestInsights(t, srv)

	vars := idleVars()
	vars["disk_root_pct"] = lang.Number(98)
	svc.EvalWithVars(time.Now(), vars)

	req := httptest.NewRequest(http.MethodGet, "/api/insights/verdicts", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	body := rec.Body.String()
	// No verdict-content fields leaked.
	assert.NotContains(t, body, "Disk critical")
	assert.NotContains(t, body, "disk_critical")
}

// intToStrLocal — local copy of a tiny formatter to avoid pulling fmt.
func intToStrLocal(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
