// Module:       internal/docker (monitor)
// Purpose:      Periodically refresh the container list the panel renders, and
//
//	serve detail and logs on demand. Since the C2 hardening the
//	data comes from the privileged helper; this process has no
//	access to the Docker socket.
//
// Dependencies: internal/docker/source.go (containerSource).
//
// @aitri-trace FR-088, FR-091, FR-092, FR-093, US-088, US-092
package docker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const defaultDockerInterval = 10 * time.Second

// Monitor periodically refreshes Docker container data through the helper.
type Monitor struct {
	src        containerSource
	mu         sync.RWMutex
	containers []ContainerInfo
	available  bool
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	intervalMu sync.RWMutex
	interval   time.Duration
}

// NewMonitor creates the production monitor, reading through the privileged
// helper. It does NOT probe the helper here: construction must never block
// startup, and an absent helper is a normal degraded state, not a failure
// (AC-093-004).
func NewMonitor() *Monitor {
	return &Monitor{src: newHelperSource(), interval: defaultDockerInterval}
}

// NewMonitorWithSource creates a monitor over an injected source, for tests.
//
// This replaces the former NewMonitorWithClient rather than aliasing it: the
// old name implied a Docker client still existed somewhere in this process,
// and it does not.
func NewMonitorWithSource(src containerSource) *Monitor {
	return &Monitor{src: src, interval: defaultDockerInterval}
}

// SetInterval updates how often containers are refreshed. Safe to call at any time.
func (m *Monitor) SetInterval(d time.Duration) {
	if d < time.Second {
		d = time.Second
	}
	m.intervalMu.Lock()
	m.interval = d
	m.intervalMu.Unlock()
}

func (m *Monitor) getInterval() time.Duration {
	m.intervalMu.RLock()
	defer m.intervalMu.RUnlock()
	return m.interval
}

// Start begins periodic container refresh in a background goroutine.
func (m *Monitor) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.run(ctx)
	}()

	log.Printf("Docker monitor started (interval=%v, source=privileged helper)", m.getInterval())
}

// Stop cancels the refresh loop and waits for it to exit.
func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	log.Println("Docker monitor stopped")
}

// Available reports whether the last refresh could read container data.
// False means "could not read", which the UI must render differently from
// "read fine, there are none" (FR-091).
func (m *Monitor) Available() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.available
}

// Containers returns the cached container list (thread-safe).
func (m *Monitor) Containers() []ContainerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ContainerInfo, len(m.containers))
	copy(result, m.containers)
	return result
}

// ContainerDetail fetches extended info for a single container on demand.
func (m *Monitor) ContainerDetail(ctx context.Context, id string) (*ContainerDetail, error) {
	if m.src == nil {
		return nil, fmt.Errorf("docker not available")
	}
	return m.src.Inspect(ctx, id)
}

// FetchLogs returns the last n lines of a container's output. The redaction
// is applied helper-side, before the text crosses back into this process.
func (m *Monitor) FetchLogs(ctx context.Context, id string, lines int) (string, error) {
	if m.src == nil {
		return "", fmt.Errorf("docker not available")
	}
	return m.src.Logs(ctx, id, lines)
}

func (m *Monitor) run(ctx context.Context) {
	m.refresh(ctx)

	current := m.getInterval()
	ticker := time.NewTicker(current)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refresh(ctx)
			// Dynamically adjust ticker if interval was changed via SetInterval.
			if next := m.getInterval(); next != current {
				current = next
				ticker.Reset(current)
			}
		}
	}
}

// refresh reads one snapshot from the source and swaps the cache.
//
// On failure it marks the monitor unavailable but KEEPS the last good list.
// A transient helper hiccup must not blank the panel for one tick and then
// repopulate it — that flicker reads as containers disappearing (AC-088-004,
// AC-006e). The availability flag is what the UI keys its error state off.
//
// The unavailable transition is logged once per state change, not once per
// tick, so a helper that is down for an hour does not write 360 lines
// (NFR-093).
func (m *Monitor) refresh(ctx context.Context) {
	if m.src == nil {
		return
	}

	containers, err := m.src.List(ctx)
	if err != nil {
		m.mu.Lock()
		wasAvailable := m.available
		m.available = false
		m.mu.Unlock()
		if wasAvailable {
			log.Printf("docker: helper unavailable, keeping last known list: %v", err)
		}
		return
	}

	m.mu.Lock()
	wasAvailable := m.available
	m.containers = containers
	m.available = true
	m.mu.Unlock()
	if !wasAvailable {
		log.Printf("docker: helper reachable, %d container(s)", len(containers))
	}
}
