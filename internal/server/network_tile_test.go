// Feature network-tile — FR-085 (link-state verdict), FR-086 (throughput
// subtitle + collapsed per-interface detail), FR-087 (absorb the WAN chip).
// Test names carry their TC id (TestTC_NT_085h ↔ TC-NT-085h).
package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/network/wanmonitor"
)

// --- fixtures -----------------------------------------------------------

func healthyProbes() []*gatewayprobe.Snapshot {
	return []*gatewayprobe.Snapshot{
		{Label: "gateway", Status: "ok", RTTMs: 1.4, LossPct: 0},
		{Label: "1.1.1.1", Status: "ok", RTTMs: 12, LossPct: 0},
		{Label: "8.8.8.8", Status: "ok", RTTMs: 14, LossPct: 0},
		{Label: "dns", Status: "ok", RTTMs: 11, LossPct: 0},
	}
}

func wanUp() *wanmonitor.Snapshot   { return &wanmonitor.Snapshot{State: "up"} }
func wanDown() *wanmonitor.Snapshot { return &wanmonitor.Snapshot{State: "down"} }

// The interfaces this Pi actually reports, virtual ones included.
func piInterfaces() []metrics.NetworkIface {
	return []metrics.NetworkIface{
		{Name: "lo", BytesSentPS: 1400, BytesRecvPS: 1400},
		{Name: "eth0", BytesSentPS: 7200, BytesRecvPS: 54700},
		{Name: "wlan0"},
		{Name: "tailscale0"},
		{Name: "docker0"},
		{Name: "br-2ce4512f3708"},
		{Name: "br-80ea838d7698", BytesSentPS: 618, BytesRecvPS: 552},
		{Name: "veth87e1c5c", BytesSentPS: 618, BytesRecvPS: 580},
	}
}

func renderTile(t *testing.T, dd DashboardData) string {
	t.Helper()
	srv, _ := setupTestServerWithSession(t)
	html := srv.renderPartial("partials/sse-metrics.html", dd)
	require.NotEmpty(t, html, "the partial must render")
	return html
}

// networkTileHTML isolates the Network tile — from its OPENING div (which is
// where metric-warning/metric-critical live) to the start of the next tile — so
// an assertion cannot accidentally pass on a neighbour's markup.
func networkTileHTML(t *testing.T, html string) string {
	t.Helper()
	label := strings.Index(html, ">Network<")
	require.GreaterOrEqual(t, label, 0, "the Network tile must be present")

	// Walk back to the tile's own opening <div ... class="metric-tile ...">.
	start := strings.LastIndex(html[:label], `<div class="metric-tile`)
	require.GreaterOrEqual(t, start, 0, "the Network tile must open with a metric-tile div")

	rest := html[start:]
	if j := strings.Index(rest[1:], `<div class="metric-tile`); j > 0 {
		return rest[:j+1]
	}
	return rest
}

// --- FR-085 — the verdict ------------------------------------------------

// TC-NT-085h / AC-085-001
func TestTC_NT_085h_AllProbesHealthyIsStable(t *testing.T) {
	got := dashboardLinkState(healthyProbes(), wanUp())

	assert.Equal(t, "stable", got.Verdict)
	assert.Contains(t, got.Reason, "WAN up")
	assert.Contains(t, got.Reason, "0% loss")
	assert.Zero(t, got.WorstLoss)
}

// TC-NT-085e / AC-085-002 — the reason must name the offender.
func TestTC_NT_085e_LossMakesItUnstable(t *testing.T) {
	probes := healthyProbes()
	probes[2].LossPct = 15 // 8.8.8.8 — 3 of its 20-sample window

	got := dashboardLinkState(probes, wanUp())

	assert.Equal(t, "unstable", got.Verdict)
	assert.Equal(t, 15.0, got.WorstLoss)
	assert.Contains(t, got.Reason, "8.8.8.8", "the admin must be told WHICH target is failing")
	assert.Contains(t, got.Reason, "15%")
}

// TC-NT-085f / AC-085-003
func TestTC_NT_085f_WANDownIsOffline(t *testing.T) {
	got := dashboardLinkState(healthyProbes(), wanDown())

	assert.Equal(t, "offline", got.Verdict,
		"the WAN monitor is the authority; it needs 3 consecutive failures, so this is not a flap")
	assert.Contains(t, got.Reason, "WAN down")
}

// TC-NT-085Bf / AC-085-004 — a broken LAN is not merely "unstable".
func TestTC_NT_085Bf_GatewayDownIsOffline(t *testing.T) {
	probes := healthyProbes()
	probes[0].Status = "timeout" // the gateway itself

	got := dashboardLinkState(probes, nil)

	assert.Equal(t, "offline", got.Verdict,
		"a box that cannot reach its own gateway has a broken LAN — a different problem from a lossy internet path")
	assert.Contains(t, got.Reason, "gateway")
}

// TC-NT-085Be / AC-085-005 (also NFR-088b) — never claim a state we cannot know.
func TestTC_NT_085Be_NoProbesIsUnknown(t *testing.T) {
	assert.Equal(t, "unknown", dashboardLinkState(nil, nil).Verdict)
	assert.Equal(t, "unknown", dashboardLinkState([]*gatewayprobe.Snapshot{}, wanUp()).Verdict)

	// …and the tile still renders, saying plainly that it cannot know.
	html := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
	}))
	assert.NotContains(t, html, "data-link-state", "no verdict may be claimed without probes")
	assert.Contains(t, html, "no probes", "the tile admits what it does not know instead of inventing a state")
	assert.NotContains(t, html, "eth0", "the throughput readout was removed on the owner's call")
}

