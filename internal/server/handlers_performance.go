package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// parseScheduleTime parses an "HH:MM" 24-hour time string into (hour, minute).
// Returns an error with the canonical message used by FR-064 / TC-SR-064f.
//
// @aitri-trace FR-064 — single helper for backup-schedule time parsing.
func parseScheduleTime(s string) (int, int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("time must be HH:MM in 00:00..23:59")
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("time must be HH:MM in 00:00..23:59")
	}
	return t.Hour(), t.Minute(), nil
}

// handlePerformanceSave handles POST /api/performance.
//
// @aitri-trace FR-057, FR-060 — uses RangeFor() for single source of truth.
// Out-of-range values now return 400 with the same hint string used in the
// label, instead of being silently dropped. Backwards compat: in-range
// values continue to succeed (NFR-029); a body that previously succeeded
// with valid numbers continues to succeed.
func (s *Server) handlePerformanceSave(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	cfg := database.DefaultPerformanceConfig()
	for _, field := range []*struct {
		Name string
		Dst  *int
	}{
		{"sse_interval_sec", &cfg.SSEIntervalSec},
		{"disk_interval_min", &cfg.DiskIntervalMin},
		{"docker_interval_sec", &cfg.DockerIntervalSec},
		{"systemd_interval_sec", &cfg.SystemdIntervalSec},
	} {
		raw := r.FormValue(field.Name)
		if raw == "" {
			continue
		}
		v, err := RangeFor(field.Name).ParseAndValidate(raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		*field.Dst = v
	}

	log.Printf("settings: saving performance config: SSE=%ds, Disk=%dm, Docker=%ds, Systemd=%ds",
		cfg.SSEIntervalSec, cfg.DiskIntervalMin, cfg.DockerIntervalSec, cfg.SystemdIntervalSec)

	if err := s.db.SavePerformanceConfig(cfg); err != nil {
		log.Printf("settings: failed to save performance config: %v", err)
		http.Error(w, "Failed to save: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.ApplyPerformanceConfig(cfg)

	setToast(w, "Performance settings updated", "success")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<div class="text-sm text-green-400 py-2 flex items-center gap-2">` +
		`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"></polyline></svg>` +
		`Settings applied</div>`))
}

func (s *Server) handleBackupConfigSave(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	// F5: base the update on the STORED config, not on defaults, so a POST that
	// omits a field preserves its saved value instead of silently resetting it.
	cfg, err := s.db.GetBackupConfig()
	if err != nil {
		cfg = database.DefaultBackupConfig()
	}
	cfg.Enabled = r.FormValue("enabled") == "on"

	// Numeric fields go through RangeFor() — out-of-range returns 400 with
	// the same hint string visible in the label.
	for _, field := range []*struct {
		Name string
		Dst  *int
	}{
		{"interval_hours", &cfg.IntervalHours},
		{"retention_count", &cfg.RetentionCount},
		{"upload_timeout_sec", &cfg.UploadTimeoutSec},
		{"max_upload_size_mb", &cfg.MaxUploadSizeMB},
	} {
		raw := r.FormValue(field.Name)
		if raw == "" {
			continue
		}
		v, err := RangeFor(field.Name).ParseAndValidate(raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		*field.Dst = v
	}
	scheduleMode := strings.TrimSpace(r.FormValue("schedule_mode"))
	if scheduleMode != "" {
		cfg.ScheduleMode = scheduleMode
	}

	// FR-064: accept either `time=HH:MM` (new) or legacy `hour=N&minute=M`.
	// New format wins when both are present. Log a deprecation warning on any
	// receipt of the legacy fields (NFR-029 backwards-compat).
	hourRaw := r.FormValue("schedule_hour")
	minuteRaw := r.FormValue("schedule_minute")
	timeRaw := strings.TrimSpace(r.FormValue("time"))
	if timeRaw != "" {
		h, m, err := parseScheduleTime(timeRaw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.ScheduleHour = h
		cfg.ScheduleMinute = m
		if hourRaw != "" || minuteRaw != "" {
			log.Printf(`level=warn msg="deprecated form-field" field=hour endpoint=/api/backup/config note="use time=HH:MM"`)
		}
	} else {
		if hourRaw != "" {
			h, err := RangeFor("schedule_hour").ParseAndValidate(hourRaw)
			if err != nil {
				http.Error(w, "hour must be 0..23", http.StatusBadRequest)
				return
			}
			cfg.ScheduleHour = h
			log.Printf(`level=warn msg="deprecated form-field" field=hour endpoint=/api/backup/config note="use time=HH:MM"`)
		}
		if minuteRaw != "" {
			m, err := RangeFor("schedule_minute").ParseAndValidate(minuteRaw)
			if err != nil {
				http.Error(w, "minute must be 0..59", http.StatusBadRequest)
				return
			}
			cfg.ScheduleMinute = m
		}
	}
	mode := strings.TrimSpace(r.FormValue("destination_mode"))
	if mode != "" {
		cfg.DestinationMode = mode
	}
	// Only touch local_path / encryption_key_ref when the form actually carries
	// them, so a partial POST cannot blank out a stored path or key ref (F5).
	if r.Form.Has("local_path") {
		rawLocalPath := strings.TrimSpace(r.FormValue("local_path"))
		cleanedLocalPath, err := database.ValidateBackupPath(rawLocalPath, s.cfg.BackupRoot)
		if err != nil {
			log.Printf("settings: rejected backup local_path %q: %v", rawLocalPath, err)
			http.Error(w, "Invalid backup path: "+err.Error(), http.StatusBadRequest)
			return
		}
		cfg.LocalPath = cleanedLocalPath
	}
	cfg.EncryptEnabled = r.FormValue("encrypt_enabled") == "on"
	if r.Form.Has("encryption_key_ref") {
		cfg.EncryptionKeyRef = strings.TrimSpace(r.FormValue("encryption_key_ref"))
	}

	if err := s.db.SaveBackupConfig(cfg); err != nil {
		log.Printf("settings: failed to save backup config: %v", err)
		http.Error(w, "Failed to save backup settings", http.StatusInternalServerError)
		return
	}
	s.ApplyBackupConfig(cfg)

	setToast(w, "Backup settings updated", "success")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<div class="text-sm text-green-400 py-2 flex items-center gap-2">` +
		`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"></polyline></svg>` +
		`Backup settings applied</div>`))
}

func (s *Server) handleSettingsBackup(w http.ResponseWriter, r *http.Request) {
	// B11: write into a private (0700) temp directory with an unpredictable
	// name instead of a guessable path in the world-shared temp dir. That
	// closes the pre-create/symlink race on the os.Remove→VACUUM INTO window,
	// since no other user can traverse into or plant a symlink inside the dir.
	tmpDir, err := os.MkdirTemp("", "ultron-backup-")
	if err != nil {
		log.Printf("settings: backup temp dir failed: %v", err)
		http.Error(w, "Backup failed", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)
	tmpFile := filepath.Join(tmpDir, "ultron.db")

	if err := s.db.Backup(tmpFile); err != nil {
		log.Printf("settings: backup failed: %v", err)
		http.Error(w, "Backup failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=ultron.db")
	w.Header().Set("Content-Type", "application/x-sqlite3")
	http.ServeFile(w, r, tmpFile)
}

func (s *Server) handleSettingsBackupRun(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	if err := s.performAutomatedBackup(); err != nil {
		log.Printf("settings: manual backup failed: %v", err)
		setToast(w, "Backup failed: "+err.Error(), "error")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	setToast(w, "Backup created and sent to Telegram", "success")
	w.WriteHeader(http.StatusOK)
}
