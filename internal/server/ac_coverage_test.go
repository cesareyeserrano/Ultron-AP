// Acceptance-criteria coverage tests — each test traces one AC from
// spec/01_REQUIREMENTS.json via its @aitri-tc marker. They close the
// traceability gap found by `aitri verify-complete` (BL-033): the behaviors
// existed but nothing exercised their THEN clauses.
package server

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

// @aitri-tc TC-001a — RAM used/total and percentage render on the
// dashboard metrics tile when telemetry is available (AC-001-002).
func TestDashboardMetricsPartial_RendersRAM(t *testing.T) {
	srv, _ := setupTestServerWithSession(t)

	dd := DashboardData{Metrics: &metrics.Snapshot{
		RAM: metrics.RAMMetrics{Total: 1 << 30, Used: 512 << 20, Percent: 50.0},
	}}
	html := srv.renderPartial("partials/sse-metrics.html", dd)

	require.NotEmpty(t, html)
	assert.Contains(t, html, "Memory")
	assert.Contains(t, html, formatPercent(50.0), "RAM percentage must render")
	assert.Contains(t, html, formatBytes(512<<20), "RAM used must render")
	assert.Contains(t, html, formatBytes(1<<30), "RAM total must render")
}

// @aitri-tc TC-001b — every mounted partition renders used/total/percent
// on the dashboard disk tile (AC-001-003).
func TestDashboardMetricsPartial_RendersEveryPartition(t *testing.T) {
	srv, _ := setupTestServerWithSession(t)

	dd := DashboardData{Metrics: &metrics.Snapshot{
		Disks: []metrics.DiskPartition{
			{Path: "/", Total: 32 << 30, Used: 16 << 30, Percent: 50.0},
			{Path: "/data", Total: 128 << 30, Used: 96 << 30, Percent: 75.0},
		},
	}}
	html := srv.renderPartial("partials/sse-metrics.html", dd)

	for _, p := range []string{"/", "/data"} {
		assert.Contains(t, html, ">"+p+"<", "partition %s must be listed", p)
	}
	assert.Contains(t, html, formatBytes(16<<30))
	assert.Contains(t, html, formatBytes(96<<30))
	assert.Contains(t, html, formatPercent(75.0))
}

// @aitri-tc TC-001c — per-interface send/receive byte rates render on the
// dashboard network tile (AC-001-004).
func TestDashboardMetricsPartial_RendersEveryInterface(t *testing.T) {
	srv, _ := setupTestServerWithSession(t)

	dd := DashboardData{Metrics: &metrics.Snapshot{
		Networks: []metrics.NetworkIface{
			{Name: "eth0", BytesSentPS: 1024, BytesRecvPS: 2048},
			{Name: "wlan0", BytesSentPS: 512, BytesRecvPS: 4096},
		},
	}}
	html := srv.renderPartial("partials/sse-metrics.html", dd)

	assert.Contains(t, html, "eth0")
	assert.Contains(t, html, "wlan0")
	for _, b := range []uint64{1024, 2048, 512, 4096} {
		assert.Contains(t, html, formatBytes(b)+"/s", "rate %d must render as bytes/s", b)
	}
}

// @aitri-tc TC-001g — metrics reach connected SSE clients from the
// broadcast path (no HTTP request), and the periodic cadence defaults to the
// configured 5s interval (AC-001-006).
func TestSSEBroadcast_PeriodicMetricsWithoutRequest(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	def := database.DefaultPerformanceConfig()
	assert.Equal(t, time.Duration(def.SSEIntervalSec)*time.Second, srv.sseInterval(),
		"periodic broadcast cadence must follow the configured interval")

	client := srv.sseBroker.addClient()
	defer srv.sseBroker.removeClient(client)

	// One broadcast tick, exactly as startSSEBroadcast performs it.
	srv.sseBroker.broadcast(srv.buildSharedSSEEvents(false))

	select {
	case payload := <-client.ch:
		assert.Contains(t, string(payload), "event: metrics",
			"broadcast must push a metrics event to connected clients")
	case <-time.After(2 * time.Second):
		t.Fatal("no SSE payload delivered to the registered client")
	}
}

