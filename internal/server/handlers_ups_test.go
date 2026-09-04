package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/ups"
)

func fptr(v float64) *float64 { return &v }
func iptr(v int) *int         { return &v }

// onlineSnapshot is a representative reachable OL snapshot.
func onlineSnapshot() *ups.Snapshot {
	return &ups.Snapshot{
		State: ups.StateOnline, RawStatus: "OL", Reachable: true,
		LoadPct: fptr(2), InputV: fptr(122), BatteryV: fptr(27.1), BattPctEst: fptr(95),
		Beeper: "enabled", CutoffV: 21.0, DelayShut: iptr(30), DelayStart: iptr(60),
		LastGood: time.Now(),
	}
}

// TC-UPS-005h (FR-017): UPS card shows 'En red' with values when OL.
func TestTC_UPS_005h_CardOnline(t *testing.T) {
	// @aitri-tc TC-UPS-005h
	srv, _ := setupTestServerWithSession(t)
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: onlineSnapshot()})
	require.NotEmpty(t, html)
	assert.Contains(t, html, "En red")
	assert.Contains(t, html, "2 %")
	assert.Contains(t, html, "122 V")
	assert.Contains(t, html, "27.1 V")
	assert.Contains(t, html, "estimado")
	assert.Contains(t, html, "id=\"ups-card\"")
}

// TC-UPS-006e (FR-017): UPS card shows 'Sin datos' when unreachable, not zeros.
func TestTC_UPS_006e_CardUnreachable(t *testing.T) {
	// @aitri-tc TC-UPS-006e
	srv, _ := setupTestServerWithSession(t)
	snap := &ups.Snapshot{State: ups.StateUnreachable, Reachable: false, CutoffV: 21.0}
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: snap})
	assert.Contains(t, html, "Sin datos")
	assert.Contains(t, html, "UPS sin comunicación")
	assert.Contains(t, html, "metric-ups-muted", "unreachable card must carry the muted/dashed class")
	assert.Contains(t, html, "—", "value fields must show em-dash, not zeros")
	assert.NotContains(t, html, "0 %", "must not render a fabricated zero load")
	assert.NotContains(t, html, "0 V", "must not render a fabricated zero voltage")
}

// TC-UPS-007f (FR-017): a NUT value with HTML metacharacters is escaped in the card.
func TestTC_UPS_007f_CardEscapesValue(t *testing.T) {
	// @aitri-tc TC-UPS-007f
	srv, _ := setupTestServerWithSession(t)
	snap := onlineSnapshot()
	snap.Beeper = "<img src=x onerror=alert(1)>"
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: snap})
	assert.NotContains(t, html, "<img src=x onerror=alert(1)>", "raw markup must not survive")
	assert.Contains(t, html, "&lt;img", "value must be HTML-escaped")
}

// TC-UPS-008e (FR-017): card updates via SSE and is laid out mobile-friendly.
func TestTC_UPS_008e_SSEAndResponsive(t *testing.T) {
	// @aitri-tc TC-UPS-008e
	// SSE swap wiring: the dashboard subscribes to the "ups" event.
	dash, err := os.ReadFile(filepath.Join("..", "..", "web", "templates", "dashboard.html"))
	require.NoError(t, err)
	assert.Contains(t, string(dash), `sse-swap="ups"`, "dashboard must subscribe to the ups SSE event")

	srv, _ := setupTestServerWithSession(t)
	// Compact UPS tile is a family member in the responsive metrics grid.
	metrics := srv.renderPartial("partials/sse-metrics.html", DashboardData{UPS: onlineSnapshot()})
	assert.Contains(t, metrics, `id="ups-tile"`, "compact UPS tile must render in the metrics grid")
	assert.Contains(t, metrics, "En red", "compact tile shows the principal status indicator")
	assert.Contains(t, metrics, "metric-tile", "compact tile reuses the responsive tile class")
	// The UPS card sits in a responsive grid (full-width on mobile, one summary
	// column on desktop) so it fits 375px without overflow.
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: onlineSnapshot()})
	assert.Contains(t, html, "grid-cols-1", "card grid collapses to one column on mobile")
	assert.Contains(t, html, "lg:grid-cols-3", "card aligns to the summary grid on desktop")
}

