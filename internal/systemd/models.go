package systemd

import "time"

// ServiceHealth represents the health state of a systemd service for UI display.
type ServiceHealth string

const (
	ServiceActive   ServiceHealth = "active"   // green
	ServiceInactive ServiceHealth = "inactive" // grey
	ServiceFailed   ServiceHealth = "failed"   // red
)

// ServiceInfo holds status data for a single systemd service.
type ServiceInfo struct {
	Name        string        `json:"name"`
	LoadState   string        `json:"load_state"`
	ActiveState string        `json:"active_state"`
	SubState    string        `json:"sub_state"`
	Description string        `json:"description"`
	Health      ServiceHealth `json:"health"`
	Since       time.Time     `json:"since,omitempty"`
}

// MapServiceHealth maps a systemd active state to a ServiceHealth indicator.
func MapServiceHealth(activeState string) ServiceHealth {
	switch activeState {
	case "active", "reloading", "activating":
		return ServiceActive
	case "failed":
		return ServiceFailed
	default:
		return ServiceInactive
	}
}
