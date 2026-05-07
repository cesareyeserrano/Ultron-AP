// Package notify — event.go defines the rich notification context the alert
// engine assembles for each fire/resolve and passes through Notifier.Notify.
//
// @aitri-trace FR-016 FR-018 FR-019 FR-024
package notify

import (
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/cause"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/render"
)

// EventKind identifies a fresh fire or a recovery (resolve).
type EventKind string

const (
	EventFire    EventKind = "fire"
	EventResolve EventKind = "resolve"
)

// Surface identifies which alert family this event belongs to. Drives the
// surface-specific block selection in the renderer.
type Surface string

const (
	SurfaceResource Surface = "resource"
	SurfaceSystemd  Surface = "systemd"
	SurfaceDocker   Surface = "docker"
)

// Event is the read-only context passed to every Notifier.Notify call.
//
// Fields:
//   - Alert: required for fires, may be nil for engine-side resolve events.
//   - Rule:  resolved by the dispatcher via Alert.ConfigID lookup. May be
//     nil when no rule exists (the renderer falls back to "(threshold n/a)").
//   - Kind:  EventFire | EventResolve.
//   - Surface: derived by the engine from cfg.Metric / Source prefix.
//   - FirstFiredAt: the wall clock at which the rule first fired in the
//     current breach. Zero ⇒ unknown (renderer uses absolute timestamp).
//   - ResolvedAt: only valid when Kind == EventResolve.
//   - Hostname: cached at process start.
//   - PublicURL: ULTRON_PUBLIC_URL or derived from configured host/port.
type Event struct {
	Alert        *database.Alert
	Rule         *database.AlertConfig
	Kind         EventKind
	Surface      Surface
	FirstFiredAt time.Time
	ResolvedAt   time.Time
	Hostname     string
	PublicURL    string

	// Trend, when non-nil, is the FR-022 5-minute prior-vs-current sample
	// for resource fires. The dispatcher populates this from the metrics
	// ring buffer; resolves and non-resource surfaces leave it nil.
	Trend *render.Trend

	// Cause, when non-nil, is the FR-029 probable-cause line. The
	// dispatcher populates this via the cause package; nil ⇒ omit the
	// situation line.
	Cause *cause.Cause

	// Systemd, when non-nil, is the FR-020 surface block (unit + state
	// + active-since + journal tail). Populated by the dispatcher for
	// systemd-surface fires; the renderer omits the surface block on
	// nil.
	Systemd *render.SystemdData

	// Docker, when non-nil, is the FR-021 surface block (container +
	// image + state + exit code + log tail). Populated by the dispatcher
	// for docker-surface fires.
	Docker *render.DockerData
}

// SurfaceFromSource maps an Alert.Source value to the renderer's surface
// taxonomy. Source uses the engine's "docker:<name>" / "systemd:<name>"
// prefix convention (see internal/alerts/engine.go); bare metric ids
// like "cpu" / "ram" are resource alerts.
func SurfaceFromSource(source string) Surface {
	switch {
	case strings.HasPrefix(source, "docker:"):
		return SurfaceDocker
	case strings.HasPrefix(source, "systemd:"):
		return SurfaceSystemd
	default:
		return SurfaceResource
	}
}

// renderKind returns the corresponding render.EventKind for this Event.
func (e *Event) renderKind() render.EventKind {
	if e.Kind == EventResolve {
		return render.KindResolve
	}
	return render.KindFire
}

// renderSurface returns the render.Surface for this Event.
func (e *Event) renderSurface() render.Surface {
	switch e.Surface {
	case SurfaceSystemd:
		return render.SurfaceSystemd
	case SurfaceDocker:
		return render.SurfaceDocker
	default:
		return render.SurfaceResource
	}
}
