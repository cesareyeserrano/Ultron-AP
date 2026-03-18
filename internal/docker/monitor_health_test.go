package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

// --- Tests: Health Status Mapping ---

func TestMapHealthStatus_Running(t *testing.T) {
	assert.Equal(t, HealthRunning, MapHealthStatus("running", 0))
}

func TestMapHealthStatus_ExitedClean(t *testing.T) {
	assert.Equal(t, HealthStopped, MapHealthStatus("exited", 0))
}

func TestMapHealthStatus_ExitedError(t *testing.T) {
	assert.Equal(t, HealthError, MapHealthStatus("exited", 1))
}

func TestMapHealthStatus_Dead(t *testing.T) {
	assert.Equal(t, HealthError, MapHealthStatus("dead", 137))
}

func TestMapHealthStatus_Paused(t *testing.T) {
	assert.Equal(t, HealthPaused, MapHealthStatus("paused", 0))
}

func TestMapHealthStatus_Created(t *testing.T) {
	assert.Equal(t, HealthPaused, MapHealthStatus("created", 0))
}

func TestMapHealthStatus_Unknown(t *testing.T) {
	assert.Equal(t, HealthStopped, MapHealthStatus("removing", 0))
}

// --- Tests: CPU Calculation ---

func TestCalculateCPUPercent(t *testing.T) {
	stats := sampleStats()
	pct := calculateCPUPercent(&stats)
	// cpuDelta=100M, sysDelta=500M, ratio=0.2, cpus=4, result=80%
	assert.InDelta(t, 80.0, pct, 0.1)
}

func TestCalculateCPUPercent_ZeroDelta(t *testing.T) {
	stats := &container.StatsResponse{
		Stats: container.Stats{
			CPUStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 100},
				SystemUsage: 100,
			},
			PreCPUStats: container.CPUStats{
				CPUUsage:    container.CPUUsage{TotalUsage: 100},
				SystemUsage: 100,
			},
		},
	}
	assert.Equal(t, 0.0, calculateCPUPercent(stats))
}

// --- Tests: Exit Code Parsing ---

func TestParseExitCode(t *testing.T) {
	tests := []struct {
		status string
		code   int
	}{
		{"Exited (0) 5 minutes ago", 0},
		{"Exited (1) 10 minutes ago", 1},
		{"Exited (137) 1 hour ago", 137},
		{"Up 2 hours", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.code, parseExitCode(tt.status))
		})
	}
}
