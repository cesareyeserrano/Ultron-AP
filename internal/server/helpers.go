package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/network/wanmonitor"
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

// LinkState is the Network tile's verdict (FR-085). It lives only between the
// helper and the template — never persisted, never serialised, never sent over
// SSE as data.
type LinkState struct {
	Verdict   string  // "stable" | "unstable" | "offline" | "unknown"
	Reason    string  // e.g. "WAN up · 0% loss" or "15% loss · 8.8.8.8 ✕"
	WorstLoss float64 // 0..100 — the worst LossPct across the probes
}

// dashboardLinkState collapses the probe fleet and the WAN monitor into one
// word. It is the whole point of the tile: throughput has no good or bad value,
// so the tile could never be scanned; latency and loss do.
//
// Thresholds are NOT redefined here — it reads the same latencyCrit* constants
// the sparklines colour by (helpers_sparkline.go). Two threshold sets on one
// page would eventually contradict each other, and a tile that says "Stable"
// above a chart rendered red is worse than either threshold being off.
//
// Rule order matters and is deliberate:
//  1. no probes            → unknown (never claim a state we cannot know)
//  2. WAN monitor down     → offline (it needs 3 consecutive failures, so it
//     does not flap)
//  3. gateway not ok       → offline (a box that cannot reach its own gateway
//     has a broken LAN — a different problem from a lossy internet path)
//  4. any probe bad        → unstable
//  5. otherwise            → stable
func dashboardLinkState(probes []*gatewayprobe.Snapshot, wan *wanmonitor.Snapshot) LinkState {
	live := make([]*gatewayprobe.Snapshot, 0, len(probes))
	for _, p := range probes {
		if p != nil {
			live = append(live, p)
		}
	}
	if len(live) == 0 {
		return LinkState{Verdict: "unknown"}
	}

	var (
		worstLoss float64
		offender  *gatewayprobe.Snapshot
		gateway   *gatewayprobe.Snapshot
	)
	for _, p := range live {
		if p.Label == gatewayProbeLabel {
			gateway = p
		}
		if p.LossPct > worstLoss {
			worstLoss = p.LossPct
		}
		if probeUnhealthy(p) && offender == nil {
			offender = p
		}
	}

	// 2 — the WAN monitor is the authority on WAN reachability.
	if wan != nil && string(wan.State) == "down" {
		return LinkState{Verdict: "offline", Reason: "WAN down", WorstLoss: worstLoss}
	}

	// 3 — the LAN itself is broken.
	if gateway != nil && string(gateway.Status) != "ok" {
		return LinkState{
			Verdict:   "offline",
			Reason:    fmt.Sprintf("gateway %s", gateway.Status),
			WorstLoss: worstLoss,
		}
	}

	// 4 — a lossy or slow path.
	if offender != nil {
		reason := fmt.Sprintf("%.0f%% loss · %s", offender.LossPct, offender.Label)
		if string(offender.Status) != "ok" {
			reason = fmt.Sprintf("%s %s", offender.Label, offender.Status)
		}
		return LinkState{Verdict: "unstable", Reason: reason, WorstLoss: worstLoss}
	}

	// 5 — healthy.
	reason := fmt.Sprintf("%.0f%% loss", worstLoss)
	if wan != nil && string(wan.State) == "up" {
		reason = "WAN up · " + reason
	}
	return LinkState{Verdict: "stable", Reason: reason, WorstLoss: worstLoss}
}

// probeUnhealthy applies the EXISTING crit thresholds. Note what it does NOT
// do: trip on any loss at all. The probe's loss window is 20 samples, so a
// single dropped ping is 5% — a ">0%" rule would flip the tile to yellow on one
// lost packet and train the admin to ignore it (ADR-2).
func probeUnhealthy(p *gatewayprobe.Snapshot) bool {
	if s := strings.ToLower(strings.TrimSpace(string(p.Status))); s != "ok" && s != "init" && s != "" {
		return true
	}
	return p.LossPct >= latencyCritLossPct || p.RTTMs >= latencyCritRTTMs
}

// linkStateClass maps the verdict onto the tile classes the OTHER tiles already
// use, so Network inherits the same visual language rather than inventing one.
func linkStateClass(verdict string) string {
	switch verdict {
	case "offline":
		return "metric-critical"
	case "unstable":
		return "metric-warning"
	default:
		return ""
	}
}

func linkStateTextClass(verdict string) string {
	switch verdict {
	case "offline":
		return "text-danger"
	case "unstable":
		return "text-yellow-400"
	case "stable":
		return "text-green-400"
	default:
		return "text-text-muted"
	}
}

func linkStateLabel(verdict string) string {
	switch verdict {
	case "offline":
		return "Offline"
	case "unstable":
		return "Unstable"
	case "stable":
		return "Stable"
	default:
		return "Unknown"
	}
}

// primaryNetwork returns the busiest real interface — max(sent+recv), NEVER the
// sum of all of them. tailscale0 tunnels over eth0: adding them would
// double-count the same bytes the moment the VPN carries traffic, inflating the
// headline precisely when the admin is looking at it (ADR-3).
func primaryNetwork(ifaces []metrics.NetworkIface) *metrics.NetworkIface {
	candidates := dashboardNetworks(ifaces)
	if len(candidates) == 0 {
		return nil
	}
	best := 0
	for i := 1; i < len(candidates); i++ {
		if candidates[i].BytesSentPS+candidates[i].BytesRecvPS >
			candidates[best].BytesSentPS+candidates[best].BytesRecvPS {
			best = i
		}
	}
	out := candidates[best]
	return &out
}
