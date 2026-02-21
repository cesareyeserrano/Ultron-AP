package server

import (
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
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
	switch {
	case *temp > 75:
		return "text-danger"
	case *temp >= 60:
		return "text-yellow-400"
	default:
		return "text-green-400"
	}
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

func sparklineSVG(snapshots []metrics.Snapshot, field string) template.HTML {
	if len(snapshots) == 0 {
		return ""
	}

	w, h := 300, 60
	maxPoints := 120
	data := snapshots
	if len(data) > maxPoints {
		data = data[len(data)-maxPoints:]
	}

	values := make([]float64, len(data))
	for i, s := range data {
		switch field {
		case "cpu":
			values[i] = s.CPU.TotalPercent
		case "ram":
			values[i] = s.RAM.Percent
		}
	}

	minV, maxV := 0.0, 100.0
	points := make([]string, len(values))
	for i, v := range values {
		x := float64(i) / float64(len(values)-1) * float64(w)
		y := float64(h) - ((v - minV) / (maxV - minV) * float64(h))
		y = math.Max(1, math.Min(float64(h-1), y))
		points[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}

	svg := fmt.Sprintf(
		`<svg viewBox="0 0 %d %d" class="w-full h-16" preserveAspectRatio="none"><polyline points="%s" fill="none" stroke="var(--color-accent)" stroke-width="1.5" vector-effect="non-scaling-stroke"/></svg>`,
		w, h, strings.Join(points, " "),
	)

	return template.HTML(svg)
}
