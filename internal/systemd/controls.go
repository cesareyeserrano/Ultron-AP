package systemd

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const controlTimeout = 30 * time.Second

// validServiceName matches safe systemd service names.
var validServiceName = regexp.MustCompile(`^[a-zA-Z0-9_.@\-]+$`)

// ServiceAction represents the result of a service control operation.
type ServiceAction struct {
	ServiceName string
	Action      string
	Success     bool
	Message     string
}

// StartService starts an inactive systemd service.
func (m *Monitor) StartService(ctx context.Context, name string) ServiceAction {
	return m.runControl(ctx, "start", name)
}

// StopService stops a running systemd service.
func (m *Monitor) StopService(ctx context.Context, name string) ServiceAction {
	return m.runControl(ctx, "stop", name)
}

// RestartService restarts a systemd service.
func (m *Monitor) RestartService(ctx context.Context, name string) ServiceAction {
	return m.runControl(ctx, "restart", name)
}

func (m *Monitor) runControl(ctx context.Context, action, name string) ServiceAction {
	result := ServiceAction{ServiceName: name, Action: action}

	if !validServiceName.MatchString(name) {
		result.Message = fmt.Sprintf("Invalid service name: %s", name)
		return result
	}

	if m.runner == nil {
		result.Message = "systemctl not available"
		return result
	}

	ctlCtx, cancel := context.WithTimeout(ctx, controlTimeout)
	defer cancel()

	out, err := m.runner.Run(ctlCtx, "systemctl", action, name)
	if err != nil {
		stderr := strings.TrimSpace(string(out))
		if strings.Contains(stderr, "Permission denied") || strings.Contains(stderr, "Interactive authentication") {
			result.Message = fmt.Sprintf("Permission denied: cannot %s %s", action, name)
		} else if strings.Contains(stderr, "not found") || strings.Contains(stderr, "No such") {
			result.Message = fmt.Sprintf("Service not found: %s", name)
		} else if ctlCtx.Err() != nil {
			result.Message = fmt.Sprintf("Timeout: %s %s did not complete in %v", action, name, controlTimeout)
		} else {
			msg := stderr
			if msg == "" {
				msg = err.Error()
			}
			result.Message = fmt.Sprintf("Failed to %s %s: %s", action, name, msg)
		}
		return result
	}

	result.Success = true
	result.Message = fmt.Sprintf("Service %s %sed", name, action)
	m.forceRefresh(ctx)
	return result
}

// forceRefresh triggers an immediate service list refresh.
func (m *Monitor) forceRefresh(ctx context.Context) {
	refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	m.refresh(refreshCtx)
}
