package server

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
	"github.com/cesareyeserrano/ultron-ap/internal/tailscale"
)

// DashboardData holds all data for dashboard rendering.
type DashboardData struct {
	Metrics          *metrics.Snapshot
	CPUHistory       []metrics.Snapshot
	RAMHistory       []metrics.Snapshot
	Containers       []docker.ContainerInfo
	DockerAvail      bool
	Services         []systemd.ServiceInfo
	SystemdAvail     bool
	Uptime           string
	Tailscale        TailscaleData
}

// TailscaleData is the data passed to the tailscale-peers partial.
type TailscaleData struct {
	Available bool
	Status    *tailscale.Status
}

func gatherTailscaleData() TailscaleData {
	if !tailscale.Available() {
		return TailscaleData{Available: false}
	}
	status, err := tailscale.GetStatus()
	if err != nil {
		return TailscaleData{Available: true}
	}
	return TailscaleData{Available: true, Status: status}
}

// --- SSE Broker ---

type sseClient struct {
	ch     chan []byte
	closed bool
}

type sseBroker struct {
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
}

func newSSEBroker() *sseBroker {
	return &sseBroker{
		clients: make(map[*sseClient]struct{}),
	}
}

func (b *sseBroker) addClient() *sseClient {
	c := &sseClient{ch: make(chan []byte, 8)}
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	return c
}

func (b *sseBroker) removeClient(c *sseClient) {
	b.mu.Lock()
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
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := s.sseBroker.addClient()
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
		for range ticker.C {
			s.sseBroker.mu.RLock()
			count := len(s.sseBroker.clients)
			s.sseBroker.mu.RUnlock()
			if count == 0 {
				continue
			}
			data := s.buildSSEPayload()
			s.sseBroker.broadcast(data)
			// Dynamically adjust ticker if interval was changed.
			if next := s.sseInterval(); next != current {
				current = next
				ticker.Reset(current)
			}
		}
	}()
}

func (s *Server) buildSSEPayload() []byte {
	var buf bytes.Buffer
	dd := s.gatherDashboardData()

	// Metrics event
	metricsHTML := s.renderPartial("partials/sse-metrics.html", dd)
	writeSSEEvent(&buf, "metrics", metricsHTML)

	// Docker event
	dockerHTML := s.renderPartial("partials/sse-docker.html", dd)
	writeSSEEvent(&buf, "docker", dockerHTML)

	// Systemd event
	systemdHTML := s.renderPartial("partials/sse-systemd.html", dd)
	writeSSEEvent(&buf, "systemd", systemdHTML)

	// Charts event
	chartsHTML := s.renderPartial("partials/sse-charts.html", dd)
	writeSSEEvent(&buf, "charts", chartsHTML)

	// Alert count event — uses a 30s TTL cache to avoid a DB query on every tick.
	if s.db != nil {
		unackCount := s.cachedAlertCount()
		if unackCount > 0 {
			writeSSEEvent(&buf, "alert-count", fmt.Sprintf("%d", unackCount))
		} else {
			writeSSEEvent(&buf, "alert-count", "")
		}
	}

	return buf.Bytes()
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

func (s *Server) gatherDashboardData() DashboardData {
	dd := DashboardData{
		Uptime: formatUptime(time.Since(s.startedAt)),
	}

	if s.collector != nil {
		dd.Metrics = s.collector.Latest()
		// Last 10 min at 5s interval = 120 points (sufficient for sparkline charts)
		dd.CPUHistory = s.collector.History(120)
		dd.RAMHistory = dd.CPUHistory // Same data, different field rendered
	}

	if s.docker != nil {
		dd.DockerAvail = s.docker.Available()
		dd.Containers = s.docker.Containers()
	}

	if s.systemd != nil {
		dd.SystemdAvail = s.systemd.Available()
		dd.Services = s.systemd.Services()
	}

	// NOTE: Tailscale is NOT fetched here — it spawns an external process
	// and is only loaded on page load / manual refresh.

	return dd
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

// --- Template Helpers ---

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), []string{"KB", "MB", "GB", "TB"}[exp])
}

func formatPercent(f float64) string {
	return fmt.Sprintf("%.1f%%", f)
}

func tempColor(temp *float64) string {
	if temp == nil {
		return "text-text-muted"
	}
	switch {
	case *temp > 75:
		return "text-danger"
	case *temp >= 60:
		return "text-yellow-400"
	default:
		return "text-green-400"
	}
}

func healthColor(h docker.HealthStatus) string {
	switch h {
	case docker.HealthRunning:
		return "bg-green-500"
	case docker.HealthError:
		return "bg-red-500"
	case docker.HealthPaused:
		return "bg-yellow-500"
	default:
		return "bg-gray-500"
	}
}

func svcHealthColor(h systemd.ServiceHealth) string {
	switch h {
	case systemd.ServiceActive:
		return "bg-green-500"
	case systemd.ServiceFailed:
		return "bg-red-500"
	default:
		return "bg-gray-500"
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// sparklineSVG generates a simple SVG polyline sparkline from metric history.
func sparklineSVG(snapshots []metrics.Snapshot, field string) template.HTML {
	if len(snapshots) == 0 {
		return ""
	}

	w, h := 300, 60
	maxPoints := 120 // Show up to 10 min of data at 5s intervals

	// Use the last maxPoints
	data := snapshots
	if len(data) > maxPoints {
		data = data[len(data)-maxPoints:]
	}

	// Extract values
	values := make([]float64, len(data))
	for i, s := range data {
		switch field {
		case "cpu":
			values[i] = s.CPU.TotalPercent
		case "ram":
			values[i] = s.RAM.Percent
		}
	}

	// Scale to SVG coords
	minV, maxV := 0.0, 100.0 // Percent always 0-100
	points := make([]string, len(values))
	for i, v := range values {
		x := float64(i) / float64(len(values)-1) * float64(w)
		y := float64(h) - ((v - minV) / (maxV - minV) * float64(h))
		y = math.Max(1, math.Min(float64(h-1), y))
		points[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}

	svg := fmt.Sprintf(
		`<svg viewBox="0 0 %d %d" class="w-full h-16" preserveAspectRatio="none"><polyline points="%s" fill="none" stroke="var(--color-accent)" stroke-width="1.5" vector-effect="non-scaling-stroke"/></svg>`,
		w, h, joinPoints(points),
	)

	return template.HTML(svg)
}

func formatTemp(temp *float64) string {
	if temp == nil {
		return "--"
	}
	return fmt.Sprintf("%.0f°C", *temp)
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func joinPoints(pts []string) string {
	var b strings.Builder
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(p)
	}
	return b.String()
}
