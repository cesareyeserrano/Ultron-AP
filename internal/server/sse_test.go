package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
)

func setupSSETestServer(t *testing.T) (*Server, *database.Session) {
	t.Helper()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Port:       8080,
		DBPath:     filepath.Join(tmpDir, "test.db"),
		LogLevel:   "info",
		AdminUser:  "admin",
		AdminPass:  "secret",
		SessionTTL: 24 * time.Hour,
		BackupRoot: tmpDir,
	}

	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	err = db.CreateUser("admin", "$2a$10$dummy")
	require.NoError(t, err)

	session := &database.Session{
		ID:        "test-sse-session",
		UserID:    1,
		CSRFToken: "test-csrf",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err = db.CreateSession(session)
	require.NoError(t, err)

	srv := New(cfg, db, nil, nil, nil, nil, nil)
	return srv, session
}

// --- Template Helper Tests ---

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{52428800, "50.0 MB"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, formatBytes(tt.input))
	}
}

func TestFormatPercent(t *testing.T) {
	assert.Equal(t, "45.3%", formatPercent(45.3))
	assert.Equal(t, "0.0%", formatPercent(0))
	assert.Equal(t, "100.0%", formatPercent(100))
}

func TestTempColor(t *testing.T) {
	hot := 80.0
	warm := 65.0
	cool := 45.0

	assert.Equal(t, "text-danger", tempColor(&hot))
	assert.Equal(t, "text-yellow-400", tempColor(&warm))
	assert.Equal(t, "text-green-400", tempColor(&cool))
	assert.Equal(t, "text-text-muted", tempColor(nil))
}

func TestFormatTemp(t *testing.T) {
	val := 52.3
	assert.Equal(t, "52°C", formatTemp(&val))
	assert.Equal(t, "--", formatTemp(nil))
}

func TestShortID(t *testing.T) {
	assert.Equal(t, "abc123def456", shortID("abc123def456789"))
	assert.Equal(t, "short", shortID("short"))
}

func TestSparklineSVG_Empty(t *testing.T) {
	result := sparklineSVG(nil)
	assert.Empty(t, string(result))
}

func TestSparklineSVG_WithData(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50}
	result := sparklineSVG(values)
	assert.Contains(t, string(result), "<svg")
	assert.Contains(t, string(result), "polyline")
}

// --- SSE Broker Tests ---

func TestSSEBroker_AddRemoveClient(t *testing.T) {
	b := newSSEBroker()
	c := b.addClient()

	b.mu.RLock()
	assert.Len(t, b.clients, 1)
	b.mu.RUnlock()

	b.removeClient(c)
	b.mu.RLock()
	assert.Len(t, b.clients, 0)
	b.mu.RUnlock()
}

func TestSSEBroker_Broadcast(t *testing.T) {
	b := newSSEBroker()
	c1 := b.addClient()
	c2 := b.addClient()
	defer b.removeClient(c1)
	defer b.removeClient(c2)

	b.broadcast([]byte("test"))

	select {
	case data := <-c1.ch:
		assert.Equal(t, "test", string(data))
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

	select {
	case data := <-c2.ch:
		assert.Equal(t, "test", string(data))
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

// --- SSE Endpoint Tests ---

func TestSSEEndpoint_RequiresAuth(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code) // API routes return 401, not redirect
}

func TestSSEEndpoint_SetsHeaders(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})

	// Use a context with cancel to close the SSE connection
	ctx, cancel := context.WithTimeout(req.Context(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}

func TestSSEEndpoint_SendsInitialData(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sse/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})

	ctx, cancel := context.WithTimeout(req.Context(), 100*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "event: metrics")
	assert.Contains(t, body, "event: charts")
	assert.Contains(t, body, "event: summary")
}

// --- Docker Detail Endpoint Tests ---

func TestDockerDetailEndpoint_RequiresAuth(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/docker/abc123", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code) // API routes return 401, not redirect
}

func TestDockerDetailEndpoint_NoDocker(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/docker/abc123", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// --- Dashboard Handler Tests ---

func TestDashboard_RendersWithContent(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "sse-connect")
	assert.Contains(t, body, "/static/js/sse.js")
	assert.Contains(t, body, "Operational Indicators")
	assert.Contains(t, body, "Containers")
}

