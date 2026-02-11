package docker

import "time"

// HealthStatus represents the health state of a container for UI display.
type HealthStatus string

const (
	HealthRunning HealthStatus = "running" // green
	HealthStopped HealthStatus = "stopped" // grey
	HealthError   HealthStatus = "error"   // red
	HealthPaused  HealthStatus = "paused"  // yellow
)

// ContainerInfo holds summary data for a single Docker container.
type ContainerInfo struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Image      string       `json:"image"`
	State      string       `json:"state"`
	Status     string       `json:"status"`
	Health     HealthStatus `json:"health"`
	CreatedAt  time.Time    `json:"created_at"`
	CPUPercent float64      `json:"cpu_percent"`
	MemUsage   uint64       `json:"mem_usage"`
	MemLimit   uint64       `json:"mem_limit"`
	MemPercent float64      `json:"mem_percent"`
}

// ContainerDetail holds extended data for a single container.
type ContainerDetail struct {
	ID          string        `json:"id"`
	Ports       []PortMapping `json:"ports"`
	Volumes     []VolumeMount `json:"volumes"`
	EnvVarNames []string      `json:"env_var_names"`
}

// PortMapping describes a port mapping between host and container.
type PortMapping struct {
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// VolumeMount describes a volume mount in a container.
type VolumeMount struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
}

// MapHealthStatus maps Docker container state + exit code to a HealthStatus.
func MapHealthStatus(state string, exitCode int) HealthStatus {
	switch state {
	case "running":
		return HealthRunning
	case "created", "paused":
		return HealthPaused
	case "exited", "dead":
		if exitCode != 0 {
			return HealthError
		}
		return HealthStopped
	default:
		return HealthStopped
	}
}
