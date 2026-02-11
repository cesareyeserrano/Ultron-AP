package docker

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Docker Client ---

type mockDockerClient struct {
	containers    []types.Container
	inspectResult types.ContainerJSON
	statsJSON     container.StatsResponse
	listErr       error
	inspectErr    error
	statsErr      error
	pingErr       error
}

func (m *mockDockerClient) Ping(_ context.Context) (types.Ping, error) {
	return types.Ping{}, m.pingErr
}

func (m *mockDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
	return m.containers, m.listErr
}

func (m *mockDockerClient) ContainerStats(_ context.Context, _ string, _ bool) (container.StatsResponseReader, error) {
	if m.statsErr != nil {
		return container.StatsResponseReader{}, m.statsErr
	}
	data, _ := json.Marshal(m.statsJSON)
	return container.StatsResponseReader{
		Body: io.NopCloser(strings.NewReader(string(data))),
	}, nil
}

func (m *mockDockerClient) ContainerInspect(_ context.Context, _ string) (types.ContainerJSON, error) {
	return m.inspectResult, m.inspectErr
}

func (m *mockDockerClient) Close() error {
	return nil
}

// --- Helper ---

func sampleContainers() []types.Container {
	return []types.Container{
		{
			ID:      "abc123def456789012345678",
			Names:   []string{"/web-app"},
			Image:   "nginx:latest",
			State:   "running",
			Status:  "Up 2 hours",
			Created: time.Now().Add(-2 * time.Hour).Unix(),
		},
		{
			ID:      "def456ghi789012345678901",
			Names:   []string{"/db"},
			Image:   "postgres:16",
			State:   "exited",
			Status:  "Exited (0) 5 minutes ago",
			Created: time.Now().Add(-24 * time.Hour).Unix(),
		},
		{
			ID:      "ghi789jkl012345678901234",
			Names:   []string{"/worker"},
			Image:   "myapp:latest",
			State:   "exited",
			Status:  "Exited (1) 10 minutes ago",
			Created: time.Now().Add(-3 * time.Hour).Unix(),
		},
	}
}

func sampleStats() container.StatsResponse {
	return container.StatsResponse{
		Stats: container.Stats{
			CPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 200000000, // 200ms
				},
				SystemUsage: 1000000000, // 1s
				OnlineCPUs:  4,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage: container.CPUUsage{
					TotalUsage: 100000000, // 100ms
				},
				SystemUsage: 500000000, // 0.5s
			},
			MemoryStats: container.MemoryStats{
				Usage: 52428800,   // 50MB
				Limit: 1073741824, // 1GB
			},
		},
	}
}

// --- Tests: Health Status Mapping (AC3) ---

func TestMapHealthStatus_Running(t *testing.T) {
	assert.Equal(t, HealthRunning, MapHealthStatus("running", 0))
}

func TestMapHealthStatus_ExitedClean(t *testing.T) {
	assert.Equal(t, HealthStopped, MapHealthStatus("exited", 0))
}

func TestMapHealthStatus_ExitedError(t *testing.T) {
	assert.Equal(t, HealthError, MapHealthStatus("exited", 1))
}

func TestMapHealthStatus_Dead(t *testing.T) {
	assert.Equal(t, HealthError, MapHealthStatus("dead", 137))
}

func TestMapHealthStatus_Paused(t *testing.T) {
	assert.Equal(t, HealthPaused, MapHealthStatus("paused", 0))
}

func TestMapHealthStatus_Created(t *testing.T) {
	assert.Equal(t, HealthPaused, MapHealthStatus("created", 0))
}

func TestMapHealthStatus_Unknown(t *testing.T) {
	assert.Equal(t, HealthStopped, MapHealthStatus("removing", 0))
}

// --- Tests: Container Listing (AC1) ---

func TestMonitor_ListContainers(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleContainers(),
		statsJSON:  sampleStats(),
	}
	m := newMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 3)

	// Running container
	assert.Equal(t, "web-app", containers[0].Name)
	assert.Equal(t, "nginx:latest", containers[0].Image)
	assert.Equal(t, "running", containers[0].State)
	assert.Equal(t, "Up 2 hours", containers[0].Status)
	assert.Equal(t, HealthRunning, containers[0].Health)
	assert.False(t, containers[0].CreatedAt.IsZero())

	// Stopped container (clean exit)
	assert.Equal(t, "db", containers[1].Name)
	assert.Equal(t, HealthStopped, containers[1].Health)

	// Error container (exit code 1)
	assert.Equal(t, "worker", containers[2].Name)
	assert.Equal(t, HealthError, containers[2].Health)
}

