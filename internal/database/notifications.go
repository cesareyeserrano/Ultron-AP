package database

import (
	"database/sql"
	"fmt"
)

// NotificationConfig stores configuration for a notification channel.
type NotificationConfig struct {
	ID      int64
	Channel string // "telegram" or "email"
	Enabled bool
	Config  string // JSON blob
}

// GetNotificationConfig returns config for a channel, or nil if not set.
func (db *DB) GetNotificationConfig(channel string) (*NotificationConfig, error) {
	var nc NotificationConfig
	var enabled int
	err := db.QueryRow(
		`SELECT id, channel, enabled, config FROM NotificationConfig WHERE channel = ?`, channel,
	).Scan(&nc.ID, &nc.Channel, &enabled, &nc.Config)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get notification config: %w", err)
	}
	nc.Enabled = enabled == 1
	return &nc, nil
}

// UpsertNotificationConfig inserts or updates a notification channel config.
func (db *DB) UpsertNotificationConfig(nc *NotificationConfig) error {
	enabled := 0
	if nc.Enabled {
		enabled = 1
	}
	_, err := db.Exec(
		`INSERT INTO NotificationConfig (channel, enabled, config)
		 VALUES (?, ?, ?)
		 ON CONFLICT(channel) DO UPDATE SET enabled=excluded.enabled, config=excluded.config, updated_at=CURRENT_TIMESTAMP`,
		nc.Channel, enabled, nc.Config,
	)
	if err != nil {
		return fmt.Errorf("cannot upsert notification config: %w", err)
	}
	return nil
}

// ListNotificationConfigs returns all notification configs.
func (db *DB) ListNotificationConfigs() ([]NotificationConfig, error) {
	rows, err := db.Query(`SELECT id, channel, enabled, config FROM NotificationConfig ORDER BY channel`)
	if err != nil {
		return nil, fmt.Errorf("cannot list notification configs: %w", err)
	}
	defer rows.Close()

	var configs []NotificationConfig
	for rows.Next() {
		var nc NotificationConfig
		var enabled int
		if err := rows.Scan(&nc.ID, &nc.Channel, &enabled, &nc.Config); err != nil {
			return nil, fmt.Errorf("cannot scan notification config: %w", err)
		}
		nc.Enabled = enabled == 1
		configs = append(configs, nc)
	}
	return configs, rows.Err()
}
