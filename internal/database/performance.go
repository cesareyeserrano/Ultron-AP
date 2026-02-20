package database

import (
	"encoding/json"
	"fmt"
)

// PerformanceConfig holds configurable polling/refresh intervals.
type PerformanceConfig struct {
	SSEIntervalSec     int // dashboard push interval: 2–60s, default 5
	DiskIntervalMin    int // disk usage re-read interval: 1–1440min, default 30
	DockerIntervalSec  int // docker container refresh: 5–300s, default 10
	SystemdIntervalSec int // systemd service refresh: 5–300s, default 30
}

// DefaultPerformanceConfig returns factory defaults.
func DefaultPerformanceConfig() PerformanceConfig {
	return PerformanceConfig{
		SSEIntervalSec:     5,
		DiskIntervalMin:    30,
		DockerIntervalSec:  10,
		SystemdIntervalSec: 30,
	}
}

// GetPerformanceConfig loads the performance config from the database.
// Returns defaults if not yet configured.
func (db *DB) GetPerformanceConfig() (PerformanceConfig, error) {
	cfg := DefaultPerformanceConfig()
	nc, err := db.GetNotificationConfig("performance")
	if err != nil {
		return cfg, fmt.Errorf("cannot get performance config: %w", err)
	}
	if nc == nil {
		return cfg, nil // not configured yet — use defaults
	}
	if err := json.Unmarshal([]byte(nc.Config), &cfg); err != nil {
		return DefaultPerformanceConfig(), nil // corrupt data — fall back to defaults
	}
	return cfg, nil
}

// SavePerformanceConfig persists the performance config to the database.
func (db *DB) SavePerformanceConfig(cfg PerformanceConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("cannot marshal performance config: %w", err)
	}
	return db.UpsertNotificationConfig(&NotificationConfig{
		Channel: "performance",
		Enabled: true,
		Config:  string(data),
	})
}
