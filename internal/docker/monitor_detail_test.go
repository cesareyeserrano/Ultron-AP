package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Tests: Container Details ---

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
	m := NewMonitorWithClient(mock)
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
	m := NewMonitorWithClient(mock)
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
	m := NewMonitorWithClient(mock)
	detail, err := m.ContainerDetail(context.Background(), "abc123")
	require.NoError(t, err)

	require.Len(t, detail.EnvVarNames, 3)
	assert.Equal(t, "DATABASE_URL", detail.EnvVarNames[0])
	assert.Equal(t, "API_KEY", detail.EnvVarNames[1])
	assert.Equal(t, "NODE_ENV", detail.EnvVarNames[2])
}

func TestMonitor_ContainerDetail_NilNetworkSettings(t *testing.T) {
	mock := &mockDockerClient{
		inspectResult: types.ContainerJSON{
			NetworkSettings: nil,
			Config:          &container.Config{},
		},
	}
	m := NewMonitorWithClient(mock)
	detail, err := m.ContainerDetail(context.Background(), "abc")
	require.NoError(t, err)
	assert.Empty(t, detail.Ports)
}

// --- Tests: Error Handling ---

func TestMonitor_DockerNotAvailable(t *testing.T) {
	m := NewMonitorWithClient(nil)
	assert.False(t, m.Available())
	assert.Empty(t, m.Containers())
}

func TestMonitor_ContainerDetail_DockerNotAvailable(t *testing.T) {
	m := NewMonitorWithClient(nil)
	_, err := m.ContainerDetail(context.Background(), "abc123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "docker not available")
}

func TestMonitor_ListError_SetsUnavailable(t *testing.T) {
	mock := &mockDockerClient{
		listErr: assert.AnError,
	}
	m := NewMonitorWithClient(mock)
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
	m := NewMonitorWithClient(mock)
	m.refresh(context.Background())

	containers := m.Containers()
	require.Len(t, containers, 1)
	assert.Equal(t, 0.0, containers[0].CPUPercent)
	assert.Equal(t, uint64(0), containers[0].MemUsage)
}
