package server

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
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
	Metrics      *metrics.Snapshot
	CPUValues    []float64 // Only what's needed for sparklines
	RAMValues    []float64
	TempValues   []float64
	ProcessStats map[string]ProcessConsumer
	Containers   []docker.ContainerInfo
	DockerAvail  bool
	Services     []systemd.ServiceInfo
	SystemdAvail bool
	Uptime       string
	Tailscale    TailscaleData
	Version      string
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
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

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
			heavyEvery := cadenceEvery(current, 30*time.Second) // docker/systemd/alerts
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

	// Metrics event
	metricsHTML := s.renderPartial("partials/sse-metrics.html", dd)
	writeSSEEvent(&buf, "metrics", metricsHTML)

	if includeCharts {
		// Charts event
		chartsHTML := s.renderPartial("partials/sse-charts.html", dd)
		writeSSEEvent(&buf, "charts", chartsHTML)
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
		Uptime:  formatUptime(time.Since(s.startedAt)),
		Version: Version,
	}

	if s.collector != nil {
		dd.Metrics = s.collector.Latest()
		// Only fetch the necessary numbers for sparklines (last 5 min = 60 points)
		history := s.collector.History(60)
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

	if s.systemd != nil {
		dd.SystemdAvail = s.systemd.Available()
		dd.Services = s.systemd.Services()
	}

	dd.ProcessStats = collectProcessUsage()

	// NOTE: Tailscale is NOT fetched here — it spawns an external process
	// and is only loaded on page load / manual refresh.

	return dd
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
