package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dclient "github.com/docker/docker/client"
)

const refreshInterval = 10 * time.Second

// Monitor periodically refreshes Docker container data.
type Monitor struct {
	client     DockerClient
	mu         sync.RWMutex
	containers []ContainerInfo
	available  bool
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewMonitor creates a Docker monitor. If Docker is not reachable, it logs a
// warning and returns a monitor that reports Available() == false.
func NewMonitor() *Monitor {
	m := &Monitor{}

	cli, err := dclient.NewClientWithOpts(dclient.FromEnv, dclient.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("docker: failed to create client: %v", err)
		return m
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		log.Printf("docker: daemon not reachable: %v", err)
		_ = cli.Close()
		return m
	}

	m.client = cli
	m.available = true
	return m
}

// newMonitorWithClient creates a monitor with an injected client (for testing).
func newMonitorWithClient(client DockerClient) *Monitor {
	return &Monitor{
		client:    client,
		available: client != nil,
	}
}

// Start begins periodic container refresh in a background goroutine.
func (m *Monitor) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.run(ctx)
	}()

	log.Printf("Docker monitor started (interval=%v)", refreshInterval)
}

// Stop cancels the refresh loop and waits for it to exit.
func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	if m.client != nil {
		_ = m.client.Close()
	}
	log.Println("Docker monitor stopped")
}

// Available reports whether Docker is reachable.
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
	if m.client == nil {
		return nil, fmt.Errorf("docker not available")
	}

	inspect, err := m.client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("inspect container %s: %w", id, err)
	}

	detail := &ContainerDetail{ID: id}

	// Ports
	if inspect.NetworkSettings != nil {
		for port, bindings := range inspect.NetworkSettings.Ports {
			for _, b := range bindings {
				detail.Ports = append(detail.Ports, PortMapping{
					HostPort:      b.HostPort,
					ContainerPort: string(port),
					Protocol:      port.Proto(),
				})
			}
		}
	}

	// Volumes
	for _, mount := range inspect.Mounts {
		detail.Volumes = append(detail.Volumes, VolumeMount{
			Source:      mount.Source,
			Destination: mount.Destination,
			Mode:        mount.Mode,
		})
	}

	// Env var names only (no values)
	if inspect.Config != nil {
		for _, env := range inspect.Config.Env {
			parts := strings.SplitN(env, "=", 2)
			detail.EnvVarNames = append(detail.EnvVarNames, parts[0])
		}
	}

	return detail, nil
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
	if m.client == nil {
		// Try to reconnect
		cli, err := dclient.NewClientWithOpts(dclient.FromEnv, dclient.WithAPIVersionNegotiation())
		if err != nil {
			return
		}
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, err = cli.Ping(pingCtx)
		cancel()
		if err != nil {
			_ = cli.Close()
			return
		}
		m.mu.Lock()
		m.client = cli
		m.available = true
		m.mu.Unlock()
		log.Println("docker: connected to daemon")
	}

	containers, err := m.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		log.Printf("docker: list error: %v", err)
		m.mu.Lock()
		m.available = false
		m.client = nil
		m.mu.Unlock()
		return
	}

	infos := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		info := containerToInfo(c)

		// Fetch stats only for running containers
		if c.State == "running" {
			m.fetchStats(ctx, c.ID, &info)
		}

		infos = append(infos, info)
	}

	m.mu.Lock()
	m.containers = infos
	m.available = true
	m.mu.Unlock()
}

func containerToInfo(c types.Container) ContainerInfo {
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	} else {
		// Container with no name — use truncated ID
		name = c.ID
		if len(name) > 12 {
			name = name[:12]
		}
	}

	exitCode := 0
	// Docker reports exit code in State for stopped containers
	// The Status string contains "Exited (1)" etc.
	if c.State == "exited" || c.State == "dead" {
		// Parse exit code from status text like "Exited (1) 5 minutes ago"
		exitCode = parseExitCode(c.Status)
	}

	return ContainerInfo{
		ID:        c.ID,
		Name:      name,
		Image:     c.Image,
		State:     c.State,
		Status:    c.Status,
		Health:    MapHealthStatus(c.State, exitCode),
		CreatedAt: time.Unix(c.Created, 0),
	}
}

// parseExitCode extracts exit code from Docker status string like "Exited (1) 5 minutes ago".
func parseExitCode(status string) int {
	var code int
	_, _ = fmt.Sscanf(status, "Exited (%d)", &code)
	return code
}

func (m *Monitor) fetchStats(ctx context.Context, id string, info *ContainerInfo) {
	shortID := id
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	statsResp, err := m.client.ContainerStats(ctx, id, false)
	if err != nil {
		log.Printf("docker: stats error for %s: %v", shortID, err)
		return
	}
	defer statsResp.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		log.Printf("docker: stats decode error for %s: %v", shortID, err)
		return
	}

	info.CPUPercent = calculateCPUPercent(&stats)
	info.MemUsage = stats.MemoryStats.Usage
	info.MemLimit = stats.MemoryStats.Limit
	if stats.MemoryStats.Limit > 0 {
		info.MemPercent = float64(stats.MemoryStats.Usage) / float64(stats.MemoryStats.Limit) * 100.0
	}
}

// calculateCPUPercent computes CPU usage percentage from Docker stats.
func calculateCPUPercent(stats *container.StatsResponse) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)

	if systemDelta <= 0 || cpuDelta <= 0 {
		return 0.0
	}

	cpus := float64(stats.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = 1.0
	}

	return (cpuDelta / systemDelta) * cpus * 100.0
}
