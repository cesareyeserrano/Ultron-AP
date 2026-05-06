package database

import (
	"encoding/json"
	"fmt"
)

// PerformanceConfig holds configurable polling/refresh intervals plus the
// alert-engine cooldowns FR-004 promises operators can tune. Cooldown
// fields default to 15 minutes — the literal value previously hardcoded
// in the engine — so existing deployments without saved config see
// identical behaviour to before BL-001.
type PerformanceConfig struct {
	SSEIntervalSec      int `json:",omitempty"` // dashboard push interval: 2–60s, default 5
	DiskIntervalMin     int `json:",omitempty"` // disk usage re-read interval: 1–1440min, default 30
	DockerIntervalSec   int `json:",omitempty"` // docker container refresh: 5–300s, default 10
	SystemdIntervalSec  int `json:",omitempty"` // systemd service refresh: 5–300s, default 30
	BackupIntervalHours int `json:",omitempty"` // automated backup interval: 1–720h, default 24

	// DockerCooldownMin and SystemdCooldownMin are the alert-engine
	// cooldown windows (FR-004 AC-002 — "configurable cooldown per alert
	// rule, default 15 min"). Range 1–1440 min (24h max). 0 means
	// "use default" (back-compat with rows persisted before BL-001).
	DockerCooldownMin  int `json:",omitempty"`
	SystemdCooldownMin int `json:",omitempty"`
}

// DefaultPerformanceConfig returns factory defaults.
func DefaultPerformanceConfig() PerformanceConfig {
	return PerformanceConfig{
		SSEIntervalSec:      5,
		DiskIntervalMin:     30,
		DockerIntervalSec:   10,
		SystemdIntervalSec:  30,
		BackupIntervalHours: 24,
		DockerCooldownMin:   15,
		SystemdCooldownMin:  15,
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
	// Back-compat: rows persisted before BL-001 lack the cooldown fields.
	// json.Unmarshal leaves them at zero — restore the documented default
	// so SetAlertCooldowns never receives a zero value at boot.
	if cfg.DockerCooldownMin <= 0 {
		cfg.DockerCooldownMin = 15
	}
	if cfg.SystemdCooldownMin <= 0 {
		cfg.SystemdCooldownMin = 15
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
