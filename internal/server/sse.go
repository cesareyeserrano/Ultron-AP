package server

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/insights/lang"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/network/wanmonitor"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
	"github.com/cesareyeserrano/ultron-ap/internal/tailscale"
	"github.com/cesareyeserrano/ultron-ap/internal/ups"
)

// DashboardData holds all data for dashboard rendering.
type DashboardData struct {
	Metrics        *metrics.Snapshot
	MetricsAgeSec  int64
	MetricsStale   bool
	SystemStatus   string
	SystemHint     string
	ServicesStatus string
	ServicesHint   string
	DataStatus     string
	DataHint       string
	CPUValues      []float64 // Only what's needed for sparklines
	RAMValues      []float64
	TempValues     []float64
	ChartWindow    string
	ChartPoints    int
	ProcessStats   map[string]ProcessConsumer
	Containers     []docker.ContainerInfo
	DockerAvail    bool
	Services       []systemd.ServiceInfo
	SystemdAvail   bool
	Uptime         string
	Tailscale      TailscaleData
	Network        []*gatewayprobe.Snapshot
	NetworkTargets []NetworkTargetView
	WAN            *wanmonitor.Snapshot
	UPS            *ups.Snapshot
	UPSBatteryV    []float64     // battery-voltage series for the charts area (FR-019)
	UPSInputV      []float64     // input-voltage series (FR-019)
	UPSOutageSteps []float64     // 1=on battery, 0=on mains — the "cortes" timeline (FR-020 on the dashboard)
	UPSOutageCount int           // outages touching the selected window
	UPSOutageTotal string        // total time on battery within the window, human ("12 min"); "" when zero
	UPSLastOutage  string        // "hace X · duró Y" | "en curso" | ""
	UPSOnlineSince string        // human duration since mains was last restored ("3 d 4 h"); "" when n/a
	UPSInsights    []ups.Insight // UPS-derived observations (FR-022)
	Version        string
}

// TailscaleData is the data passed to the tailscale-peers partial.
type TailscaleData struct {
	Available bool
	Status    *tailscale.Status
}

type ProcessConsumer struct {
	Name       string
	CPUPercent float64
	RSSBytes   uint64
}

var (
	tailscaleStatusCacheMu sync.Mutex
	tailscaleStatusCache   *tailscale.Status
)

func gatherTailscaleData() TailscaleData {
	if !tailscale.Available() {
		return TailscaleData{Available: false}
	}
	status, err := tailscale.GetStatus()
	if err != nil {
		tailscaleStatusCacheMu.Lock()
		cached := tailscaleStatusCache
		tailscaleStatusCacheMu.Unlock()
		if cached != nil {
			return TailscaleData{Available: true, Status: cached}
		}
		return TailscaleData{Available: true}
	}
	tailscaleStatusCacheMu.Lock()
	tailscaleStatusCache = status
	tailscaleStatusCacheMu.Unlock()
	return TailscaleData{Available: true, Status: status}
}

// --- SSE Broker ---

type sseClient struct {
	ch     chan []byte
	closed bool
	ip     string
	// chartWindow/chartPoints are this client's own dashboard chart selection
	// (BG-046). The broadcast renders the window-dependent charts event per
	// client so one viewer's choice never overwrites another's.
	chartWindow string
	chartPoints int
}

// chart returns the client's chart window/points, falling back to defaults.
func (c *sseClient) chart() (string, int) {
	w := c.chartWindow
	if w == "" {
		w = "5m"
	}
	p := c.chartPoints
	if p < 12 {
		p = 60
	}
	return w, p
}

type sseBroker struct {
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	ipCount map[string]int
	// Small hard limits protect low-resource Pi from reconnect floods.
	maxClients int
	maxPerIP   int

	// done is closed once on shutdown. SSE handlers select on it and return, so
	// http.Server.Shutdown can finish: it waits for active connections, and an
	// SSE stream only ends when the CLIENT disconnects. Without this the wait
	// always hit its 10s deadline, the process exited 1, and systemd recorded a
	// failed service on every single restart (BG-075).
	done     chan struct{}
	doneOnce sync.Once
}

// shutdown releases every SSE handler. Idempotent.
func (b *sseBroker) shutdown() {
	b.doneOnce.Do(func() {
		if b.done != nil {
			close(b.done)
		}
	})
}

