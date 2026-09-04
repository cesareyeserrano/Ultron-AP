package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// FanModes / OLEDMetrics are closed sets (FR-082 / FR-083). Values outside them
// are rejected with an error the HTTP layer turns into a 400 — never clamped,
// so a typo in a client never silently becomes a different setting.
var (
	FanModes    = []string{"auto", "quiet", "performance", "off"}
	OLEDMetrics = []string{"cpu", "ram", "temp", "ip"}
)

var (
	ErrInvalidFanMode    = errors.New("fan mode must be auto, quiet, performance or off")
	ErrInvalidOLEDMetric = errors.New("OLED metric must be cpu, ram, temp or ip")
)

// HardwareConfig is the Pironman panel configuration.
//
// Ultron STORES these values; it does not drive the fan or write to the OLED
// panel. That is deliberate and declared in the feature's no-go zone: a prior
// hardware-control attempt cost the Pi significant CPU and memory. The schema
// here is exactly what a future actuator would read, so nothing is wasted —
// but no goroutine, GPIO or I2C access exists in this codebase today.
type HardwareConfig struct {
	FanMode     string
	OLEDEnabled bool
	OLEDMetric  string
}

func DefaultHardwareConfig() HardwareConfig {
	return HardwareConfig{FanMode: "auto", OLEDEnabled: false, OLEDMetric: "cpu"}
}

// GetHardwareConfig reads the singleton row, creating it with defaults on first
// call. A missing row is never an error to the caller — the settings page must
// render on a fresh install.
func (db *DB) GetHardwareConfig() (HardwareConfig, error) {
	var c HardwareConfig
	var oledEnabled int

	err := db.QueryRow(`
		SELECT fan_mode, oled_enabled, oled_metric
		FROM HardwareConfig WHERE id = 1`,
	).Scan(&c.FanMode, &oledEnabled, &c.OLEDMetric)

	if errors.Is(err, sql.ErrNoRows) {
		def := DefaultHardwareConfig()
		if saveErr := db.SaveHardwareConfig(def); saveErr != nil {
			return def, fmt.Errorf("seed hardware config: %w", saveErr)
		}
		return def, nil
	}
	if err != nil {
		return DefaultHardwareConfig(), fmt.Errorf("read hardware config: %w", err)
	}

	c.OLEDEnabled = oledEnabled == 1
	return c, nil
}

// SaveHardwareConfig validates the enums and upserts the singleton row.
func (db *DB) SaveHardwareConfig(c HardwareConfig) error {
	if !contains(FanModes, c.FanMode) {
		return ErrInvalidFanMode
	}
	if !contains(OLEDMetrics, c.OLEDMetric) {
		return ErrInvalidOLEDMetric
	}

	oled := 0
	if c.OLEDEnabled {
		oled = 1
	}

	_, err := db.Exec(`
		INSERT INTO HardwareConfig (id, fan_mode, oled_enabled, oled_metric, updated_at)
		VALUES (1, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			fan_mode     = excluded.fan_mode,
			oled_enabled = excluded.oled_enabled,
			oled_metric  = excluded.oled_metric,
			updated_at   = CURRENT_TIMESTAMP`,
		c.FanMode, oled, c.OLEDMetric)
	if err != nil {
		return fmt.Errorf("save hardware config: %w", err)
	}
	return nil
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