func TestMonitor_ContainerNoName_UsesTruncatedID(t *testing.T) {
	mock := &mockDockerClient{
		containers: []types.Container{
			{
				ID:      "abcdef1234567890abcdef1234567890",
				Names:   nil, // no name
				Image:   "alpine:latest",
				State:   "running",
				Status:  "Up 1 minute",
				Created: time.Now().Unix(),
			},
		},
		statsJSON: sampleStats(),
	}
	m := newMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 1)
	assert.Equal(t, "abcdef123456", containers[0].Name)
}

// --- Tests: Per-container Metrics (AC2) ---

func TestMonitor_StatsForRunningContainer(t *testing.T) {
	mock := &mockDockerClient{
		containers: []types.Container{
			{
				ID:      "abc123",
				Names:   []string{"/web"},
				Image:   "nginx",
				State:   "running",
				Status:  "Up",
				Created: time.Now().Unix(),
			},
		},
		statsJSON: sampleStats(),
	}
	m := newMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 1)

	c := containers[0]
	assert.Greater(t, c.CPUPercent, 0.0)
	assert.Equal(t, uint64(52428800), c.MemUsage)
	assert.Equal(t, uint64(1073741824), c.MemLimit)
	assert.InDelta(t, 4.88, c.MemPercent, 0.1)
}

func TestMonitor_NoStatsForStoppedContainer(t *testing.T) {
	mock := &mockDockerClient{
		containers: []types.Container{
			{
				ID:      "abc123",
				Names:   []string{"/stopped"},
				Image:   "nginx",
				State:   "exited",
				Status:  "Exited (0)",
				Created: time.Now().Unix(),
			},
		},
	}
	m := newMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 1)
	assert.Equal(t, 0.0, containers[0].CPUPercent)
	assert.Equal(t, uint64(0), containers[0].MemUsage)
}

// --- Tests: Container Details (AC4) ---

func TestMonitor_ContainerDetail_Ports(t *testing.T) {
	mock := &mockDockerClient{
		inspectResult: types.ContainerJSON{
			NetworkSettings: &types.NetworkSettings{
				NetworkSettingsBase: types.NetworkSettingsBase{
					Ports: nat.PortMap{
						"80/tcp": []nat.PortBinding{
							{HostIP: "0.0.0.0", HostPort: "8080"},
						},
					},
				},
			},
			Mounts: nil,
			Config: &container.Config{},
		},
	}
	m := newMonitorWithClient(mock)
	detail, err := m.ContainerDetail(context.Background(), "abc123")
	require.NoError(t, err)

	require.Len(t, detail.Ports, 1)
	assert.Equal(t, "8080", detail.Ports[0].HostPort)
	assert.Equal(t, "80/tcp", detail.Ports[0].ContainerPort)
	assert.Equal(t, "tcp", detail.Ports[0].Protocol)
}

func TestMonitor_ContainerDetail_Volumes(t *testing.T) {
	mock := &mockDockerClient{
		inspectResult: types.ContainerJSON{
			NetworkSettings: &types.NetworkSettings{},
			Mounts: []types.MountPoint{
				{
					Type:        mount.TypeBind,
					Source:      "/host/data",
					Destination: "/app/data",
					Mode:        "rw",
				},
			},
			Config: &container.Config{},
		},
	}
	m := newMonitorWithClient(mock)
	detail, err := m.ContainerDetail(context.Background(), "abc123")
	require.NoError(t, err)

	require.Len(t, detail.Volumes, 1)
	assert.Equal(t, "/host/data", detail.Volumes[0].Source)
	assert.Equal(t, "/app/data", detail.Volumes[0].Destination)
	assert.Equal(t, "rw", detail.Volumes[0].Mode)
}

func TestMonitor_ContainerDetail_EnvVarNamesOnly(t *testing.T) {
	mock := &mockDockerClient{
		inspectResult: types.ContainerJSON{
			NetworkSettings: &types.NetworkSettings{},
			Config: &container.Config{
				Env: []string{
					"DATABASE_URL=postgres://localhost/db",
					"API_KEY=secret123",
					"NODE_ENV=production",
				},
			},
		},
	}
	m := newMonitorWithClient(mock)
	detail, err := m.ContainerDetail(context.Background(), "abc123")
	require.NoError(t, err)

	require.Len(t, detail.EnvVarNames, 3)
	assert.Equal(t, "DATABASE_URL", detail.EnvVarNames[0])
	assert.Equal(t, "API_KEY", detail.EnvVarNames[1])
	assert.Equal(t, "NODE_ENV", detail.EnvVarNames[2])
}

// --- Tests: Error Handling ---

func TestMonitor_DockerNotAvailable(t *testing.T) {
	m := newMonitorWithClient(nil)
	assert.False(t, m.Available())
	assert.Empty(t, m.Containers())
}