// TC-NT-085Bh / AC-085-001 — ADR-2: one dropped ping must not flap the tile.
func TestTC_NT_085Bh_SingleDroppedPingDoesNotFlap(t *testing.T) {
	probes := healthyProbes()
	probes[1].LossPct = 5 // exactly one lost packet in a 20-sample window

	got := dashboardLinkState(probes, wanUp())

	assert.Equal(t, "stable", got.Verdict,
		"a >0%% rule would turn the tile yellow on a single lost packet and train the admin to ignore it")
	assert.Equal(t, 5.0, got.WorstLoss, "the loss is still reported, it just does not trip the verdict")
}

// --- FR-087 — the absorbed WAN chip --------------------------------------

// TC-NT-087h / AC-087-001
func TestTC_NT_087h_WANStateMovesIntoTheTile(t *testing.T) {
	full := renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     wanUp(),
	})

	assert.Contains(t, networkTileHTML(t, full), "WAN up", "the tile carries the WAN state now")
	assert.NotContains(t, full, "bg-green-400/10 text-green-400 font-mono",
		"the standalone WAN chip below the grid must be gone")
	assert.NotContains(t, full, "WAN ?")
}

// TC-NT-087f / AC-087-002
func TestTC_NT_087f_WANDownRendersOfflineWithNoChip(t *testing.T) {
	full := renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     wanDown(),
	})
	tile := networkTileHTML(t, full)

	assert.Contains(t, tile, "Offline")
	assert.Contains(t, tile, "metric-critical")
	assert.NotContains(t, full, "WAN DOWN",
		"the old chip is gone — the tile is the single source of WAN truth")
}

// TC-NT-087e / AC-087-003 — a nil WAN snapshot must not crash the template.
func TestTC_NT_087e_NilWANDoesNotCrashTheTile(t *testing.T) {
	html := renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     nil,
	})
	tile := networkTileHTML(t, html)

	assert.Contains(t, tile, "Stable", "the probes still answer, so the link is stable")
	assert.NotContains(t, tile, "WAN up", "no WAN state may be claimed without a snapshot")
	assert.NotContains(t, tile, "WAN down")
}

// --- NFR regressions -----------------------------------------------------

// TC-NT-088h / NFR-088b — the neighbours are untouched.
func TestTC_NT_088h_NeighbouringTilesUnchanged(t *testing.T) {
	temp := 51.0
	html := renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{
			CPU:         metrics.CPUMetrics{TotalPercent: 22.2},
			RAM:         metrics.RAMMetrics{Total: 7900000000, Used: 3600000000, Percent: 45.7},
			Temperature: &temp,
			Disks:       []metrics.DiskPartition{{Path: "/", Total: 915300000000, Used: 71600000000, Percent: 8.2}},
			Networks:    piInterfaces(),
		},
		Network: healthyProbes(),
		WAN:     wanUp(),
	})

	assert.Contains(t, html, formatPercent(22.2), "CPU still renders")
	assert.Contains(t, html, formatPercent(45.7), "Memory still renders")
	assert.Contains(t, html, formatPercent(8.2), "Disk still renders")
	assert.Contains(t, html, "51", "Temp still renders")
}

// TC-NT-088f / NFR-088b — a nil probe entry would kill the dashboard for every
// connected SSE client if it panicked.
func TestTC_NT_088f_NilProbeEntryDoesNotPanic(t *testing.T) {
	probes := []*gatewayprobe.Snapshot{
		{Label: "gateway", Status: "ok"},
		nil,
		{Label: "8.8.8.8", Status: "ok"},
	}

	require.NotPanics(t, func() {
		got := dashboardLinkState(probes, wanUp())
		assert.Equal(t, "stable", got.Verdict)
	})
}

// --- e2e -----------------------------------------------------------------

// TC-NT-E2E-001h — the PAGE template path (not the SSE one).
func TestTC_NT_E2E_001h_DashboardPageRendersTheNewTile(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	getRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/login", nil))
	csrf := extractCSRFToken(getRec.Body.String())
	require.NotEmpty(t, csrf)

	form := url.Values{"username": {"admin"}, "password": {"secret"}, "csrf_token": {csrf}}
	post := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(postRec, post)
	require.Equal(t, http.StatusSeeOther, postRec.Code)

	var session *http.Cookie
	for _, c := range postRec.Result().Cookies() {
		if c.Name == "session" {
			session = c
		}
	}
	require.NotNil(t, session)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, ">Network<", "the tile renders through the page template path")
	assert.NotContains(t, body, "WAN ?", "the orphan WAN chip is gone from the page")
	assert.NotContains(t, body, "WAN DOWN")
}

// TC-NT-E2E-002h — the SSE path, i.e. the OTHER FuncMap. templates.go carries
// two; registering the new funcs in only one silently breaks this path.
func TestTC_NT_E2E_002h_SSEMetricsEventRendersTheTile(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	payload := string(srv.buildSharedSSEEvents(false))

	require.Contains(t, payload, "event: metrics")
	assert.Contains(t, payload, ">Network<",
		"the tile must render through the SSE FuncMap too — a missing registration fails silently here")
	assert.NotContains(t, payload, "template", "no template error text may leak into the event")
}
