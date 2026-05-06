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
}

type sseBroker struct {
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	ipCount map[string]int
	// Small hard limits protect low-resource Pi from reconnect floods.
	maxClients int
	maxPerIP   int
}

func newSSEBroker() *sseBroker {
	return &sseBroker{
		clients:    make(map[*sseClient]struct{}),
		ipCount:    make(map[string]int),
		maxClients: 50,
		maxPerIP:   8,
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

// --- SSE Handler ---

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	s.setDashboardChartWindow(r.URL.Query().Get("window"))

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
	defer s.sseBroker.removeClient(client)

	// Send initial data immediately
	initial := s.buildSSEPayload()
	w.Write(initial)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
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
			data := s.buildSSEPayloadWithOptions(
				tick%chartsEvery == 0,
				tick%heavyEvery == 0,
			)
			s.sseBroker.broadcast(data)
			// Dynamically adjust ticker if interval was changed.
			if next := s.sseInterval(); next != current {
				current = next
				tick = 0
				ticker.Reset(current)
			}
		}
	}()
}

func (s *Server) buildSSEPayload() []byte {
	return s.buildSSEPayloadWithOptions(true, true)
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

func (s *Server) buildSSEPayloadWithOptions(includeCharts bool, includeHeavy bool) []byte {
	var buf bytes.Buffer
	dd := s.gatherDashboardData()
	if includeHeavy {
		// Heavy summary refreshes must include VPN peer state; otherwise
		// SSE swaps overwrite the initial page-load data with an empty list.
		dd.Tailscale = gatherTailscaleData()
	}

	// Metrics event
	metricsHTML := s.renderPartial("partials/sse-metrics.html", dd)
	writeSSEEvent(&buf, "metrics", metricsHTML)

	if includeCharts {
		// Charts event
		chartsHTML := s.renderPartial("partials/sse-charts.html", dd)
		writeSSEEvent(&buf, "charts", chartsHTML)
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
	// current snapshot, then render the active-set fragment.
	if s.insights != nil {
		s.evalInsightsTick(dd)
		verdictsHTML := s.renderVerdictsFragment(time.Now())
		writeSSEEvent(&buf, "verdicts", verdictsHTML)
	}

	return buf.Bytes()
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

	// WAN gateway / cloudflare ok flags inferred from the gatewayprobe
	// snapshot list (best-effort name match — labels are operator-set).
	for _, snap := range dd.Network {
		if snap == nil {
			continue
		}
		ok := 0.0
		if string(snap.Status) == "ok" {
			ok = 1.0
		}
		switch snap.Label {
		case "gateway":
			vars["wan_gateway_ok"] = lang.Number(ok)
		case "cloudflare":
			vars["wan_cloudflare_ok"] = lang.Number(ok)
		}
	}

	s.insights.EvalWithVars(now, vars)
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

func (s *Server) setDashboardChartWindow(window string) {
	normalized, points := chartWindowPoints(window, s.cfg.MetricsInterval)
	s.dashboardChartWindow.Store(normalized)
	s.dashboardHistoryPoints.Store(int64(points))
}

func (s *Server) gatherDashboardData() DashboardData {
	chartWindow, _ := s.dashboardChartWindow.Load().(string)
	chartPoints := int(s.dashboardHistoryPoints.Load())
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