func TestSettings_DoesNotEnableSSE(t *testing.T) {
	srv, session := setupSSETestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "sse-connect")
	assert.NotContains(t, body, "/static/js/sse.js")
}

// --- GatherDashboardData Tests ---

func TestGatherDashboardData_NilCollectors(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	dd := srv.gatherDashboardData("5m", 60)

	assert.Nil(t, dd.Metrics)
	assert.False(t, dd.DockerAvail)
	assert.False(t, dd.SystemdAvail)
	assert.NotEmpty(t, dd.Uptime)
}

// --- WriteSSEEvent Tests ---

func TestWriteSSEEvent(t *testing.T) {
	b := &bytes.Buffer{}
	writeSSEEvent(b, "metrics", "<div>test</div>")
	assert.Equal(t, "event: metrics\ndata: <div>test</div>\n\n", b.String())
}

// TestChartWindowIsPerClient is a regression test for BG-046: the chart window
// used to be server-global, so one client's selection leaked into every other
// client's charts. The window now lives on each sseClient, and buildChartsEvent
// renders for a given window — so two different windows must produce different
// charts events, and a client's stored window must round-trip via chart().
func TestChartWindowIsPerClient(t *testing.T) {
	srv, _ := setupSSETestServer(t)

	short := srv.buildChartsEvent("5m", 60)
	long := srv.buildChartsEvent("24h", 1440)
	assert.NotEqual(t, string(short), string(long),
		"different windows must render different charts events (no shared global)")

	// Per-client window round-trips with defaults applied.
	c := &sseClient{chartWindow: "24h", chartPoints: 1440}
	w, p := c.chart()
	assert.Equal(t, "24h", w)
	assert.Equal(t, 1440, p)

	empty := &sseClient{}
	w, p = empty.chart()
	assert.Equal(t, "5m", w)
	assert.Equal(t, 60, p)
}

// BG-074 — the insights tick used to key on a probe label ("cloudflare") that no
// default target carries, so wan_cloudflare_ok was never set and loss_pct was
// never published: two bundled rules could never fire. The flags are now derived
// structurally from the probe list.
func TestInsightsVars_WANFlagsAndLossFromDefaultProbeLabels(t *testing.T) {
	dd := DashboardData{Network: []*gatewayprobe.Snapshot{
		{Label: "gateway", Status: "ok", LossPct: 0},
		{Label: "1.1.1.1", Status: "timeout", LossPct: 25},
		{Label: "8.8.8.8", Status: "ok", LossPct: 5},
		{Label: "dns", Status: "ok", LossPct: 0},
	}}

	vars := insightsNetworkVars(dd)

	require.Contains(t, vars, "wan_gateway_ok")
	assert.Equal(t, 1.0, vars["wan_gateway_ok"], "the gateway answered")

	require.Contains(t, vars, "wan_cloudflare_ok",
		"the internet-reachable flag must be set with the DEFAULT labels — no target is named 'cloudflare'")
	assert.Equal(t, 1.0, vars["wan_cloudflare_ok"], "8.8.8.8 answered, so the internet is reachable")

	require.Contains(t, vars, "loss_pct")
	assert.Equal(t, 25.0, vars["loss_pct"], "loss_pct is the worst loss across the probes")
}

// The internet flag must go to 0 when EVERY off-box probe fails, while the
// gateway still answers — that is exactly the "LAN fine, WAN dead" case the
// wan_lan_disambig rule exists to catch.
func TestInsightsVars_InternetDownButGatewayUp(t *testing.T) {
	dd := DashboardData{Network: []*gatewayprobe.Snapshot{
		{Label: "gateway", Status: "ok"},
		{Label: "1.1.1.1", Status: "timeout", LossPct: 100},
		{Label: "8.8.8.8", Status: "timeout", LossPct: 100},
	}}

	vars := insightsNetworkVars(dd)

	assert.Equal(t, 1.0, vars["wan_gateway_ok"])
	assert.Equal(t, 0.0, vars["wan_cloudflare_ok"], "no off-box target answered")
	assert.Equal(t, 100.0, vars["loss_pct"])
}