// @aitri-tc TC-001i — the dashboard shows system uptime (AC-001-007).
func TestDashboard_ShowsUptime(t *testing.T) {
	srv, session := setupTestServerWithSession(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `title="Server uptime"`, "uptime element must render in the header")

	// The value renders inside the uptime block as <span class="font-mono">Xm</span>.
	block := regexp.MustCompile(`(?s)title="Server uptime".*?<span class="font-mono">([^<]*)</span>`).FindStringSubmatch(body)
	require.NotNil(t, block, "uptime span must render inside the uptime block")
	value := strings.TrimSpace(block[1])
	assert.Regexp(t, `\d+[dhm]`, value, "uptime must show a duration, got %q", value)
}

// @aitri-tc TC-004g — rendered alerts carry their severity color:
// critical red, warning yellow, info accent-blue (AC-004-007).
func TestAlertsPartial_SeverityColorIndicators(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	for _, a := range []*database.Alert{
		{Severity: "critical", Message: "crit-alert", Source: "cpu"},
		{Severity: "warning", Message: "warn-alert", Source: "ram"},
		{Severity: "info", Message: "info-alert", Source: "network"},
	} {
		require.NoError(t, srv.db.CreateAlert(a))
	}
	alerts, err := srv.db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 3)

	html := srv.renderPartial("partials/alerts-list.html", map[string]interface{}{
		"Alerts": alerts, "CSRFToken": "t",
	})

	assert.Contains(t, html, "bg-danger", "critical alert must render the red indicator")
	assert.Contains(t, html, "bg-yellow-400", "warning alert must render the yellow indicator")
	assert.Contains(t, html, "bg-accent", "info alert must render the accent indicator")
}

// @aitri-tc TC-006a — saving the email form persists the SMTP channel
// configuration (host, port, user, password, from, to) (AC-006-001).
func TestNotificationSave_EmailPersistsSMTPFields(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "test-secret-key")
	srv, session := setupSSETestServer(t)

	form := url.Values{
		"csrf_token":    {session.CSRFToken},
		"smtp_host":     {"mail.lan"},
		"smtp_port":     {"587"},
		"smtp_user":     {"ultron"},
		"smtp_password": {"s3cret"},
		"from":          {"ultron@lan"},
		"to":            {"admin@lan"},
		"enabled":       {"on"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/notifications/email", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	got, err := srv.db.GetNotificationConfig("email")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Enabled)
	for _, v := range []string{"mail.lan", "587", "ultron", "s3cret", "ultron@lan", "admin@lan"} {
		assert.Contains(t, got.Config, v, "SMTP field %q must persist", v)
	}
}

// @aitri-tc TC-007b — the session cookie issued on login expires after the
// 24h default TTL (HttpOnly is asserted by TestLogin_Success) (AC-007-003).
func TestLogin_SessionCookieDefaultTTL(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(getRec, getReq)
	csrfToken := extractCSRFToken(getRec.Body.String())
	require.NotEmpty(t, csrfToken)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("csrf_token", csrfToken)
	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(postRec, postReq)
	require.Equal(t, http.StatusSeeOther, postRec.Code)

	var sessionCookie *http.Cookie
	for _, c := range postRec.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
		}
	}
	require.NotNil(t, sessionCookie)
	assert.Equal(t, int((24 * time.Hour).Seconds()), sessionCookie.MaxAge,
		"session cookie must expire after the 24h default TTL")
	assert.True(t, sessionCookie.HttpOnly)
}