func newSSEBroker() *sseBroker {
	return &sseBroker{
		clients:    make(map[*sseClient]struct{}),
		ipCount:    make(map[string]int),
		maxClients: 50,
		maxPerIP:   8,
		done:       make(chan struct{}),
	}
}

func (b *sseBroker) addClient() *sseClient {
	c := &sseClient{ch: make(chan []byte, 8)}
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	return c
}

func (b *sseBroker) addClientForIP(ip string) (*sseClient, bool) {
	c := &sseClient{ch: make(chan []byte, 8), ip: ip}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.clients) >= b.maxClients {
		return nil, false
	}
	if ip != "" && b.ipCount[ip] >= b.maxPerIP {
		return nil, false
	}
	b.clients[c] = struct{}{}
	if ip != "" {
		b.ipCount[ip]++
	}
	return c, true
}

func (b *sseBroker) removeClient(c *sseClient) {
	b.mu.Lock()
	if c.ip != "" {
		if n := b.ipCount[c.ip] - 1; n > 0 {
			b.ipCount[c.ip] = n
		} else {
			delete(b.ipCount, c.ip)
		}
	}
	delete(b.clients, c)
	c.closed = true
	close(c.ch)
	b.mu.Unlock()
}

func (b *sseBroker) broadcast(data []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for c := range b.clients {
		select {
		case c.ch <- data:
		default:
			// Client too slow, skip
		}
	}
}

// broadcastBuild sends per-client bytes computed by build(c). Used when part of
// the payload depends on per-client state (the chart window, BG-046). build is
// called under the read lock so the send coordinates with removeClient the same
// way broadcast does; build must not acquire the broker lock.
func (b *sseBroker) broadcastBuild(build func(c *sseClient) []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for c := range b.clients {
		select {
		case c.ch <- build(c):
		default:
			// Client too slow, skip
		}
	}
}

// --- SSE Handler ---

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	window, points := chartWindowPoints(r.URL.Query().Get("window"), s.cfg.MetricsInterval)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	clientIP := s.clientIPFromRequest(r)
	client, ok := s.sseBroker.addClientForIP(clientIP)
	if !ok {
		s.auditLog(r, "security", "sse_reject", clientIP, "sse connection limit exceeded", false)
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}
	client.chartWindow = window
	client.chartPoints = points
	defer s.sseBroker.removeClient(client)

	// Send initial data immediately, with charts rendered for this client's window.
	var initial bytes.Buffer
	initial.Write(s.buildSharedSSEEvents(true))
	initial.Write(s.buildChartsEvent(window, points))
	w.Write(initial.Bytes())
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.sseBroker.done:
			// BG-075: the server is shutting down. Returning here lets
			// http.Server.Shutdown finish instead of waiting out its deadline on
			// a stream that only ever ends when the browser goes away.
			return
		case data, ok := <-client.ch:
			if !ok {
				return
			}
			_ = rc.SetWriteDeadline(time.Time{})
			w.Write(data)
			flusher.Flush()
		}
	}
}

// startSSEBroadcast starts a goroutine that broadcasts dashboard data to SSE clients.
func (s *Server) startSSEBroadcast() {
	go func() {
		current := s.sseInterval()
		ticker := time.NewTicker(current)
		defer ticker.Stop()
		tick := 0
		for range ticker.C {
			s.sseBroker.mu.RLock()
			count := len(s.sseBroker.clients)
			s.sseBroker.mu.RUnlock()
			if count == 0 {
				continue
			}
			tick++
			// Metrics remain fast; charts and heavy sections use slower cadences.
			chartsEvery := cadenceEvery(current, 15*time.Second)
			heavyEvery := cadenceEvery(current, 10*time.Second) // summary + docker/systemd/alerts
			includeCharts := tick%chartsEvery == 0
			includeHeavy := tick%heavyEvery == 0

			// Window-independent events render once and are shared by all clients.
			shared := s.buildSharedSSEEvents(includeHeavy)
			if !includeCharts {
				s.sseBroker.broadcast(shared)
			} else {
				// Charts depend on each client's selected window (BG-046), so
				// append a per-client charts event to the shared payload.
				s.sseBroker.broadcastBuild(func(c *sseClient) []byte {
					w, p := c.chart()
					var buf bytes.Buffer
					buf.Write(shared)
					buf.Write(s.buildChartsEvent(w, p))
					return buf.Bytes()
				})
			}
			// Dynamically adjust ticker if interval was changed.
			if next := s.sseInterval(); next != current {
				current = next
				tick = 0
				ticker.Reset(current)
			}
		}
	}()
}

