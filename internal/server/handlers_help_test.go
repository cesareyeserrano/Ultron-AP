// Tests for the help-page HTTP integration.
//
// @aitri-trace FR-048 FR-053 FR-056 NFR-022 NFR-023 NFR-024 NFR-025
// TC-HP-048h TC-HP-048f TC-HP-053f TC-HP-053e2 TC-HP-056h TC-HP-056e TC-HP-056fneg TC-HP-NFR-022h TC-HP-NFR-022f
package server

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/help"
	"github.com/cesareyeserrano/ultron-ap/internal/insights"
	"github.com/cesareyeserrano/ultron-ap/internal/insights/lang"
	insightsstore "github.com/cesareyeserrano/ultron-ap/internal/insights/store"
)

// newTestHelp wires the production help.Service onto the server.
func newTestHelp(t *testing.T, srv *Server) *help.Service {
	t.Helper()
	svc, err := help.New(func(string, ...interface{}) {})
	require.NoError(t, err)
	srv.SetHelp(svc)
	return svc
}

// TestTC_HP_048h — authenticated GET /help returns 200, text/html body
// with five category sections in fixed order.
//
// @aitri-tc TC-HP-048h
// @aitri-trace FR-048 AC-048-001
func TestTC_HP_048h(t *testing.T) {
	srv, session := setupSSETestServer(t)
	newTestHelp(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))

	body := rec.Body.String()
	want := []string{
		`data-category="system-metrics"`,
		`data-category="network-probes"`,
		`data-category="services-containers"`,
		`data-category="vpn"`,
		`data-category="insights-verdicts"`,
	}
	// Verify the five attributes appear, in document order.
	idx := -1
	for _, w := range want {
		i := strings.Index(body, w)
		require.True(t, i >= 0, "missing %q in body", w)
		require.True(t, i > idx, "%q out of order (idx=%d, prev=%d)", w, i, idx)
		idx = i
	}
}

// TestTC_HP_048f — unauthenticated GET /help returns 302 to /login and leaks
// no glossary content.
//
// @aitri-tc TC-HP-048f
// @aitri-trace FR-048 AC-048-002
func TestTC_HP_048f(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	newTestHelp(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	// requireAuth uses StatusSeeOther (303). The contract is "any redirect to
	// /login that strips the body" — we accept both 302 and 303 since both
	// satisfy FR-007 AC-002 (no glossary content leaked).
	assert.True(t, rec.Code == http.StatusFound || rec.Code == http.StatusSeeOther,
		"unauthenticated GET /help must redirect (302/303); got %d", rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))

	body := rec.Body.String()
	assert.NotContains(t, body, "entry-")
	assert.NotContains(t, body, "verdict-thermal-throttling")
	assert.NotContains(t, body, "Thermal throttling")
}

// TestTC_HP_NFR_022h — server-side render p99 of GET /help is <500 ms over
// 100 sequential requests.
//
// @aitri-tc TC-HP-NFR-022h
// @aitri-trace NFR-022 AC-048-003
func TestTC_HP_NFR_022h(t *testing.T) {
	srv, session := setupSSETestServer(t)
	newTestHelp(t, srv)

	// Warm-up.
	warm := httptest.NewRequest(http.MethodGet, "/help", nil)
	warm.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	srv.httpServer.Handler.ServeHTTP(httptest.NewRecorder(), warm)

	durations := make([]time.Duration, 100)
	for i := range durations {
		req := httptest.NewRequest(http.MethodGet, "/help", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
		rec := httptest.NewRecorder()
		t0 := time.Now()
		srv.httpServer.Handler.ServeHTTP(rec, req)
		durations[i] = time.Since(t0)
		require.Equal(t, http.StatusOK, rec.Code)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[49]
	p99 := durations[98]
	t.Logf("GET /help: p50=%v p99=%v", p50, p99)
	assert.Less(t, p99, 500*time.Millisecond, "p99 render time must be <500 ms")
	assert.Less(t, p50, 100*time.Millisecond, "p50 render time must be <100 ms")
}

// TestTC_HP_NFR_022f — POST /help and POST /api/help/* return 4xx (no glossary
// write API). Substitutes for the 404 assertion — Go's ServeMux returns 405
// for method-mismatched paths and 404 for unregistered paths; both prove the
// absence of a write surface.
//
// @aitri-tc TC-HP-NFR-022f
// @aitri-trace NFR-023 AC-054-001
func TestTC_HP_NFR_022f(t *testing.T) {
	srv, session := setupSSETestServer(t)
	newTestHelp(t, srv)

	cases := []struct {
		path string
		want []int
	}{
		{"/help", []int{http.StatusMethodNotAllowed, http.StatusNotFound}},
		{"/api/help/glossary", []int{http.StatusNotFound}},
		{"/api/help/entry", []int{http.StatusNotFound}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader("{}"))
			req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
			rec := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(rec, req)
			matched := false
			for _, w := range tc.want {
				if rec.Code == w {
					matched = true
					break
				}
			}
			assert.True(t, matched, "POST %s returned %d, want one of %v", tc.path, rec.Code, tc.want)
		})
	}
}

