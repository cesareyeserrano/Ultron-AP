package server

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

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

// sparkAvg returns the arithmetic mean of the series (0 when empty) — the
// "avg" figure the latency-style history tiles show.
func sparkAvg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func sparkMid(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minV, maxV := values[0], values[0]
	for _, v := range values[1:] {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return (minV + maxV) / 2
}

func tempSeriesClass(values []float64) string {
	if len(values) == 0 {
		return "text-text-muted"
	}
	return tempClassForValue(sparkCurrent(values))
}

func cpuSeriesClass(values []float64) string {
	if len(values) == 0 {
		return "text-text-muted"
	}
	return usageClassForPercent(sparkCurrent(values))
}

func cpuSeriesStroke(values []float64) string {
	if len(values) == 0 {
		return "var(--color-accent)"
	}
	return usageStrokeForPercent(sparkCurrent(values))
}

func ramSeriesClass(values []float64) string {
	if len(values) == 0 {
		return "text-text-muted"
	}
	return usageClassForPercent(sparkCurrent(values))
}

func ramSeriesStroke(values []float64) string {
	if len(values) == 0 {
		return "var(--color-accent)"
	}
	return usageStrokeForPercent(sparkCurrent(values))
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

// Latency probe thresholds (ms / loss %). Tuned for a home network: a few ms
// to the gateway is normal, sub-100 ms to most internet targets is fine.
const (
	latencyWarnRTTMs   = 80.0
	latencyCritRTTMs   = 200.0
	latencyWarnLossPct = 0.0  // any loss bumps to warn
	latencyCritLossPct = 10.0 // sustained loss bumps to crit
)

func latencyState(rttMs, lossPct float64, status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "", "init":
		return "muted"
	case "ok":
		// fall through to numeric checks
	default:
		return "crit"
	}
	if lossPct >= latencyCritLossPct || rttMs >= latencyCritRTTMs {
		return "crit"
	}
	if lossPct > latencyWarnLossPct || rttMs >= latencyWarnRTTMs {
		return "warn"
	}
	return "ok"
}

func latencySeriesClass(rttMs, lossPct float64, status string) string {
	switch latencyState(rttMs, lossPct, status) {
	case "crit":
		return "text-danger"
	case "warn":
		return "text-yellow-400"
	case "ok":
		return "text-green-400"
	default:
		return "text-text-muted"
	}
}

func latencySeriesStroke(rttMs, lossPct float64, status string) string {
	switch latencyState(rttMs, lossPct, status) {
	case "crit":
		return "var(--color-danger)"
	case "warn":
		return "var(--color-yellow-400)"
	case "ok":
		return "var(--color-green-400)"
	default:
		return "var(--color-accent)"
	}
}