// TC-UPS-009h (FR-017): beeper state rendered from ups.beeper.status.
func TestTC_UPS_009h_BeeperDisplay(t *testing.T) {
	// @aitri-tc TC-UPS-009h
	srv, _ := setupTestServerWithSession(t)
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: onlineSnapshot()})
	assert.Contains(t, html, "activado", "enabled beeper renders as 'activado'")
}

// TC-UPS-027h (FR-023): shutdown block shows delays with 'gestionado por NUT'.
func TestTC_UPS_027h_ShutdownDelays(t *testing.T) {
	// @aitri-tc TC-UPS-027h
	srv, _ := setupTestServerWithSession(t)
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: onlineSnapshot()})
	assert.Contains(t, html, "gestionado por NUT")
	assert.Contains(t, html, "30 s")
	assert.Contains(t, html, "60 s")
}

// TC-UPS-028e (FR-023): a missing shutdown variable shows 'no disponible'.
func TestTC_UPS_028e_ShutdownMissing(t *testing.T) {
	// @aitri-tc TC-UPS-028e
	srv, _ := setupTestServerWithSession(t)
	snap := onlineSnapshot()
	snap.DelayStart = nil // UPS did not publish it
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: snap})
	assert.Contains(t, html, "no disponible")
}

// TC-UPS-029f (FR-023): the shutdown block exposes no actionable control.
func TestTC_UPS_029f_NoControl(t *testing.T) {
	// @aitri-tc TC-UPS-029f
	srv, _ := setupTestServerWithSession(t)
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: onlineSnapshot()})
	for _, forbidden := range []string{"<form", "<button", "<input", "hx-post", "hx-put", "hx-delete"} {
		assert.NotContains(t, html, forbidden, "shutdown block must not expose a control")
	}
}

// TC-UPS-030h (FR-023): low-battery cutoff shown as 'punto de apagado' from config.
func TestTC_UPS_030h_CutoffDisplay(t *testing.T) {
	// @aitri-tc TC-UPS-030h
	srv, _ := setupTestServerWithSession(t)
	snap := onlineSnapshot()
	snap.CutoffV = 21.0
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: snap})
	assert.Contains(t, html, "21.0 V")
	assert.Contains(t, html, "Punto de apagado")
	assert.Contains(t, html, "gestionado por NUT")
}

// TC-UPS-034h (NFR-016): with NUT absent the UPS card shows 'Sin datos' while an
// existing tile (metrics) still renders — the panel degrades cleanly.
func TestTC_UPS_034h_CleanDegradation(t *testing.T) {
	// @aitri-tc TC-UPS-034h
	srv, _ := setupTestServerWithSession(t)
	// Existing tile still renders (proves the rest of the dashboard is unaffected).
	metricsHTML := srv.renderPartial("partials/sse-metrics.html", DashboardData{})
	require.NotEmpty(t, metricsHTML)
	// UPS unreachable → Sin datos, no crash.
	upsHTML := srv.renderPartial("partials/sse-ups.html",
		DashboardData{UPS: &ups.Snapshot{State: ups.StateUnreachable, Reachable: false, CutoffV: 21.0}})
	assert.Contains(t, upsHTML, "Sin datos")
}

// TC-UPS-035e (NFR-016): module inert when disabled — no UPS card is rendered
// when dd.UPS is nil.
func TestTC_UPS_035e_InertWhenDisabled(t *testing.T) {
	// @aitri-tc TC-UPS-035e
	srv, _ := setupTestServerWithSession(t)
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: nil})
	assert.NotContains(t, html, "ups-card", "no card must render when the module is disabled")
}