func cadenceEvery(current, target time.Duration) int {
	if current <= 0 {
		return 1
	}
	if current >= target {
		return 1
	}
	steps := int((target + current - 1) / current) // ceil(target/current)
	if steps < 1 {
		return 1
	}
	return steps
}

// buildSharedSSEEvents renders the window-independent events shared by every
// client: metrics, and (on the heavy cadence) summary + alert-count, plus the
// verdicts fragment. It runs the single insights engine evaluation per tick.
// The charts event is window-dependent and rendered per client by
// buildChartsEvent (BG-046), so it is NOT included here.
func (s *Server) buildSharedSSEEvents(includeHeavy bool) []byte {
	var buf bytes.Buffer
	// Chart fields are unused by these partials, so default window/points.
	dd := s.gatherDashboardData("5m", 60)
	if includeHeavy {
		// Heavy summary refreshes must include VPN peer state; otherwise
		// SSE swaps overwrite the initial page-load data with an empty list.
		dd.Tailscale = gatherTailscaleData()
	}

	// Metrics event
	metricsHTML := s.renderPartial("partials/sse-metrics.html", dd)
	writeSSEEvent(&buf, "metrics", metricsHTML)

	// UPS event (FR-017) — only when the module is enabled (dd.UPS non-nil).
	if dd.UPS != nil {
		upsHTML := s.renderPartial("partials/sse-ups.html", dd)
		writeSSEEvent(&buf, "ups", upsHTML)
	}

	if includeHeavy {
		// Summary event includes VPN online users and service/container snapshots.
		summaryHTML := s.renderPartial("partials/sse-summary.html", dd)
		writeSSEEvent(&buf, "summary", summaryHTML)
	}

	// Alert count event uses TTL cache and follows heavy cadence.
	if includeHeavy && s.db != nil {
		unackCount := s.cachedAlertCount()
		if unackCount > 0 {
			writeSSEEvent(&buf, "alert-count", fmt.Sprintf("%d", unackCount))
		} else {
			writeSSEEvent(&buf, "alert-count", "")
		}
	}

	// Verdicts event (FR-043) — drive one engine evaluation against the
	// current snapshot, then render the active-set fragment. Must run exactly
	// once per tick (engine state), so it lives in the shared payload.
	if s.insights != nil {
		s.evalInsightsTick(dd)
		verdictsHTML := s.renderVerdictsFragment(time.Now())
		writeSSEEvent(&buf, "verdicts", verdictsHTML)
	}

	return buf.Bytes()
}

// buildChartsEvent renders the window-dependent charts event for a single
// client's selected window/points (BG-046).
func (s *Server) buildChartsEvent(window string, points int) []byte {
	var buf bytes.Buffer
	dd := s.gatherChartData(window, points)
	chartsHTML := s.renderPartial("partials/sse-charts.html", dd)
	writeSSEEvent(&buf, "charts", chartsHTML)
	return buf.Bytes()
}

