package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
)

const stopTimeout = 10 * time.Second

// ContainerAction represents the result of a container control operation.
type ContainerAction struct {
	ContainerID   string
	ContainerName string
	Action        string
	Success       bool
	Message       string
}

// Start starts a stopped container.
func (m *Monitor) StartContainer(ctx context.Context, containerID string) ContainerAction {
	result := ContainerAction{ContainerID: containerID, Action: "start"}

	if m.client == nil {
		result.Message = "Docker daemon unreachable"
		return result
	}

	name := m.containerName(containerID)
	result.ContainerName = name

	if err := m.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		result.Message = fmt.Sprintf("Failed to start %s: %s", name, err.Error())
		return result
	}

	result.Success = true
	result.Message = fmt.Sprintf("Container %s started", name)
	m.forceRefresh(ctx)
	return result
}

// StopContainer stops a running container with a default timeout.
func (m *Monitor) StopContainer(ctx context.Context, containerID string) ContainerAction {
	result := ContainerAction{ContainerID: containerID, Action: "stop"}

	if m.client == nil {
		result.Message = "Docker daemon unreachable"
		return result
	}

	name := m.containerName(containerID)
	result.ContainerName = name

	timeout := int(stopTimeout.Seconds())
	if err := m.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		result.Message = fmt.Sprintf("Failed to stop %s: %s", name, err.Error())
		return result
	}

	result.Success = true
	result.Message = fmt.Sprintf("Container %s stopped", name)
	m.forceRefresh(ctx)
	return result
}

// RestartContainer restarts a container with a default timeout.
func (m *Monitor) RestartContainer(ctx context.Context, containerID string) ContainerAction {
	result := ContainerAction{ContainerID: containerID, Action: "restart"}

	if m.client == nil {
		result.Message = "Docker daemon unreachable"
		return result
	}

	name := m.containerName(containerID)
	result.ContainerName = name

	timeout := int(stopTimeout.Seconds())
	if err := m.client.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		result.Message = fmt.Sprintf("Failed to restart %s: %s", name, err.Error())
		return result
	}

	result.Success = true
	result.Message = fmt.Sprintf("Container %s restarted", name)
	m.forceRefresh(ctx)
	return result
}

// containerName returns the human-readable name for a container ID from the cache.
func (m *Monitor) containerName(id string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.containers {
		if c.ID == id {
			return c.Name
		}
	}
	// Fall back to short ID if not in cache
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// forceRefresh triggers an immediate container list refresh.
func (m *Monitor) forceRefresh(ctx context.Context) {
	refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	m.refresh(refreshCtx)
}