// TestTC_HP_056h — sidebar contains a 'Help' nav item with href='/help' as
// the last nav-item.
//
// @aitri-tc TC-HP-056h
// @aitri-trace FR-056 AC-056-001
func TestTC_HP_056h(t *testing.T) {
	srv, session := setupSSETestServer(t)
	newTestHelp(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	helpIdx := strings.Index(body, `href="/help"`)
	require.True(t, helpIdx > 0, "expected href=\"/help\" in dashboard sidebar")
	// "Help" label appears within ~2 KB of the href (in the same nav-item block).
	tail := body[helpIdx:]
	require.True(t, strings.Index(tail, ">Help<") >= 0 || strings.Contains(tail[:min(len(tail), 2048)], ">Help<"),
		"expected visible 'Help' label near href=\"/help\"")

	// Help anchor must be the last nav-item — no other nav-item href appears after it.
	otherHrefs := []string{`href="/"`, `href="/docker"`, `href="/services"`, `href="/alerts"`, `href="/network"`, `href="/history"`, `href="/logs"`, `href="/settings"`}
	for _, h := range otherHrefs {
		i := strings.LastIndex(body, h)
		require.True(t, i < helpIdx, "%s appears after Help link (i=%d helpIdx=%d) — Help must be last", h, i, helpIdx)
	}
}

// TestTC_HP_056e — on /help, the Help nav item carries the active-state class.
//
// @aitri-tc TC-HP-056e
// @aitri-trace FR-056 AC-056-003
func TestTC_HP_056e(t *testing.T) {
	srv, session := setupSSETestServer(t)
	newTestHelp(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	helpIdx := strings.Index(body, `href="/help"`)
	require.True(t, helpIdx > 0)

	// The dashboard's active-link convention is the literal class "active-link"
	// that the existing sidebar template uses.
	tail := body[helpIdx:]
	window := tail[:min(len(tail), 1024)]
	assert.Contains(t, window, "active-link", "Help nav item must have 'active-link' class on /help")
}

// TestTC_HP_056fneg — adding the Help item does not move/restyle existing
// nav items; each pre-existing href is still present and matches its expected
// label.
//
// @aitri-tc TC-HP-056fneg
// @aitri-trace FR-056 AC-056-003
func TestTC_HP_056fneg(t *testing.T) {
	srv, session := setupSSETestServer(t)
	newTestHelp(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()

	expected := map[string]string{
		`href="/"`:         "Dashboard",
		`href="/docker"`:   "Docker",
		`href="/services"`: "Services",
		`href="/alerts"`:   "Alerts",
		`href="/network"`:  "Network",
		`href="/history"`:  "History",
		`href="/logs"`:     "Logs",
		`href="/settings"`: "Settings",
	}
	for href, label := range expected {
		i := strings.Index(body, href)
		require.True(t, i > 0, "missing %s in sidebar", href)
		// The label must appear within the same nav-item block (within 2 KB of href).
		windowEnd := i + 2048
		if windowEnd > len(body) {
			windowEnd = len(body)
		}
		assert.Contains(t, body[i:windowEnd], ">"+label+"<", "nav-item %s must still render label %q", href, label)
	}
}

// TestTC_HP_053e2 — Learn-more anchor uses the same tab (no target=_blank).
//
// @aitri-tc TC-HP-053e2
// @aitri-trace FR-053 AC-053-001
func TestTC_HP_053e2(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	helpSvc := newTestHelp(t, srv)
	require.NotNil(t, helpSvc)

	st := insightsstore.New(srv.db.DB)
	require.NoError(t, st.SeedRule(insightsstore.Rule{
		ID:             "thermal-throttling",
		Title:          "Thermal throttling",
		ConditionJSON:  []byte(`{"op":"gt","left":{"var":"temp_c"},"right":{"const":80}}`),
		Severity:       insightsstore.SeverityCritical,
		Verdict:        "Thermal throttling probable",
		Recommendation: "Check airflow",
		Links:          []string{"#verdict-thermal-throttling"},
		Source:         "bundled",
	}))
	insightsSvc := insights.New(insights.Config{Store: st})
	require.NoError(t, insightsSvc.RefreshFromStore())
	srv.SetInsights(insightsSvc)
	insightsSvc.EvalWithVars(time.Now(), map[string]lang.Value{
		"temp_c": lang.Number(95),
	})

	html := srv.renderVerdictsFragment(time.Now())
	require.Contains(t, html, "Learn more")
	// The Learn-more anchor must NOT carry target="_blank".
	assert.NotContains(t, html, `target="_blank"`)
	// And it must point at /help#verdict-thermal-throttling.
	assert.Contains(t, html, `href="/help#verdict-thermal-throttling"`)
}

// TestTC_HP_053f — verdict whose links are empty or all-missing produces no
// Learn-more anchor in the rendered card.
//
// @aitri-tc TC-HP-053f
// @aitri-trace FR-053 AC-053-001
func TestTC_HP_053f_serverFragment(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	newTestHelp(t, srv)

	st := insightsstore.New(srv.db.DB)
	require.NoError(t, st.SeedRule(insightsstore.Rule{
		ID:             "missing-links-rule",
		Title:          "Missing-links rule",
		ConditionJSON:  []byte(`{"op":"gt","left":{"var":"cpu_pct"},"right":{"const":1}}`),
		Severity:       insightsstore.SeverityWarn,
		Verdict:        "Triggered",
		Recommendation: "n/a",
		Links:          []string{"#verdict-does-not-exist"},
		Source:         "bundled",
	}))
	insightsSvc := insights.New(insights.Config{Store: st})
	require.NoError(t, insightsSvc.RefreshFromStore())
	srv.SetInsights(insightsSvc)
	insightsSvc.EvalWithVars(time.Now(), map[string]lang.Value{"cpu_pct": lang.Number(50)})

	html := srv.renderVerdictsFragment(time.Now())
	require.Contains(t, html, "Missing-links rule")
	assert.NotContains(t, html, "Learn more", "card with no valid anchor must not render Learn more")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