// gatherChartData builds only the fields the charts partial needs
// (CPUValues/RAMValues/TempValues sparklines, ChartWindow, NetworkTargets) for
// the given window/points, without the heavier summary/docker/systemd gather.
func (s *Server) gatherChartData(window string, points int) DashboardData {
	if points < 12 {
		points = 60
	}
	if window == "" {
		window = "5m"
	}
	dd := DashboardData{ChartWindow: window, ChartPoints: points}
	if s.collector != nil {
		history := s.collector.History(points)
		dd.CPUValues = make([]float64, len(history))
		dd.RAMValues = make([]float64, len(history))
		dd.TempValues = make([]float64, len(history))
		lastTemp := 0.0
		for i, snap := range history {
			dd.CPUValues[i] = snap.CPU.TotalPercent
			dd.RAMValues[i] = snap.RAM.Percent
			if snap.Temperature != nil {
				lastTemp = *snap.Temperature
			}
			dd.TempValues[i] = lastTemp
		}
	}
	if s.gateway != nil {
		dd.NetworkTargets = s.gatherNetworkTargetViews(points)
	}
	if s.ups != nil {
		// UPS series for the charts area (FR-019: battery.voltage, input.voltage,
		// ups.load), sliced to the selected window from the poll-time cache — no
		// DB access here.
		snap := s.ups.Current()
		dd.UPS = &snap
		cutoff := time.Now().Add(-chartWindowDuration(window))
		for _, sm := range s.ups.CachedSamples() {
			if sm.TS.Before(cutoff) {
				continue
			}
			if sm.BatteryV != nil {
				dd.UPSBatteryV = append(dd.UPSBatteryV, *sm.BatteryV)
			}
			if sm.InputV != nil {
				dd.UPSInputV = append(dd.UPSInputV, *sm.InputV)
			}
			step := 0.0
			if sm.State.OnBattery() {
				step = 1.0
			}
			dd.UPSOutageSteps = append(dd.UPSOutageSteps, step)
		}
		// Cap the series to the chart resolution like every other tile —
		// otherwise a 24h window ships thousands of SVG points per SSE tick.
		dd.UPSBatteryV = downsampleAvg(dd.UPSBatteryV, points)
		dd.UPSInputV = downsampleAvg(dd.UPSInputV, points)
		dd.UPSOutageSteps = downsampleMax(dd.UPSOutageSteps, points)
		// Outage events touching the window: count, total on-battery time and
		// the most recent one (events are cached newest-first).
		var total time.Duration
		for _, ev := range s.ups.CachedEvents() {
			open := ev.End == nil
			if !open && ev.End.Before(cutoff) {
				continue // ended before the window
			}
			dd.UPSOutageCount++
			if open {
				total += time.Since(ev.Start)
				if dd.UPSLastOutage == "" {
					dd.UPSLastOutage = "en curso"
				}
			} else {
				if ev.DurationS != nil {
					total += time.Duration(*ev.DurationS) * time.Second
				}
				if dd.UPSLastOutage == "" {
					last := "hace " + ups.FormatDur(time.Since(*ev.End))
					if ev.DurationS != nil {
						last += " · duró " + ups.FormatDur(time.Duration(*ev.DurationS)*time.Second)
					}
					dd.UPSLastOutage = last
				}
			}
		}
		if total > 0 {
			dd.UPSOutageTotal = ups.FormatDur(total)
		}
	}
	return dd
}

// downsampleAvg reduces vals to at most n points by averaging equal buckets.
func downsampleAvg(vals []float64, n int) []float64 {
	if n <= 0 || len(vals) <= n {
		return vals
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		lo, hi := i*len(vals)/n, (i+1)*len(vals)/n
		if hi <= lo {
			hi = lo + 1
		}
		var sum float64
		for _, v := range vals[lo:hi] {
			sum += v
		}
		out[i] = sum / float64(hi-lo)
	}
	return out
}

// downsampleMax reduces vals to at most n points keeping each bucket's maximum
// — used for the outage steps so a short outage inside a bucket stays visible
// instead of being averaged away.
func downsampleMax(vals []float64, n int) []float64 {
	if n <= 0 || len(vals) <= n {
		return vals
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		lo, hi := i*len(vals)/n, (i+1)*len(vals)/n
		if hi <= lo {
			hi = lo + 1
		}
		maxV := vals[lo]
		for _, v := range vals[lo:hi] {
			if v > maxV {
				maxV = v
			}
		}
		out[i] = maxV
	}
	return out
}

// chartWindowDuration maps the dashboard window selector value to a duration.
func chartWindowDuration(window string) time.Duration {
	switch window {
	case "5m":
		return 5 * time.Minute
	case "2h":
		return 2 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "12h":
		return 12 * time.Hour
	default: // "24h" and anything unrecognised
		return 24 * time.Hour
	}
}

