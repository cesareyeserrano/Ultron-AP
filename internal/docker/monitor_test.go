package docker

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Tests: Container Listing ---

func TestMonitor_ListContainers(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleContainers(),
		statsJSON:  sampleStats(),
	}
	m := NewMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 3)

	assert.Equal(t, "web-app", containers[0].Name)
	assert.Equal(t, "nginx:latest", containers[0].Image)
	assert.Equal(t, "running", containers[0].State)
	assert.Equal(t, "Up 2 hours", containers[0].Status)
	assert.Equal(t, HealthRunning, containers[0].Health)
	assert.False(t, containers[0].CreatedAt.IsZero())

	assert.Equal(t, "db", containers[1].Name)
	assert.Equal(t, HealthStopped, containers[1].Health)

	assert.Equal(t, "worker", containers[2].Name)
	assert.Equal(t, HealthError, containers[2].Health)
}

func TestMonitor_ContainerNoName_UsesTruncatedID(t *testing.T) {
	mock := &mockDockerClient{
		containers: []types.Container{
			{
				ID:      "abcdef1234567890abcdef1234567890",
				Names:   nil,
				Image:   "alpine:latest",
				State:   "running",
				Status:  "Up 1 minute",
				Created: time.Now().Unix(),
			},
		},
		statsJSON: sampleStats(),
	}
	m := NewMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 1)
	assert.Equal(t, "abcdef123456", containers[0].Name)
}

// --- Tests: Per-container Metrics ---

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
	m := NewMonitorWithClient(mock)
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
	m := NewMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 1)
	assert.Equal(t, 0.0, containers[0].CPUPercent)
	assert.Equal(t, uint64(0), containers[0].MemUsage)
}

// --- Tests: Monitor Goroutine Lifecycle ---

func TestMonitor_StartStop(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleContainers(),
		statsJSON:  sampleStats(),
	}
	m := NewMonitorWithClient(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	containers := m.Containers()
	assert.NotEmpty(t, containers)
	assert.True(t, m.Available())

	m.Stop()
	assert.NotEmpty(t, m.Containers())
}

func TestMonitor_ThreadSafety(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleContainers(),
		statsJSON:  sampleStats(),
	}
	m := NewMonitorWithClient(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	defer m.Stop()

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

func TestMonitor_ContainersReturnsCopy(t *testing.T) {
	mock := &mockDockerClient{
		containers: sampleContainers(),
		statsJSON:  sampleStats(),
	}
	m := NewMonitorWithClient(mock)
	m.refresh(context.Background())

	c1 := m.Containers()
	c2 := m.Containers()

	require.Len(t, c1, 3)
	c1[0].Name = "modified"
	assert.NotEqual(t, c1[0].Name, c2[0].Name)
}

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
	m := NewMonitorWithClient(mock)
	m.refresh(context.Background())

	result := m.Containers()
	assert.Len(t, result, 55)
}
