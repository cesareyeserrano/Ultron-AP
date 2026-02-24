package database

import (
	"database/sql"
	"fmt"
)

type BackupConfig struct {
	Enabled          bool
	IntervalHours    int
	RetentionCount   int
	ScheduleMode     string
	ScheduleHour     int
	ScheduleMinute   int
	DestinationMode  string
	LocalPath        string
	EncryptEnabled   bool
	EncryptionKeyRef string
	UploadTimeoutSec int
	MaxUploadSizeMB  int
}

func DefaultBackupConfig() BackupConfig {
	return BackupConfig{
		Enabled:          true,
		IntervalHours:    24,
		RetentionCount:   7,
		ScheduleMode:     "interval",
		ScheduleHour:     3,
		ScheduleMinute:   0,
		DestinationMode:  "local_only",
		LocalPath:        "",
		EncryptEnabled:   false,
		EncryptionKeyRef: "",
		UploadTimeoutSec: 30,
		MaxUploadSizeMB:  50,
	}
}

func normalizeBackupConfig(cfg BackupConfig) BackupConfig {
	out := cfg
	if out.IntervalHours < 1 {
		out.IntervalHours = 1
	}
	if out.IntervalHours > 720 {
		out.IntervalHours = 720
	}
	if out.RetentionCount < 1 {
		out.RetentionCount = 1
	}
	if out.RetentionCount > 200 {
		out.RetentionCount = 200
	}
	switch out.ScheduleMode {
	case "interval", "daily", "weekly", "biweekly":
	default:
		out.ScheduleMode = "interval"
	}
	if out.ScheduleHour < 0 {
		out.ScheduleHour = 0
	}
	if out.ScheduleHour > 23 {
		out.ScheduleHour = 23
	}
	if out.ScheduleMinute < 0 {
		out.ScheduleMinute = 0
	}
	if out.ScheduleMinute > 59 {
		out.ScheduleMinute = 59
	}
	if out.DestinationMode != "local_only" && out.DestinationMode != "local_plus_telegram" {
		out.DestinationMode = "local_only"
	}
	if out.UploadTimeoutSec < 5 {
		out.UploadTimeoutSec = 5
	}
	if out.UploadTimeoutSec > 300 {
		out.UploadTimeoutSec = 300
	}
	if out.MaxUploadSizeMB < 1 {
		out.MaxUploadSizeMB = 1
	}
	if out.MaxUploadSizeMB > 1024 {
		out.MaxUploadSizeMB = 1024
	}
	return out
}

func (db *DB) GetBackupConfig() (BackupConfig, error) {
	cfg := DefaultBackupConfig()
	var enabled int
	var encrypt int
	err := db.QueryRow(`SELECT enabled, interval_hours, retention_count, schedule_mode, schedule_hour, schedule_minute, destination_mode, local_path, encrypt_enabled, encryption_key_ref, upload_timeout_sec, max_upload_size_mb
		FROM BackupConfig WHERE id = 1`).Scan(
		&enabled,
		&cfg.IntervalHours,
		&cfg.RetentionCount,
		&cfg.ScheduleMode,
		&cfg.ScheduleHour,
		&cfg.ScheduleMinute,
		&cfg.DestinationMode,
		&cfg.LocalPath,
		&encrypt,
		&cfg.EncryptionKeyRef,
		&cfg.UploadTimeoutSec,
		&cfg.MaxUploadSizeMB,
	)
	if err == sql.ErrNoRows {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("cannot get backup config: %w", err)
	}
	cfg.Enabled = enabled == 1
	cfg.EncryptEnabled = encrypt == 1
	return normalizeBackupConfig(cfg), nil
}

func (db *DB) SaveBackupConfig(cfg BackupConfig) error {
	cfg = normalizeBackupConfig(cfg)
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	encrypt := 0
	if cfg.EncryptEnabled {
		encrypt = 1
	}
	_, err := db.Exec(`INSERT INTO BackupConfig (
			id, enabled, interval_hours, retention_count, schedule_mode, schedule_hour, schedule_minute, destination_mode, local_path, encrypt_enabled, encryption_key_ref, upload_timeout_sec, max_upload_size_mb
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			enabled=excluded.enabled,
			interval_hours=excluded.interval_hours,
			retention_count=excluded.retention_count,
			schedule_mode=excluded.schedule_mode,
			schedule_hour=excluded.schedule_hour,
			schedule_minute=excluded.schedule_minute,
			destination_mode=excluded.destination_mode,
			local_path=excluded.local_path,
			encrypt_enabled=excluded.encrypt_enabled,
			encryption_key_ref=excluded.encryption_key_ref,
			upload_timeout_sec=excluded.upload_timeout_sec,
			max_upload_size_mb=excluded.max_upload_size_mb,
			updated_at=CURRENT_TIMESTAMP`,
		enabled,
		cfg.IntervalHours,
		cfg.RetentionCount,
		cfg.ScheduleMode,
		cfg.ScheduleHour,
		cfg.ScheduleMinute,
		cfg.DestinationMode,
		cfg.LocalPath,
		encrypt,
		cfg.EncryptionKeyRef,
		cfg.UploadTimeoutSec,
		cfg.MaxUploadSizeMB,
	)
	if err != nil {
		return fmt.Errorf("cannot save backup config: %w", err)
	}
	return nil
}