// evalInsightsTick is the single integration point between the SSE broker
// and the insights engine. It projects the dashboard's existing variable
// surface (CPU, RAM, temp, disk, services, containers, gateway/cloudflare
// probes, lan-device-offline) into the engine's lang variable map and calls
// Eval. The engine never imports server / alerts / notify — variables flow
// through here as plain values (NFR-021).
func (s *Server) evalInsightsTick(dd DashboardData) {
	now := time.Now()
	if dd.Metrics == nil {
		s.insights.SnapshotMissing()
		return
	}
	vars := map[string]lang.Value{}
	vars["cpu_pct"] = lang.Number(dd.Metrics.CPU.TotalPercent)
	vars["ram_pct"] = lang.Number(dd.Metrics.RAM.Percent)
	if dd.Metrics.Temperature != nil {
		vars["temp_c"] = lang.Number(*dd.Metrics.Temperature)
	}
	for _, p := range dd.Metrics.Disks {
		if p.Path == "/" {
			vars["disk_root_pct"] = lang.Number(p.Percent)
		}
	}
	// swap_pct is not tracked yet by the parent collector — leave missing
	// (the lang package treats missing → false, so memory_pressure simply
	// does not fire). FR-041 AC-002 is satisfied; the rule is a no-op until
	// the collector exposes swap.
	failedSvc := 0
	for _, svc := range dd.Services {
		if svc.ActiveState == "failed" {
			failedSvc++
		}
	}
	vars["services_failed"] = lang.Number(float64(failedSvc))

	failedCont := 0
	for _, c := range dd.Containers {
		if c.State != "running" {
			failedCont++
		}
	}
	vars["containers_failed"] = lang.Number(float64(failedCont))

	for k, v := range insightsNetworkVars(dd) {
		vars[k] = lang.Number(v)
	}

	s.insights.EvalWithVars(now, vars)
}

// gatewayProbeLabel is the label defaultNetTargets() gives the probe whose Host
// is empty — the one resolved from the routing table. Everything else in the
// probe list is an off-box target (BG-074).
const gatewayProbeLabel = "gateway"

// insightsNetworkVars derives the network variables the insight rules read.
//
// BG-074: this used to switch on snap.Label with case "cloudflare" — a label no
// default target carries (they are gateway, 1.1.1.1, 8.8.8.8, dns), so
// wan_cloudflare_ok was NEVER set and the bundled wan_lan_disambig rule could
// not fire. loss_pct was never published at all, killing sustained_packet_loss.
// Both are now derived STRUCTURALLY rather than by name: the gateway is the
// probe resolved from the routing table, every other probe is an off-box
// target, and "the internet is reachable" means ANY of them answers. That
// survives an operator renaming their targets, which the old name-match did not.
func insightsNetworkVars(dd DashboardData) map[string]float64 {
	out := make(map[string]float64, 3)

	var (
		internetSeen, internetOK bool
		worstLoss                float64
	)
	for _, snap := range dd.Network {
		if snap == nil {
			continue
		}
		ok := 0.0
		if string(snap.Status) == "ok" {
			ok = 1.0
		}
		if snap.LossPct > worstLoss {
			worstLoss = snap.LossPct
		}
		if snap.Label == gatewayProbeLabel {
			out["wan_gateway_ok"] = ok
			continue
		}
		internetSeen = true
		if ok == 1.0 {
			internetOK = true
		}
	}

	if internetSeen {
		out["wan_cloudflare_ok"] = 0.0
		if internetOK {
			out["wan_cloudflare_ok"] = 1.0
		}
	}
	out["loss_pct"] = worstLoss
	return out
}

func writeSSEEvent(buf *bytes.Buffer, event string, data string) {
	buf.WriteString("event: " + event + "\n")
	// SSE protocol requires each data line to be prefixed with "data: ".
	// Multi-line HTML must be encoded this way, otherwise blank lines
	// in the HTML prematurely terminate the SSE event.
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		buf.WriteString("data: " + line + "\n")
	}
	buf.WriteString("\n")
}

func chartWindowPoints(window string, sampleInterval time.Duration) (string, int) {
	if sampleInterval <= 0 {
		sampleInterval = 5 * time.Second
	}
	pointsFor := func(d time.Duration) int {
		n := int(d / sampleInterval)
		if n < 12 {
			return 12
		}
		return n
	}

	switch strings.ToLower(strings.TrimSpace(window)) {
	case "5m", "live":
		return "5m", pointsFor(5 * time.Minute)
	case "2h", "120m":
		return "2h", pointsFor(2 * time.Hour)
	case "6h", "360m":
		return "6h", pointsFor(6 * time.Hour)
	case "60m":
		return "2h", pointsFor(2 * time.Hour)
	case "12h":
		return "12h", pointsFor(12 * time.Hour)
	case "24h":
		return "24h", pointsFor(24 * time.Hour)
	case "week":
		return "24h", pointsFor(24 * time.Hour)
	default:
		return "5m", pointsFor(5 * time.Minute)
	}
}

