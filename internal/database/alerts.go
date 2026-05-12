package database

import (
	"database/sql"
	"fmt"
	"time"
)

// AlertConfig represents a configured alert rule.
type AlertConfig struct {
	ID                int64
	Name              string
	Metric            string
	Operator          string
	Threshold         float64
	Target            *string
	SustainedDuration int
	Severity          string
	Enabled           bool
	CooldownMinutes   int
	CreatedAt         time.Time
	UpdatedAt         time.Time
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

func (ac AlertConfig) TargetDisplay() string {
	if ac.Target == nil || *ac.Target == "" {
		return "-"
	}
	return *ac.Target
}

func (ac AlertConfig) SustainedDisplay() string {
	if ac.Metric == "wan_outage" || ac.Metric == "public_ip_change" {
		return "-"
	}
	return fmt.Sprintf("%ds", ac.SustainedDuration)
}

func (ac AlertConfig) ConditionDisplay() string {
	if ac.Metric == "wan_outage" || ac.Metric == "public_ip_change" {
		return "-"
	}
	return fmt.Sprintf("%s %.1f", ac.Operator, ac.Threshold)
}

// CreateAlertConfig inserts a new alert rule.
func (db *DB) CreateAlertConfig(ac *AlertConfig) error {
	enabled := 0
	if ac.Enabled {
		enabled = 1
	}
	result, err := db.Exec(
		`INSERT INTO AlertConfig (name, metric, operator, threshold, target, sustained_duration, severity, enabled, cooldown_minutes)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ac.Name, ac.Metric, ac.Operator, ac.Threshold, ac.Target, ac.SustainedDuration, ac.Severity, enabled, ac.CooldownMinutes,
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
		`SELECT id, name, metric, operator, threshold, target, sustained_duration, severity, enabled, cooldown_minutes, created_at, updated_at
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
		var target sql.NullString
		if err := rows.Scan(&ac.ID, &ac.Name, &ac.Metric, &ac.Operator, &ac.Threshold,
			&target, &ac.SustainedDuration, &ac.Severity, &enabled, &ac.CooldownMinutes, &ac.CreatedAt, &ac.UpdatedAt); err != nil {
			return nil, fmt.Errorf("cannot scan alert config: %w", err)
		}
		if target.Valid {
			ac.Target = &target.String
		}
		ac.Enabled = enabled == 1
		configs = append(configs, ac)
	}
	return configs, rows.Err()
}

// ListEnabledAlertConfigs returns only enabled alert configs.
func (db *DB) ListEnabledAlertConfigs() ([]AlertConfig, error) {
	rows, err := db.Query(
		`SELECT id, name, metric, operator, threshold, target, sustained_duration, severity, enabled, cooldown_minutes, created_at, updated_at
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
		var target sql.NullString
		if err := rows.Scan(&ac.ID, &ac.Name, &ac.Metric, &ac.Operator, &ac.Threshold,
			&target, &ac.SustainedDuration, &ac.Severity, &enabled, &ac.CooldownMinutes, &ac.CreatedAt, &ac.UpdatedAt); err != nil {
			return nil, fmt.Errorf("cannot scan alert config: %w", err)
		}
		if target.Valid {
			ac.Target = &target.String
		}
		ac.Enabled = enabled == 1
		configs = append(configs, ac)
	}
	return configs, rows.Err()
}

// CreateAlert inserts a triggered alert.
func (db *DB) CreateAlert(a *Alert) error {
	a.CreatedAt = time.Now()
	ack := 0
	if a.Acknowledged {
		ack = 1
	}
	result, err := db.Exec(
		`INSERT INTO Alert (config_id, severity, message, source, value, acknowledged, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ConfigID, a.Severity, a.Message, a.Source, a.Value, ack, a.CreatedAt,
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
	var target sql.NullString
	err := db.QueryRow(
		`SELECT id, name, metric, operator, threshold, target, sustained_duration, severity, enabled, cooldown_minutes, created_at, updated_at
		 FROM AlertConfig WHERE id = ?`, id,
	).Scan(&ac.ID, &ac.Name, &ac.Metric, &ac.Operator, &ac.Threshold,
		&target, &ac.SustainedDuration, &ac.Severity, &enabled, &ac.CooldownMinutes, &ac.CreatedAt, &ac.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if target.Valid {
		ac.Target = &target.String
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get alert config: %w", err)
	}
	ac.Enabled = enabled == 1
	return &ac, nil
}

// UpdateAlertConfig updates an existing alert rule.
func (db *DB) UpdateAlertConfig(ac *AlertConfig) error {
	enabled := 0
	if ac.Enabled {
		enabled = 1
	}
	_, err := db.Exec(
		`UPDATE AlertConfig SET name=?, metric=?, operator=?, threshold=?, target=?, sustained_duration=?, severity=?, enabled=?, cooldown_minutes=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=?`,
		ac.Name, ac.Metric, ac.Operator, ac.Threshold, ac.Target, ac.SustainedDuration, ac.Severity, enabled, ac.CooldownMinutes, ac.ID,
	)
	if err != nil {
		return fmt.Errorf("cannot update alert config %d: %w", ac.ID, err)
	}
	return nil
}

// ToggleAlertConfig flips the enabled state of an alert config.
func (db *DB) ToggleAlertConfig(id int64) error {
	_, err := db.Exec(
		`UPDATE AlertConfig SET enabled = CASE WHEN enabled = 1 THEN 0 ELSE 1 END, updated_at=CURRENT_TIMESTAMP WHERE id=?`, id,
	)
	if err != nil {
		return fmt.Errorf("cannot toggle alert config %d: %w", id, err)
	}
	return nil
}

// DeleteAlertConfig removes an alert config by ID.
func (db *DB) DeleteAlertConfig(id int64) error {
	_, err := db.Exec("DELETE FROM AlertConfig WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("cannot delete alert config %d: %w", id, err)
	}
	return nil
}

// AcknowledgeAlert marks an alert as acknowledged.
func (db *DB) AcknowledgeAlert(id int64) error {
	_, err := db.Exec("UPDATE Alert SET acknowledged = 1 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("cannot acknowledge alert %d: %w", id, err)
	}
	return nil
}

// UnacknowledgedAlertCount returns the count of unacknowledged alerts.
func (db *DB) UnacknowledgedAlertCount() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM Alert WHERE acknowledged = 0").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cannot count unacknowledged alerts: %w", err)
	}
	return count, nil
}

// ListAlertsBySeverity returns alerts filtered by severity.
func (db *DB) ListAlertsBySeverity(severity string, limit int) ([]Alert, error) {
	rows, err := db.Query(
		`SELECT id, config_id, severity, message, source, value, acknowledged, created_at
		 FROM Alert WHERE severity = ? ORDER BY created_at DESC LIMIT ?`, severity, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot list alerts by severity: %w", err)
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

// DeleteAlerts removes alerts. If severity is empty, removes all.
func (db *DB) DeleteAlerts(severity string) (int64, error) {
	return deleteAlertsExec(db, severity)
}

// DeleteAlertsTx is the in-transaction variant of DeleteAlerts. Use with
// WithAuditTx to commit the delete and its audit log entry atomically.
//
// @aitri-trace BG-024 BL-010
func (db *DB) DeleteAlertsTx(tx *sql.Tx, severity string) (int64, error) {
	return deleteAlertsExec(tx, severity)
}

// dbExec is the common surface of *sql.DB and *sql.Tx — both expose Exec.
type dbExec interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func deleteAlertsExec(x dbExec, severity string) (int64, error) {
	var (
		res sql.Result
		err error
	)
	if severity == "" {
		res, err = x.Exec("DELETE FROM Alert")
	} else {
		res, err = x.Exec("DELETE FROM Alert WHERE severity = ?", severity)
	}
	if err != nil {
		return 0, fmt.Errorf("cannot delete alerts: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
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
