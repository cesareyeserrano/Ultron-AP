package systemd

import (
	"context"
	"log"
	"sync"
	"time"
)

const defaultSystemdInterval = 30 * time.Second

// Monitor periodically refreshes systemd service data.
type Monitor struct {
	runner    CommandRunner
	mu        sync.RWMutex
	services  []ServiceInfo
	available bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	intervalMu sync.RWMutex
	interval   time.Duration
}

// NewMonitor creates a systemd monitor. If systemctl is not available,
// the monitor logs a warning and returns Available() == false.
// SetInterval updates how often services are refreshed. Safe to call at any time.
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

func NewMonitor() *Monitor {
	m := &Monitor{runner: &ExecRunner{}, interval: defaultSystemdInterval}

	// Check if systemctl exists
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.runner.Run(ctx, "systemctl", "--version")
	if err != nil {
		log.Printf("systemd: systemctl not available: %v", err)
		m.available = false
		return m
	}

	m.available = true
	return m
}

// NewMonitorWithRunner creates a monitor with an injected runner (for testing).
func NewMonitorWithRunner(runner CommandRunner) *Monitor {
	return &Monitor{
		runner:    runner,
		available: runner != nil,
		interval:  defaultSystemdInterval,
	}
}

// Start begins periodic service refresh in a background goroutine.
func (m *Monitor) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.run(ctx)
	}()

	log.Printf("Systemd monitor started (interval=%v)", m.getInterval())
}

// Stop cancels the refresh loop and waits for it to exit.
func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	log.Println("Systemd monitor stopped")
}

// Available reports whether systemctl is reachable.
func (m *Monitor) Available() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.available
}

// Services returns the cached service list (thread-safe copy).
func (m *Monitor) Services() []ServiceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]ServiceInfo, len(m.services))
	copy(result, m.services)
	return result
}

// Failed returns only services with failed health status.
func (m *Monitor) Failed() []ServiceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var failed []ServiceInfo
	for _, s := range m.services {
		if s.Health == ServiceFailed {
			failed = append(failed, s)
		}
	}
	return failed
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

func (m *Monitor) refresh(ctx context.Context) {
	if m.runner == nil {
		return
	}

	output, err := m.runner.Run(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--plain")
	if err != nil {
		log.Printf("systemd: list-units error: %v", err)
		m.mu.Lock()
		m.available = false
		m.mu.Unlock()
		return
	}

	services := parseListUnits(string(output))
	m.applyActiveSince(ctx, services)

	m.mu.Lock()
	m.services = services
	m.available = true
	m.mu.Unlock()
}

// applyActiveSince fills ServiceInfo.Since (FR-003 / AC-003-002 "active-since")
// with each unit's ActiveEnterTimestamp. It runs one `systemctl show` for the
// whole set; a failure leaves Since at its zero value rather than failing the
// refresh, since the unit list itself is still useful without it.
func (m *Monitor) applyActiveSince(ctx context.Context, services []ServiceInfo) {
	if len(services) == 0 {
		return
	}

	args := []string{"show", "--no-pager", "--property=Id", "--property=ActiveEnterTimestamp"}
	for i := range services {
		args = append(args, services[i].Name+".service")
	}

	output, err := m.runner.Run(ctx, "systemctl", args...)
	if err != nil {
		log.Printf("systemd: active-since lookup failed: %v", err)
		return
	}

	stamps := parseShowTimestamps(string(output))
	for i := range services {
		if t, ok := stamps[services[i].Name]; ok {
			services[i].Since = t
		}
	}
}
