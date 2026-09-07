package docker

import (
	"testing"

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
			assert.Equal(t, tt.code, ParseExitCode(tt.status))
		})
	}
}
