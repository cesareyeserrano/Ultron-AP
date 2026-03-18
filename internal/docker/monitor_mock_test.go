package docker

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
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
	startErr      error
	stopErr       error
	restartErr    error
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

func (m *mockDockerClient) ContainerStart(_ context.Context, _ string, _ container.StartOptions) error {
	return m.startErr
}

func (m *mockDockerClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return m.stopErr
}

func (m *mockDockerClient) ContainerRestart(_ context.Context, _ string, _ container.StopOptions) error {
	return m.restartErr
}

func (m *mockDockerClient) ContainerLogs(_ context.Context, _ string, _ container.LogsOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockDockerClient) Close() error {
	return nil
}

// --- Helpers ---

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

// verify mock satisfies interface
var _ DockerClient = (*mockDockerClient)(nil)