// TC-UPS-042f (NFR-018): the shutdown-config render path issues no command — it
// reads a Snapshot and emits no control/command surface.
func TestTC_UPS_042f_RenderIssuesNoCommand(t *testing.T) {
	// @aitri-tc TC-UPS-042f
	srv, _ := setupTestServerWithSession(t)
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: onlineSnapshot()})
	// A command/control surface — not the benign descriptive id "ups-shutdown".
	for _, forbidden := range []string{"hx-post", "hx-put", "hx-delete", "instcmd", "load.off", "<form", "<button", "<input"} {
		assert.NotContains(t, strings.ToLower(html), forbidden, "render path must expose no command surface")
	}
}

// TC-UPS-043h (NFR-019): benign NUT values render correctly (escaping baseline).
func TestTC_UPS_043h_BenignRender(t *testing.T) {
	// @aitri-tc TC-UPS-043h
	srv, _ := setupTestServerWithSession(t)
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: onlineSnapshot()})
	assert.Contains(t, html, "En red")
	assert.Contains(t, html, "activado")
	assert.Contains(t, html, "id=\"ups-card\"")
}

// TC-UPS-044f (NFR-019): a script-tag NUT value is escaped (XSS vector 1).
func TestTC_UPS_044f_ScriptEscaped(t *testing.T) {
	// @aitri-tc TC-UPS-044f
	srv, _ := setupTestServerWithSession(t)
	snap := onlineSnapshot()
	snap.Beeper = "<script>alert(1)</script>"
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: snap})
	assert.NotContains(t, html, "<script>alert(1)</script>")
	assert.Contains(t, html, "&lt;script&gt;")
}

// TC-UPS-045f (NFR-019): event-handler attribute injection is neutralized (XSS vector 2).
func TestTC_UPS_045f_AttributeInjectionNeutralized(t *testing.T) {
	// @aitri-tc TC-UPS-045f
	srv, _ := setupTestServerWithSession(t)
	snap := onlineSnapshot()
	// RawStatus renders into the card's title attribute; a broken-out handler
	// must be escaped, not become a live attribute.
	snap.RawStatus = `" onmouseover="alert(1)`
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: snap})
	assert.NotContains(t, html, `onmouseover="alert(1)"`, "attribute-context payload must be escaped")
}

// TC-UPS-053h (NFR-022): mock mode renders the real card locally with dummy data.
func TestTC_UPS_053h_MockRendersCard(t *testing.T) {
	// @aitri-tc TC-UPS-053h
	srv, _ := setupTestServerWithSession(t)
	cfg := ups.Config{Mock: "OL", BattLowV: 21.0, BattHighV: 27.4, UnreachableTimeout: time.Second}
	p := ups.NewPoller(ups.NewClient(cfg), cfg)
	snap := p.PollNow(context.Background()) // one mock poll, no real UPS
	require.True(t, snap.Reachable, "mock OL must produce a reachable snapshot")
	html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: &snap})
	assert.Contains(t, html, "En red", "the real card renders from mock data with no physical UPS")
}

// TC-UPS-055e (NFR-022): mock mode drives the card through each state on demand.
func TestTC_UPS_055e_MockCyclesStates(t *testing.T) {
	// @aitri-tc TC-UPS-055e
	srv, _ := setupTestServerWithSession(t)
	cfg := ups.Config{Mock: "1", BattLowV: 21.0, BattHighV: 27.4, UnreachableTimeout: time.Nanosecond}
	p := ups.NewPoller(ups.NewClient(cfg), cfg)
	seen := map[string]bool{}
	for i := 0; i < 4; i++ { // cycle: OL → OB → LB → unreachable
		snap := p.PollNow(context.Background())
		html := srv.renderPartial("partials/sse-ups.html", DashboardData{UPS: &snap})
		for _, label := range []string{"En red", "En batería", "Batería baja", "Sin datos"} {
			if strings.Contains(html, label) {
				seen[label] = true
			}
		}
	}
	for _, label := range []string{"En red", "En batería", "Batería baja", "Sin datos"} {
		assert.True(t, seen[label], "mock cycle should reach state %q", label)
	}
}