func (s *Server) gatherDashboardData(chartWindow string, chartPoints int) DashboardData {
	if chartPoints < 12 {
		chartPoints = 60
	}
	if chartWindow == "" {
		chartWindow = "5m"
	}
	dd := DashboardData{
		Uptime:         formatUptime(time.Since(s.startedAt)),
		Version:        Version,
		ChartWindow:    chartWindow,
		ChartPoints:    chartPoints,
		SystemStatus:   "unknown",
		SystemHint:     "waiting for telemetry",
		ServicesStatus: "unknown",
		ServicesHint:   "service health not loaded",
		DataStatus:     "unknown",
		DataHint:       "no sample received",
	}

	if s.collector != nil {
		dd.Metrics = s.collector.Latest()
		if dd.Metrics != nil && !dd.Metrics.Timestamp.IsZero() {
			age := time.Since(dd.Metrics.Timestamp)
			if age < 0 {
				age = 0
			}
			dd.MetricsAgeSec = int64(age / time.Second)
			staleThreshold := 3 * s.sseInterval()
			if staleThreshold < 20*time.Second {
				staleThreshold = 20 * time.Second
			}
			dd.MetricsStale = age > staleThreshold
			if dd.MetricsStale {
				dd.DataStatus = "warning"
				dd.DataHint = fmt.Sprintf("stale sample (%ds ago)", dd.MetricsAgeSec)
			} else {
				dd.DataStatus = "ok"
				dd.DataHint = fmt.Sprintf("fresh sample (%ds ago)", dd.MetricsAgeSec)
			}
			cpu := dd.Metrics.CPU.TotalPercent
			ram := dd.Metrics.RAM.Percent
			temp := 0.0
			if dd.Metrics.Temperature != nil {
				temp = *dd.Metrics.Temperature
			}
			switch {
			case cpu >= 90 || ram >= 90 || temp >= 80:
				dd.SystemStatus = "critical"
				dd.SystemHint = fmt.Sprintf("cpu %.0f%% ram %.0f%% temp %.1f°C", cpu, ram, temp)
			case cpu >= 75 || ram >= 75 || temp >= 70:
				dd.SystemStatus = "warning"
				dd.SystemHint = fmt.Sprintf("elevated load (cpu %.0f%% ram %.0f%%)", cpu, ram)
			default:
				dd.SystemStatus = "ok"
				dd.SystemHint = fmt.Sprintf("nominal (cpu %.0f%% ram %.0f%%)", cpu, ram)
			}
		}
		history := s.collector.History(chartPoints)
		dd.CPUValues = make([]float64, len(history))
		dd.RAMValues = make([]float64, len(history))
		dd.TempValues = make([]float64, len(history))
		lastTemp := 0.0
		for i, snap := range history {
			dd.CPUValues[i] = snap.CPU.TotalPercent
			dd.RAMValues[i] = snap.RAM.Percent
			if snap.Temperature != nil {
				lastTemp = *snap.Temperature
			}
			dd.TempValues[i] = lastTemp
		}
	}

	if s.docker != nil {
		dd.DockerAvail = s.docker.Available()
		dd.Containers = s.docker.Containers()
	}

	if s.gateway != nil {
		dd.Network = s.gateway.Snapshots()
		dd.NetworkTargets = s.gatherNetworkTargetViews(chartPoints)
	}

	if s.wan != nil {
		snap := s.wan.Snapshot()
		dd.WAN = &snap
	}

	if s.ups != nil {
		snap := s.ups.Current()
		dd.UPS = &snap
		// Cheap reads of the poll-time cache — no DB scan on the SSE render path.
		dd.UPSInsights = s.ups.CachedInsights()
		dd.UPSOnlineSince = s.ups.OnlineSinceLabel()
	}

	if s.systemd != nil {
		dd.SystemdAvail = s.systemd.Available()
		dd.Services = s.systemd.Services()
	}
	if len(dd.Services) == 0 && len(dd.Containers) == 0 {
		dd.ServicesStatus = "unknown"
		dd.ServicesHint = "no services or containers discovered"
	} else {
		failedServices := 0
		for _, svc := range dd.Services {
			if svc.ActiveState == "failed" {
				failedServices++
			}
		}
		runningContainers := 0
		for _, c := range dd.Containers {
			if c.State == "running" {
				runningContainers++
			}
		}
		if failedServices > 0 {
			dd.ServicesStatus = "critical"
			dd.ServicesHint = fmt.Sprintf("%d service(s) failed", failedServices)
		} else if runningContainers < len(dd.Containers) {
			dd.ServicesStatus = "warning"
			dd.ServicesHint = fmt.Sprintf("%d/%d containers running", runningContainers, len(dd.Containers))
		} else {
			dd.ServicesStatus = "ok"
			dd.ServicesHint = fmt.Sprintf("%d services active, %d containers running", countActiveServices(dd.Services), runningContainers)
		}
	}

	dd.ProcessStats = collectProcessUsage()

	// NOTE: Tailscale is NOT fetched here — it spawns an external process
	// and is only loaded on page load / manual refresh.

	return dd
}

