// Module: ai/snapshot
// Purpose: Read-only collection of already-gathered telemetry (metrics, active
//          insights, docker + systemd state) into a transient snapshot used to
//          build a prompt. Calls ONLY read methods — no collectors, no actions.
// Dependencies: internal/metrics, internal/insights, internal/docker, internal/systemd.
package ai

import (
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/insights"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

// MetricsReader exposes the latest metrics snapshot (read-only).
type MetricsReader interface {
	Latest() *metrics.Snapshot
}

// InsightsReader lists the currently active rule-based insights (read-only).
type InsightsReader interface {
	Active() []insights.Verdict
}

// DockerReader lists current container state (read-only).
type DockerReader interface {
	Available() bool
	Containers() []docker.ContainerInfo
}

// SystemdReader lists current service state (read-only).
type SystemdReader interface {
	Available() bool
	Services() []systemd.ServiceInfo
}

// Sources bundles the read-only telemetry providers. Any field may be nil; a nil
// source simply contributes nothing to the snapshot (graceful degradation).
type Sources struct {
	Metrics  MetricsReader
	Insights InsightsReader
	Docker   DockerReader
	Systemd  SystemdReader
}

// TelemetrySnapshot is a transient, never-persisted view of current state.
type TelemetrySnapshot struct {
	Metrics      *metrics.Snapshot
	Verdicts     []insights.Verdict
	Containers   []docker.ContainerInfo
	Services     []systemd.ServiceInfo
	FocusInsight *insights.Verdict // the insight identified by scope, when present
}

// Collect reads all available sources into a snapshot. For an insight-scoped
// request it also resolves the focused insight by rule id.
func Collect(src Sources, scope Scope) TelemetrySnapshot {
	var snap TelemetrySnapshot
	if src.Metrics != nil {
		snap.Metrics = src.Metrics.Latest()
	}
	if src.Insights != nil {
		snap.Verdicts = src.Insights.Active()
	}
	if src.Docker != nil && src.Docker.Available() {
		snap.Containers = src.Docker.Containers()
	}
	if src.Systemd != nil && src.Systemd.Available() {
		snap.Services = src.Systemd.Services()
	}
	if scope.Kind == ScopeInsight && scope.InsightID != "" {
		for i := range snap.Verdicts {
			if snap.Verdicts[i].RuleID == scope.InsightID {
				v := snap.Verdicts[i]
				snap.FocusInsight = &v
				break
			}
		}
	}
	return snap
}

// Sufficient reports whether there is enough data to attempt an explanation. An
// insight-scoped request needs the focused insight to exist; a system-scoped
// request needs at least one signal (FR-016 — empty telemetry → ErrInsufficient).
func (s TelemetrySnapshot) Sufficient(scope Scope) bool {
	if scope.Kind == ScopeInsight {
		return s.FocusInsight != nil
	}
	return s.Metrics != nil || len(s.Verdicts) > 0 || len(s.Containers) > 0 || len(s.Services) > 0
}
