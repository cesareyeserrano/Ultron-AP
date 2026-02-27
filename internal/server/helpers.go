package server

import (
	"fmt"
	"html/template"
	"math"
	"sort"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

const (
	tempWarnThresholdC = 60.0
	tempHighThresholdC = 75.0
)

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

// --- SVG & UI Helpers ---

func sparklineSVG(values []float64) template.HTML {
	return sparklineSVGColor(values, "var(--color-accent)")
}

func sparklineSVGColor(values []float64, strokeColor string) template.HTML {
	if len(values) == 0 {
		return ""
	}
	if strings.TrimSpace(strokeColor) == "" {
		strokeColor = "var(--color-accent)"
	}

	w, h := 300, 60
	minV, maxV := 0.0, 100.0
	points := make([]string, len(values))
	lastX, lastY := 0.0, 0.0
	for i, v := range values {
		x := float64(i) / float64(math.Max(1, float64(len(values)-1))) * float64(w)
		y := float64(h) - ((v - minV) / (maxV - minV) * float64(h))
		y = math.Max(1, math.Min(float64(h-1), y))
		points[i] = fmt.Sprintf("%.1f,%.1f", x, y)
		lastX, lastY = x, y
	}
	fillPoints := make([]string, 0, len(points)+2)
	fillPoints = append(fillPoints, fmt.Sprintf("0,%d", h-1))
	fillPoints = append(fillPoints, points...)
	fillPoints = append(fillPoints, fmt.Sprintf("%d,%d", w, h-1))

	svg := fmt.Sprintf(
		`<svg viewBox="0 0 %d %d" class="w-full h-16" preserveAspectRatio="none"><line x1="0" y1="%d" x2="%d" y2="%d" stroke="var(--color-border)" stroke-width="0.8" opacity="0.35"/><line x1="0" y1="%d" x2="%d" y2="%d" stroke="var(--color-border)" stroke-width="0.8" opacity="0.2"/><polygon points="%s" fill="%s" opacity="0.12"/><polyline points="%s" fill="none" stroke="%s" stroke-width="1.5" vector-effect="non-scaling-stroke"/><circle cx="%.1f" cy="%.1f" r="2.2" fill="%s"/></svg>`,
		w, h, h/2, w, h/2, h/4, w, h/4, strings.Join(fillPoints, " "), strokeColor, strings.Join(points, " "), strokeColor, lastX, lastY, strokeColor,
	)

	return template.HTML(svg)
}

func sparkMin(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func sparkMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func sparkCurrent(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}

func tempSeriesClass(values []float64) string {
	if len(values) == 0 {
		return "text-text-muted"
	}
	return tempClassForValue(sparkCurrent(values))
}

func tempSeriesStroke(values []float64) string {
	if len(values) == 0 {
		return "var(--color-accent)"
	}
	v := sparkCurrent(values)
	switch {
	case v > tempHighThresholdC:
		return "var(--color-danger)"
	case v >= tempWarnThresholdC:
		return "var(--color-yellow-400)"
	default:
		return "var(--color-green-400)"
	}
}

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

type serviceGroupData struct {
	Key      string
	Label    string
	Services []systemd.ServiceInfo
}

var serviceGroupOrder = []string{
	"core",
	"container",
	"network",
	"hardware",
	"observability",
	"system",
	"other",
}

func serviceGroup(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "ultron"):
		return "core"
	case strings.Contains(n, "docker") || strings.Contains(n, "containerd"):
		return "container"
	case strings.Contains(n, "tailscale") || strings.Contains(n, "network") || strings.Contains(n, "wpa") || strings.Contains(n, "dhcp"):
		return "network"
	case strings.Contains(n, "pironman") || strings.Contains(n, "fan") || strings.Contains(n, "gpio") || strings.Contains(n, "i2c"):
		return "hardware"
	case strings.Contains(n, "influx") || strings.Contains(n, "prometheus") || strings.Contains(n, "grafana"):
		return "observability"
	case strings.Contains(n, "systemd") || strings.Contains(n, "dbus") || strings.Contains(n, "ssh") || strings.Contains(n, "udev"):
		return "system"
	default:
		return "other"
	}
}

func serviceGroupLabel(key string) string {
	switch key {
	case "core":
		return "Ultron Core"
	case "container":
		return "Containers"
	case "network":
		return "Network"
	case "hardware":
		return "Hardware"
	case "observability":
		return "Observability"
	case "system":
		return "System"
	default:
		return "Other"
	}
}

