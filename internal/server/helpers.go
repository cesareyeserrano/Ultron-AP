package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

const (
	tempWarnThresholdC = 60.0
	tempHighThresholdC = 75.0
)

// setToast emits an HTMX HX-Trigger header that pops a client-side toast. The
// payload is JSON-marshalled rather than hand-spliced with fmt.Sprintf, so a
// message containing quotes, backslashes, or other special characters can never
// corrupt the JSON the browser parses (D4). This is the correct escaping for a
// JSON-in-header context — html.EscapeString, used by several call sites
// before, was the wrong tool for it.
func setToast(w http.ResponseWriter, message, toastType string) {
	b, err := json.Marshal(map[string]any{
		"showToast": map[string]string{
			"message": message,
			"type":    toastType,
		},
	})
	if err != nil {
		return
	}
	w.Header().Set("HX-Trigger", string(b))
}

// --- Formatting Helpers ---

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), []string{"KB", "MB", "GB", "TB"}[exp])
}

func formatPercent(f float64) string {
	return fmt.Sprintf("%.1f%%", f)
}

func formatTemp(temp *float64) string {
	if temp == nil {
		return "--"
	}
	return fmt.Sprintf("%.0f°C", *temp)
}

func tempColor(temp *float64) string {
	if temp == nil {
		return "text-text-muted"
	}
	return tempClassForValue(*temp)
}

func healthColor(h docker.HealthStatus) string {
	switch h {
	case docker.HealthRunning:
		return "bg-green-500"
	case docker.HealthError:
		return "bg-red-500"
	case docker.HealthPaused:
		return "bg-yellow-500"
	default:
		return "bg-gray-500"
	}
}

func svcHealthColor(h systemd.ServiceHealth) string {
	switch h {
	case systemd.ServiceActive:
		return "bg-green-500"
	case systemd.ServiceFailed:
		return "bg-red-500"
	default:
		return "bg-gray-500"
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// --- Threshold Classifiers ---

func tempClassForValue(v float64) string {
	switch {
	case v > tempHighThresholdC:
		return "text-danger"
	case v >= tempWarnThresholdC:
		return "text-yellow-400"
	default:
		return "text-green-400"
	}
}

func usageClassForPercent(v float64) string {
	switch {
	case v >= 90:
		return "text-danger"
	case v >= 75:
		return "text-yellow-400"
	default:
		return "text-green-400"
	}
}

func usageStrokeForPercent(v float64) string {
	switch {
	case v >= 90:
		return "var(--color-danger)"
	case v >= 75:
		return "var(--color-yellow-400)"
	default:
		return "var(--color-green-400)"
	}
}

// activeSince renders a systemd unit's ActiveEnterTimestamp as a compact
// "active since" label (FR-003 / AC-003-002). Units that have never activated
// carry a zero timestamp and render nothing.
func activeSince(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	return formatUptime(d)
}

// virtualIfacePrefixes are interfaces the dashboard hides: container bridges
// (docker0, br-*), veth pairs, and loopback. They are real interfaces — the
// collector still records them — but on a Pi running containers they add a
// dozen always-zero rows that bury the two the admin actually watches (BG-072).
var virtualIfacePrefixes = []string{"lo", "docker", "br-", "veth", "virbr", "cni", "flannel", "kube"}

// dashboardNetworks returns the interfaces worth showing on the dashboard tile.
// Everything is filtered as virtual only when SOMETHING survives — a host whose
// only interface is unusual still sees its traffic rather than an empty tile.
func dashboardNetworks(all []metrics.NetworkIface) []metrics.NetworkIface {
	kept := make([]metrics.NetworkIface, 0, len(all))
	for _, n := range all {
		if isVirtualIface(n.Name) {
			continue
		}
		kept = append(kept, n)
	}
	if len(kept) == 0 {
		return all
	}
	return kept
}

func isVirtualIface(name string) bool {
	for _, p := range virtualIfacePrefixes {
		if name == p || strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// dashboardDisks returns the partitions worth showing. Firmware/boot mounts are
// hidden: they are a few hundred MB that never move, and on the Pi they doubled
// the tile's height for no signal (BG-072). The root filesystem always survives.
func dashboardDisks(all []metrics.DiskPartition) []metrics.DiskPartition {
	kept := make([]metrics.DiskPartition, 0, len(all))
	for _, d := range all {
		if strings.HasPrefix(d.Path, "/boot") {
			continue
		}
		kept = append(kept, d)
	}
	if len(kept) == 0 {
		return all
	}
	return kept
}
