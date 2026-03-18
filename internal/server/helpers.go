package server

import (
	"fmt"

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