// @aitri-tc TC-008a — every controllable service row renders Start, Stop
// and Restart controls (AC-008-002).
func TestServicesPage_RendersControlButtons(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, action := range []string{"start", "stop", "restart"} {
		assert.Contains(t, body, fmt.Sprintf("/%s\"", action),
			"row must offer the %s control", action)
		assert.Regexp(t, regexp.MustCompile(`hx-post="/api/services/[^"]+/`+action+`"`), body)
	}
}

// @aitri-tc TC-008c — service actions dispatch as HTMX partial swaps and
// return an in-page HTML fragment (no navigation, UI never blocks on a full
// reload) (AC-008-006).
func TestServiceControls_AsyncHTMXFragment(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	// The controls declare their async swap target.
	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	assert.Contains(t, body, `hx-target="#services-list"`)
	assert.Contains(t, body, `hx-swap="innerHTML"`)

	// Dispatching an action returns a 200 HTML fragment, not a redirect.
	form := url.Values{"csrf_token": {session.CSRFToken}}
	post := httptest.NewRequest(http.MethodPost, "/api/services/nginx.service/restart", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	postRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(postRec, post)

	assert.Equal(t, http.StatusOK, postRec.Code, "action must respond in-page, not navigate")
	assert.Empty(t, postRec.Header().Get("Location"))
	assert.Contains(t, postRec.Header().Get("Content-Type"), "text/html")
	assert.NotEmpty(t, postRec.Body.String(), "action must render a result fragment")
}

// --- WCAG contrast checks computed from the real design tokens ---

var cssTokenRe = regexp.MustCompile(`--color-([a-z-]+):\s*#([0-9a-fA-F]{6})`)

func loadCSSTokens(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../../web/css/input.css")
	require.NoError(t, err)
	tokens := map[string]string{}
	for _, m := range cssTokenRe.FindAllStringSubmatch(string(raw), -1) {
		if _, seen := tokens[m[1]]; !seen {
			tokens[m[1]] = m[2]
		}
	}
	require.NotEmpty(t, tokens, "design tokens must parse from input.css")
	return tokens
}

// relLuminance computes WCAG 2.1 relative luminance for a 6-digit hex color.
func relLuminance(hex6 string) float64 {
	toLin := func(pair string) float64 {
		v, _ := strconv.ParseUint(pair, 16, 16)
		c := float64(v) / 255.0
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r := toLin(hex6[0:2])
	g := toLin(hex6[2:4])
	b := toLin(hex6[4:6])
	return 0.2126*r + 0.7152*g + 0.0722*b
}

func contrastRatio(fg, bg string) float64 {
	l1 := relLuminance(fg)
	l2 := relLuminance(bg)
	if l2 > l1 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// @aitri-tc TC-009a — body text tokens meet WCAG AA 4.5:1 against every
// panel background token (AC-009-001).
func TestCSSTokens_BodyTextContrastAA(t *testing.T) {
	tokens := loadCSSTokens(t)
	for _, fg := range []string{"text", "text-muted"} {
		for _, bg := range []string{"base", "surface", "card"} {
			require.Contains(t, tokens, fg)
			require.Contains(t, tokens, bg)
			ratio := contrastRatio(tokens[fg], tokens[bg])
			assert.GreaterOrEqualf(t, ratio, 4.5,
				"--color-%s on --color-%s is %.2f:1, below WCAG AA body text", fg, bg, ratio)
		}
	}
}

// @aitri-tc TC-009d — status text tokens on dark backgrounds meet WCAG
// 2.1 AA (≥3:1 for large/status text; error text on its own banner ≥4.5:1)
// (AC-009-005).
func TestCSSTokens_StatusTextContrastAA(t *testing.T) {
	tokens := loadCSSTokens(t)
	for _, fg := range []string{"danger", "accent"} {
		for _, bg := range []string{"base", "surface", "card"} {
			ratio := contrastRatio(tokens[fg], tokens[bg])
			assert.GreaterOrEqualf(t, ratio, 3.0,
				"--color-%s on --color-%s is %.2f:1, below WCAG AA large text", fg, bg, ratio)
		}
	}
	ratio := contrastRatio(tokens["error-text"], tokens["error-bg"])
	assert.GreaterOrEqual(t, ratio, 4.5, "error text on error banner must meet body AA")
}

// @aitri-tc TC-009b — status badges map each state to its semantic color
// token: running/active green, error/failed red, paused yellow, muted gray
// (AC-009-002).
func TestStatusBadge_SemanticColorTokens(t *testing.T) {
	assert.Equal(t, "bg-green-500", healthColor(docker.HealthRunning))
	assert.Equal(t, "bg-red-500", healthColor(docker.HealthError))
	assert.Equal(t, "bg-yellow-500", healthColor(docker.HealthPaused))
	assert.Equal(t, "bg-gray-500", healthColor(docker.HealthStopped))

	assert.Equal(t, "bg-green-500", svcHealthColor(systemd.ServiceActive))
	assert.Equal(t, "bg-red-500", svcHealthColor(systemd.ServiceFailed))
	assert.Equal(t, "bg-gray-500", svcHealthColor(systemd.ServiceInactive))
}

// @aitri-tc TC-009c — interactive settings controls declare ≥44px touch
// targets; no sub-44px marker remains (AC-009-004).
func TestSettingsMarkup_TouchTargets44px(t *testing.T) {
	body := getSettingsBody(t)
	assert.Contains(t, body, "min-h-[44px]", "44px touch-target markers must be present")
	assert.Contains(t, body, "min-w-[44px]")
	assert.NotContains(t, body, "min-h-[40px]", "no interactive control may declare a sub-44px target")
}

// @aitri-tc TC-014a — the SSE summary event carries the Tailscale/VPN
// status block, so VPN state refreshes without a page reload (AC-014-003).
func TestSSESummary_CarriesTailscaleStatus(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	payload := string(srv.buildSharedSSEEvents(true))
	assert.Contains(t, payload, "event: metrics")
	assert.Contains(t, payload, "event: summary", "heavy cadence must include the summary event")
	assert.Contains(t, payload, "VPN", "summary event must carry the VPN/Tailscale block")
}

// @aitri-tc TC-002b — the Docker page refreshes its container list
// automatically every 10 seconds (AC-002-004).
func TestDockerPage_AutoRefreshEvery10s(t *testing.T) {
	srv, session := setupDockerTestServer(t, &mockDockerClient{})

	req := httptest.NewRequest(http.MethodGet, "/docker", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `hx-trigger="every 10s"`, "docker list must poll every 10s")
	assert.Contains(t, body, `hx-get="/docker"`)
	assert.Contains(t, body, `hx-select="#docker-list"`)
}

// @aitri-tc TC-003b — the Services page refreshes its list automatically
// every 30 seconds (AC-003-005).
func TestServicesPage_AutoRefreshEvery30s(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `hx-trigger="every 30s"`, "services list must poll every 30s")
	assert.Contains(t, body, `hx-get="/services"`)
	assert.Contains(t, body, `hx-select="#services-list"`)
}

// @aitri-tc TC-003c — a rendered service row shows name, state and the
// active-since label (AC-003-002).
func TestServicesPartial_RendersNameStateAndActiveSince(t *testing.T) {
	srv, _ := setupTestServerWithSession(t)

	html := srv.renderPartial("partials/services-list.html", servicesPageData{
		Available: true,
		Services: []systemd.ServiceInfo{{
			Name:        "nginx",
			ActiveState: "active",
			Health:      systemd.ServiceActive,
			Description: "A high performance web server",
			Since:       time.Now().Add(-90 * time.Minute),
		}},
	})

	assert.Contains(t, html, "nginx", "row must show the unit name")
	assert.Contains(t, html, ">active<", "row must show the active state")
	assert.Contains(t, html, "data-active-since", "row must show the active-since label")
	assert.Contains(t, html, "active 1h 30m", "active-since must render the elapsed duration")
}
