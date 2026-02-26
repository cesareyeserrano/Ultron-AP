package server

import (
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
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

func sparklineSVG(values []float64) template.HTML {
	if len(values) == 0 {
		return ""
	}

	w, h := 320.0, 92.0
	minV, maxV := 0.0, 100.0
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if maxV-minV < 10 {
		center := (maxV + minV) / 2
		minV = math.Max(0, center-5)
		maxV = math.Min(100, center+5)
	}
	if maxV <= minV {
		maxV = minV + 1
	}
	points := make([]string, len(values))
	coords := make([][2]float64, len(values))
	for i, v := range values {
		x := float64(i) / float64(math.Max(1, float64(len(values)-1))) * w
		y := h - ((v-minV)/(maxV-minV) * h)
		y = math.Max(2, math.Min(h-2, y))
		points[i] = fmt.Sprintf("%.1f,%.1f", x, y)
		coords[i] = [2]float64{x, y}
	}

	last := coords[len(coords)-1]
	areaPath := fmt.Sprintf("M 0,%.1f L %s L %.1f,%.1f Z", h, strings.Join(points, " L "), w, h)
	gradientID := fmt.Sprintf("sparkFill-%d-%d", len(values), int(last[1]*10))

	svg := fmt.Sprintf(
		`<svg viewBox="0 0 %.0f %.0f" class="sparkline-svg" preserveAspectRatio="none" role="img" aria-label="usage trend">
			<defs>
				<linearGradient id="%s" x1="0" y1="0" x2="0" y2="1">
					<stop offset="0%%" stop-color="var(--color-accent)" stop-opacity="0.38"/>
					<stop offset="100%%" stop-color="var(--color-accent)" stop-opacity="0.02"/>
				</linearGradient>
			</defs>
			<line x1="0" y1="23" x2="320" y2="23" stroke="rgba(148,163,184,0.22)" stroke-width="1"/>
			<line x1="0" y1="46" x2="320" y2="46" stroke="rgba(148,163,184,0.17)" stroke-width="1"/>
			<line x1="0" y1="69" x2="320" y2="69" stroke="rgba(148,163,184,0.12)" stroke-width="1"/>
			<path d="%s" fill="url(#%s)"/>
			<polyline points="%s" fill="none" stroke="var(--color-accent)" stroke-width="2.3" vector-effect="non-scaling-stroke" stroke-linecap="round" stroke-linejoin="round"/>
			<circle cx="%.1f" cy="%.1f" r="3.6" fill="var(--color-accent)"/>
		</svg>`,
		w, h, gradientID, areaPath, gradientID, strings.Join(points, " "), last[0], last[1],
	)

	return template.HTML(svg)
}