func TestMonitor_ContainerDetail_DockerNotAvailable(t *testing.T) {
	m := newMonitorWithClient(nil)
	_, err := m.ContainerDetail(context.Background(), "abc123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "docker not available")
}

func TestMonitor_ListError_SetsUnavailable(t *testing.T) {
	mock := &mockDockerClient{
		listErr: assert.AnError,
	}
	m := newMonitorWithClient(mock)
	m.refresh(context.Background())

	assert.False(t, m.Available())
	assert.Empty(t, m.Containers())
}

func TestMonitor_StatsError_SkipsStats(t *testing.T) {
	mock := &mockDockerClient{
		containers: []types.Container{
			{
				ID:      "abc123",
				Names:   []string{"/web"},
				Image:   "nginx",
				State:   "running",
				Status:  "Up",
				Created: time.Now().Unix(),
			},
		},
		statsErr: assert.AnError,
	}
	m := newMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 1)
	// Container is listed but stats are 0
	assert.Equal(t, 0.0, containers[0].CPUPercent)
	assert.Equal(t, uint64(0), containers[0].MemUsage)
}

// --- Tests: CPU Calculation ---

func TestCalculateCPUPercent(t *testing.T) {
	stats := sampleStats()
	pct := calculateCPUPercent(&stats)
	// cpuDelta=100M, sysDelta=500M, ratio=0.2, cpus=4, result=80%
	assert.InDelta(t, 80.0, pct, 0.1)
}

func TestCalculateCPUPercent_ZeroDelta(t *testing.T) {
	stats := &container.StatsResponse{
		Stats: container.Stats{
			CPUStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 100},
				SystemUsage: 100,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 100},
				SystemUsage: 100,
			},
		},
	}
	assert.Equal(t, 0.0, calculateCPUPercent(stats))
}

// --- Tests: Exit Code Parsing ---

func TestParseExitCode(t *testing.T) {
	tests := []struct {
		status string
		code   int
	}{
		{"Exited (0) 5 minutes ago", 0},
		{"Exited (1) 10 minutes ago", 1},
		{"Exited (137) 1 hour ago", 137},
		{"Up 2 hours", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.code, parseExitCode(tt.status))
		})
	}
}

// --- Tests: Monitor Goroutine Lifecycle (AC5) ---

func TestMonitor_StartStop(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleContainers(),
		statsJSON:  sampleStats(),
	}
	m := newMonitorWithClient(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	// Wait a moment for initial refresh
	time.Sleep(100 * time.Millisecond)

	containers := m.Containers()
	assert.NotEmpty(t, containers)
	assert.True(t, m.Available())

	m.Stop()

	// After stop, containers remain cached
	assert.NotEmpty(t, m.Containers())
}

func TestMonitor_ThreadSafety(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleContainers(),
		statsJSON:  sampleStats(),
	}
	m := newMonitorWithClient(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	defer m.Stop()

	// Concurrent reads should not race
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			_ = m.Containers()
			_ = m.Available()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// --- Tests: Containers returns copy ---

func TestMonitor_ContainersReturnsCopy(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleContainers(),
		statsJSON:  sampleStats(),
	}
	m := newMonitorWithClient(mock)
	m.refresh(context.Background())

	c1 := m.Containers()
	c2 := m.Containers()

	require.Len(t, c1, 3)
	c1[0].Name = "modified"
	assert.NotEqual(t, c1[0].Name, c2[0].Name)
}

// --- Tests: 50+ containers ---

func TestMonitor_ManyContainers(t *testing.T) {
	containers := make([]types.Container, 55)
	for i := range containers {
		containers[i] = types.Container{
			ID:      strings.Repeat("a", 64),
			Names:   []string{"/container-" + strings.Repeat("a", 3)},
			Image:   "alpine:latest",
			State:   "running",
			Status:  "Up",
			Created: time.Now().Unix(),
		}
	}

	mock := &mockDockerClient{
		containers: containers,
		statsJSON:  sampleStats(),
	}
	m := newMonitorWithClient(mock)
	m.refresh(context.Background())

	result := m.Containers()
	assert.Len(t, result, 55)
}

// --- Tests: inspect with no network settings ---

func TestMonitor_ContainerDetail_NilNetworkSettings(t *testing.T) {
	mock := &mockDockerClient{
		inspectResult: types.ContainerJSON{
			NetworkSettings: nil,
			Config:          &container.Config{},
		},
	}
	m := newMonitorWithClient(mock)
	detail, err := m.ContainerDetail(context.Background(), "abc")
	require.NoError(t, err)
	assert.Empty(t, detail.Ports)
}

// verify mock satisfies interface
var _ DockerClient = (*mockDockerClient)(nil)
