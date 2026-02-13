package database

import (
	"database/sql"
	"fmt"
	"time"
)

// AlertConfig represents a configured alert rule.
type AlertConfig struct {
	ID              int64
	Name            string
	Metric          string
	Operator        string
	Threshold       float64
	Severity        string
	Enabled         bool
	CooldownMinutes int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Alert represents a triggered alert record.
type Alert struct {
	ID           int64
	ConfigID     *int64
	Severity     string
	Message      string
	Source       string
	Value        *float64
	Acknowledged bool
	CreatedAt    time.Time
}

// CreateAlertConfig inserts a new alert rule.
func (db *DB) CreateAlertConfig(ac *AlertConfig) error {
	enabled := 0
	if ac.Enabled {
		enabled = 1
	}
	result, err := db.Exec(
		`INSERT INTO AlertConfig (name, metric, operator, threshold, severity, enabled, cooldown_minutes)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ac.Name, ac.Metric, ac.Operator, ac.Threshold, ac.Severity, enabled, ac.CooldownMinutes,
	)
	if err != nil {
		return fmt.Errorf("cannot create alert config: %w", err)
	}
	ac.ID, _ = result.LastInsertId()
	return nil
}

// ListAlertConfigs returns all alert configs.
func (db *DB) ListAlertConfigs() ([]AlertConfig, error) {
	rows, err := db.Query(
		`SELECT id, name, metric, operator, threshold, severity, enabled, cooldown_minutes, created_at, updated_at
		 FROM AlertConfig ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot list alert configs: %w", err)
	}
	defer rows.Close()

	var configs []AlertConfig
	for rows.Next() {
		var ac AlertConfig
		var enabled int
		if err := rows.Scan(&ac.ID, &ac.Name, &ac.Metric, &ac.Operator, &ac.Threshold,
			&ac.Severity, &enabled, &ac.CooldownMinutes, &ac.CreatedAt, &ac.UpdatedAt); err != nil {
			return nil, fmt.Errorf("cannot scan alert config: %w", err)
		}
		ac.Enabled = enabled == 1
		configs = append(configs, ac)
	}
	return configs, rows.Err()
}

// ListEnabledAlertConfigs returns only enabled alert configs.
func (db *DB) ListEnabledAlertConfigs() ([]AlertConfig, error) {
	rows, err := db.Query(
		`SELECT id, name, metric, operator, threshold, severity, enabled, cooldown_minutes, created_at, updated_at
		 FROM AlertConfig WHERE enabled = 1 ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot list enabled alert configs: %w", err)
	}
	defer rows.Close()

	var configs []AlertConfig
	for rows.Next() {
		var ac AlertConfig
		var enabled int
		if err := rows.Scan(&ac.ID, &ac.Name, &ac.Metric, &ac.Operator, &ac.Threshold,
			&ac.Severity, &enabled, &ac.CooldownMinutes, &ac.CreatedAt, &ac.UpdatedAt); err != nil {
			return nil, fmt.Errorf("cannot scan alert config: %w", err)
		}
		ac.Enabled = enabled == 1
		configs = append(configs, ac)
	}
	return configs, rows.Err()
}

// CreateAlert inserts a triggered alert.
func (db *DB) CreateAlert(a *Alert) error {
	ack := 0
	if a.Acknowledged {
		ack = 1
	}
	result, err := db.Exec(
		`INSERT INTO Alert (config_id, severity, message, source, value, acknowledged)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		a.ConfigID, a.Severity, a.Message, a.Source, a.Value, ack,
	)
	if err != nil {
		return fmt.Errorf("cannot create alert: %w", err)
	}
	a.ID, _ = result.LastInsertId()
	return nil
}

// ListAlerts returns alerts ordered by most recent first, limited to n rows.
func (db *DB) ListAlerts(limit int) ([]Alert, error) {
	rows, err := db.Query(
		`SELECT id, config_id, severity, message, source, value, acknowledged, created_at
		 FROM Alert ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		var ack int
		if err := rows.Scan(&a.ID, &a.ConfigID, &a.Severity, &a.Message, &a.Source,
			&a.Value, &ack, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("cannot scan alert: %w", err)
		}
		a.Acknowledged = ack == 1
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// AlertConfigCount returns the number of alert configs.
func (db *DB) AlertConfigCount() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM AlertConfig").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cannot count alert configs: %w", err)
	}
	return count, nil
}

// GetAlertConfig returns a single alert config by ID.
func (db *DB) GetAlertConfig(id int64) (*AlertConfig, error) {
	var ac AlertConfig
	var enabled int
	err := db.QueryRow(
		`SELECT id, name, metric, operator, threshold, severity, enabled, cooldown_minutes, created_at, updated_at
		 FROM AlertConfig WHERE id = ?`, id,
	).Scan(&ac.ID, &ac.Name, &ac.Metric, &ac.Operator, &ac.Threshold,
		&ac.Severity, &enabled, &ac.CooldownMinutes, &ac.CreatedAt, &ac.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get alert config: %w", err)
	}
	ac.Enabled = enabled == 1
	return &ac, nil
}

// SeedDefaultAlertConfigs inserts default alert rules if none exist.
func (db *DB) SeedDefaultAlertConfigs() error {
	count, err := db.AlertConfigCount()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaults := []AlertConfig{
		{Name: "High CPU", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15},
		{Name: "High Memory", Metric: "ram", Operator: ">", Threshold: 85, Severity: "warning", Enabled: true, CooldownMinutes: 15},
		{Name: "Disk Full", Metric: "disk", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 30},
		{Name: "High Temperature", Metric: "temp", Operator: ">", Threshold: 75, Severity: "warning", Enabled: true, CooldownMinutes: 15},
	}

	for i := range defaults {
		if err := db.CreateAlertConfig(&defaults[i]); err != nil {
			return fmt.Errorf("cannot seed default alert config %q: %w", defaults[i].Name, err)
		}
	}
	return nil
}