func countActiveServices(services []systemd.ServiceInfo) int {
	active := 0
	for _, svc := range services {
		if svc.ActiveState == "active" {
			active++
		}
	}
	return active
}

// clientIPFromRequest returns the IP that should be subject to the SSE
// per-IP cap. X-Forwarded-For is honoured ONLY when the TCP peer
// (RemoteAddr) appears in the configured ULTRON_TRUSTED_PROXIES allowlist;
// otherwise the header is ignored and the TCP peer wins. With an empty
// allowlist (the default), every XFF value is dropped — the safe posture for
// a binary reached directly without a reverse proxy.
//
// @aitri-trace BG-020 BL-014
func (s *Server) clientIPFromRequest(r *http.Request) string {
	peer := tcpPeerIP(r.RemoteAddr)
	if peer != "" && s.cfg != nil && len(s.cfg.TrustedProxies) > 0 && isTrustedPeer(peer, s.cfg.TrustedProxies) {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			// Walk the XFF chain right-to-left. Skip every hop that is itself
			// trusted; the first untrusted hop is the original client. This
			// matches the Forwarded-For header convention used by nginx/caddy.
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				ip := strings.TrimSpace(parts[i])
				if ip == "" {
					continue
				}
				if !isTrustedPeer(ip, s.cfg.TrustedProxies) {
					return ip
				}
			}
		}
	}
	if peer != "" {
		return peer
	}
	return r.RemoteAddr
}

// tcpPeerIP extracts the host portion of an http.Request.RemoteAddr.
// Returns "" if the address is malformed.
func tcpPeerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return ""
	}
	return host
}

// isTrustedPeer reports whether ipStr is contained in any of the configured
// trusted-proxy networks.
func isTrustedPeer(ipStr string, trusted []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func collectProcessUsage() map[string]ProcessConsumer {
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()

	stats := map[string]ProcessConsumer{}
	out, err := exec.CommandContext(ctx, "ps", "-eo", "comm=,pcpu=,rss=").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) < 3 {
				continue
			}
			cpu, err1 := strconv.ParseFloat(fields[1], 64)
			rssKB, err2 := strconv.ParseUint(fields[2], 10, 64)
			if err1 != nil || err2 != nil {
				continue
			}
			comm := fields[0]
			cur := ProcessConsumer{
				Name:       comm,
				CPUPercent: cpu,
				RSSBytes:   rssKB * 1024,
			}
			prev, ok := stats[comm]
			if !ok || cur.CPUPercent > prev.CPUPercent || cur.RSSBytes > prev.RSSBytes {
				stats[comm] = cur
			}
		}
	}
	// Alias map for service names to process names shown by ps.
	for k, v := range map[string]string{
		"ultron-ap": "ultron-ap",
		"influxdb":  "influxd",
		"docker":    "dockerd",
		"pironman5": "pironman5-servi",
	} {
		if p, ok := stats[v]; ok {
			stats[k] = ProcessConsumer{
				Name:       k,
				CPUPercent: p.CPUPercent,
				RSSBytes:   p.RSSBytes,
			}
		}
	}
	return stats
}

func (s *Server) renderPartial(name string, data interface{}) string {
	tmpl, ok := s.tmplCache[name]
	if !ok {
		log.Printf("sse: template not in cache: %s", name)
		return ""
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("sse: render error for %s: %v", name, err)
		return ""
	}
	return buf.String()
}
