// Feature network-tile — FR-085 (link-state verdict), FR-086 (throughput
// subtitle + collapsed per-interface detail), FR-087 (absorb the WAN chip).
// Test names carry their TC id (TestTC_NT_085h ↔ TC-NT-085h).
package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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

	// …and the tile still renders, falling back to throughput.
	html := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
	}))
	assert.NotContains(t, html, "data-link-state", "no verdict may be claimed without probes")
	assert.Contains(t, html, "eth0", "the tile falls back to the throughput readout")
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

// --- FR-086 — throughput subtitle and collapsed detail --------------------

// TC-NT-086h / AC-086-001
func TestTC_NT_086h_SubtitleNamesBusiestInterface(t *testing.T) {
	html := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     wanUp(),
	}))

	assert.Contains(t, html, "Stable")
	assert.Contains(t, html, "eth0", "the busiest interface is named in the subtitle")
	assert.Contains(t, html, formatBytes(7200)+"/s")
	assert.Contains(t, html, formatBytes(54700)+"/s")

	// The idle interfaces exist, but only inside the disclosure.
	before, after, found := strings.Cut(html, "<details")
	require.True(t, found, "the per-interface detail must be a disclosure")
	assert.NotContains(t, before, "wlan0", "an idle interface must not sit in the tile body")
	assert.Contains(t, after, "wlan0")
}

// TC-NT-086e / AC-086-002 (also NFR-090b)
func TestTC_NT_086e_DetailIsCollapsedByDefault(t *testing.T) {
	html := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     wanUp(),
	}))

	require.Contains(t, html, "<details")
	openTag := regexp.MustCompile(`<details[^>]*>`).FindString(html)
	assert.NotContains(t, openTag, " open",
		"the breakdown must be closed by default — an open one reproduces the exact complaint")
}

// TC-NT-089h / AC-086-003 (also NFR-089b)
func TestTC_NT_089h_ExpandedDetailListsEveryInterface(t *testing.T) {
	html := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     wanUp(),
	}))

	_, detail, found := strings.Cut(html, "<details")
	require.True(t, found)
	for _, want := range []string{"eth0", "wlan0", "tailscale0"} {
		assert.Contains(t, detail, want, "the expanded detail must list %q", want)
	}
}

// TC-NT-086f / AC-086-004 — ADR-3: max, never sum.
func TestTC_NT_086f_BusiestInterfaceIsMaxNotSum(t *testing.T) {
	// tailscale0 tunnels over eth0 — the SAME bytes seen twice.
	ifaces := []metrics.NetworkIface{
		{Name: "eth0", BytesSentPS: 30000, BytesRecvPS: 30000},
		{Name: "tailscale0", BytesSentPS: 25000, BytesRecvPS: 25000},
	}

	got := primaryNetwork(ifaces)

	require.NotNil(t, got)
	assert.Equal(t, "eth0", got.Name, "the busiest single interface, not a total")
	assert.Equal(t, uint64(30000), got.BytesSentPS,
		"summing would report 110 KB/s for 60 KB/s of real traffic")

	html := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: ifaces},
		Network: healthyProbes(),
		WAN:     wanUp(),
	}))
	assert.NotContains(t, html, formatBytes(55000), "no summed total may appear anywhere in the tile")
}

// TC-NT-089f / AC-086-005 (also NFR-089b) — BG-072 still holds.
func TestTC_NT_089f_VirtualInterfacesStayHidden(t *testing.T) {
	html := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     wanUp(),
	}))

	for _, hidden := range []string{"docker0", "br-2ce4512f3708", "br-80ea838d7698", "veth87e1c5c", ">lo<"} {
		assert.NotContains(t, html, hidden,
			"virtual interface %q must not reappear, expanded or not", hidden)
	}
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

// TC-NT-088e / NFR-089b — the BG-072 fallback: never empty the tile.
func TestTC_NT_088e_FilterNeverEmptiesTheTile(t *testing.T) {
	only := []metrics.NetworkIface{{Name: "docker0", BytesSentPS: 10, BytesRecvPS: 20}}

	got := primaryNetwork(only)

	require.NotNil(t, got, "an unusual host must still see its traffic rather than an empty tile")
	assert.Equal(t, "docker0", got.Name)
}

// TC-NT-090h / NFR-090b — the disclosure meets the project's touch-target floor.
func TestTC_NT_090h_DisclosureMeetsTouchTarget(t *testing.T) {
	html := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     wanUp(),
	}))

	summary := regexp.MustCompile(`<summary[^>]*>`).FindString(html)
	require.NotEmpty(t, summary, "the disclosure must have a summary")
	assert.Contains(t, summary, "min-h-[44px]", "the toggle must meet the 44px floor the project enforces")
}

// TC-NT-090f / NFR-090b — nothing leaks out of the disclosure.
func TestTC_NT_090f_NoInterfaceRowLeaksOutsideDisclosure(t *testing.T) {
	tile := networkTileHTML(t, renderTile(t, DashboardData{
		Metrics: &metrics.Snapshot{Networks: piInterfaces()},
		Network: healthyProbes(),
		WAN:     wanUp(),
	}))

	// Strip the whole disclosure; whatever remains is the visible tile body.
	body := regexp.MustCompile(`(?s)<details.*?</details>`).ReplaceAllString(tile, "")

	for _, iface := range []string{"wlan0", "tailscale0"} {
		assert.NotContains(t, body, iface,
			"%q must live only inside the collapsed disclosure", iface)
	}
	// eth0 is allowed in the body: it is the named busiest interface (the subtitle).
	assert.Contains(t, body, "eth0")
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
