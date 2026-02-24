package server

import (
	"bytes"
	"fmt"
	"log"
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
	Metrics      *metrics.Snapshot
	CPUValues    []float64 // Only what's needed for sparklines
	RAMValues    []float64
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
		Uptime:  formatUptime(time.Since(s.startedAt)),
		Version: Version,
	}

	if s.collector != nil {
		dd.Metrics = s.collector.Latest()
		// Only fetch the necessary numbers for sparklines (last 5 min = 60 points)
		history := s.collector.History(60)
		dd.CPUValues = make([]float64, len(history))
		dd.RAMValues = make([]float64, len(history))
		for i, snap := range history {
			dd.CPUValues[i] = snap.CPU.TotalPercent
			dd.RAMValues[i] = snap.RAM.Percent
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
