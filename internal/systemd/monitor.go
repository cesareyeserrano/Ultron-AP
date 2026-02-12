package systemd

import (
	"context"
	"log"
	"sync"
	"time"
)

const refreshInterval = 30 * time.Second

// Monitor periodically refreshes systemd service data.
type Monitor struct {
	runner    CommandRunner
	mu        sync.RWMutex
	services  []ServiceInfo
	available bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewMonitor creates a systemd monitor. If systemctl is not available,
// the monitor logs a warning and returns Available() == false.
func NewMonitor() *Monitor {
	m := &Monitor{runner: &ExecRunner{}}

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

// newMonitorWithRunner creates a monitor with an injected runner (for testing).
func newMonitorWithRunner(runner CommandRunner) *Monitor {
	return &Monitor{
		runner:    runner,
		available: runner != nil,
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

	log.Printf("Systemd monitor started (interval=%v)", refreshInterval)
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

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refresh(ctx)
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

	m.mu.Lock()
	m.services = services
	m.available = true
	m.mu.Unlock()
}