func serviceGroupPillClass(key string) string {
	switch key {
	case "core":
		return "bg-accent/20 text-accent border-accent/20"
	case "container":
		return "bg-green-400/20 text-green-400 border-green-400/30"
	case "network":
		return "bg-yellow-400/20 text-yellow-400 border-yellow-400/30"
	case "hardware":
		return "bg-danger/20 text-danger border-danger/30"
	case "observability":
		return "bg-card text-text border-border/50"
	case "system":
		return "bg-surface text-text-muted border-border/50"
	default:
		return "bg-surface text-text-muted border-border/30"
	}
}

func groupServices(services []systemd.ServiceInfo) []serviceGroupData {
	if len(services) == 0 {
		return nil
	}
	buckets := map[string][]systemd.ServiceInfo{}
	for _, svc := range services {
		k := serviceGroup(svc.Name)
		buckets[k] = append(buckets[k], svc)
	}
	result := make([]serviceGroupData, 0, len(buckets))
	for _, key := range serviceGroupOrder {
		group, ok := buckets[key]
		if !ok {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].Name < group[j].Name
		})
		result = append(result, serviceGroupData{
			Key:      key,
			Label:    serviceGroupLabel(key),
			Services: group,
		})
	}
	return result
}

func serviceInfo(name, description string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSuffix(n, ".service")
	if strings.HasPrefix(n, "docker") {
		n = "docker"
	}
	if strings.Contains(n, "pironman5") {
		n = "pironman5"
	}
	if v, ok := map[string]string{
		"ultron-ap":        "Main Ultron web admin process.",
		"ultron-helper":    "Privileged helper boundary for controlled host actions.",
		"pironman5":        "Pironman hardware control daemon (fan/RGB/OLED).",
		"influxdb":         "Time-series database used by monitoring stack.",
		"influxd":          "InfluxDB time-series engine process.",
		"docker":           "Docker daemon for containers runtime.",
		"containerd":       "Container runtime used by Docker.",
		"tailscaled":       "Tailscale secure mesh VPN service.",
		"networkmanager":   "Network interface manager.",
		"rpi-connectd":     "Raspberry Pi remote connectivity agent.",
		"ssh":              "Secure shell access daemon.",
		"systemd-logind":   "Session and user login manager.",
		"systemd-journald": "System logs collector.",
	}[n]; ok {
		return v
	}
	if strings.TrimSpace(description) != "" {
		return description
	}
	return "System service."
}

func serviceProcessKey(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSuffix(n, ".service")
	switch n {
	case "influxdb":
		return "influxdb"
	case "docker":
		return "docker"
	case "pironman5":
		return "pironman5"
	case "dockerd":
		return "docker"
	default:
		if strings.HasPrefix(n, "docker-") || strings.HasPrefix(n, "docker.") {
			return "docker"
		}
		if strings.Contains(n, "pironman5") {
			return "pironman5"
		}
		return n
	}
}

func serviceHasRuntime(name string, stats map[string]ProcessConsumer) bool {
	if stats == nil {
		return false
	}
	_, ok := stats[serviceProcessKey(name)]
	return ok
}

func serviceCPU(name string, stats map[string]ProcessConsumer) float64 {
	if !serviceHasRuntime(name, stats) {
		return -1
	}
	return stats[serviceProcessKey(name)].CPUPercent
}

func serviceRSS(name string, stats map[string]ProcessConsumer) uint64 {
	if !serviceHasRuntime(name, stats) {
		return 0
	}
	return stats[serviceProcessKey(name)].RSSBytes
}

func countServicesState(services []systemd.ServiceInfo, state string) int {
	if len(services) == 0 {
		return 0
	}
	want := strings.ToLower(strings.TrimSpace(state))
	count := 0
	for _, svc := range services {
		if strings.ToLower(svc.ActiveState) == want {
			count++
		}
	}
	return count
}

func countContainersState(containers []docker.ContainerInfo, state string) int {
	if len(containers) == 0 {
		return 0
	}
	want := strings.ToLower(strings.TrimSpace(state))
	count := 0
	for _, c := range containers {
		if strings.ToLower(c.State) == want {
			count++
		}
	}
	return count
}

func tailscalePeerTotal(data TailscaleData) int {
	if !data.Available || data.Status == nil {
		return 0
	}
	return len(data.Status.Peers)
}

func tailscalePeerOnline(data TailscaleData) int {
	if !data.Available || data.Status == nil {
		return 0
	}
	count := 0
	for _, p := range data.Status.Peers {
		if p.Online {
			count++
		}
	}
	return count
}
