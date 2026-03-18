package server

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// handlePerformanceSave handles POST /api/performance
func (s *Server) handlePerformanceSave(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	cfg := database.DefaultPerformanceConfig()
	if v, err := strconv.Atoi(r.FormValue("sse_interval_sec")); err == nil && v >= 2 && v <= 60 {
		cfg.SSEIntervalSec = v
	}
	if v, err := strconv.Atoi(r.FormValue("disk_interval_min")); err == nil && v >= 1 && v <= 1440 {
		cfg.DiskIntervalMin = v
	}
	if v, err := strconv.Atoi(r.FormValue("docker_interval_sec")); err == nil && v >= 5 && v <= 300 {
		cfg.DockerIntervalSec = v
	}
	if v, err := strconv.Atoi(r.FormValue("systemd_interval_sec")); err == nil && v >= 5 && v <= 300 {
		cfg.SystemdIntervalSec = v
	}

	log.Printf("settings: saving performance config: SSE=%ds, Disk=%dm, Docker=%ds, Systemd=%ds",
		cfg.SSEIntervalSec, cfg.DiskIntervalMin, cfg.DockerIntervalSec, cfg.SystemdIntervalSec)

	if err := s.db.SavePerformanceConfig(cfg); err != nil {
		log.Printf("settings: failed to save performance config: %v", err)
		http.Error(w, "Failed to save: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.ApplyPerformanceConfig(cfg)

	w.Header().Set("HX-Trigger", `{"showToast": {"message": "Performance settings updated", "type": "success"}}`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<div class="text-sm text-green-400 py-2 flex items-center gap-2">` +
		`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"></polyline></svg>` +
		`Settings applied</div>`))
}

func (s *Server) handleBackupConfigSave(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	cfg := database.DefaultBackupConfig()
	cfg.Enabled = r.FormValue("enabled") == "on"
	if v, err := strconv.Atoi(r.FormValue("interval_hours")); err == nil {
		cfg.IntervalHours = v
	}
	if v, err := strconv.Atoi(r.FormValue("retention_count")); err == nil {
		cfg.RetentionCount = v
	}
	scheduleMode := strings.TrimSpace(r.FormValue("schedule_mode"))
	if scheduleMode != "" {
		cfg.ScheduleMode = scheduleMode
	}
	if v, err := strconv.Atoi(r.FormValue("schedule_hour")); err == nil {
		cfg.ScheduleHour = v
	}
	if v, err := strconv.Atoi(r.FormValue("schedule_minute")); err == nil {
		cfg.ScheduleMinute = v
	}
	mode := strings.TrimSpace(r.FormValue("destination_mode"))
	if mode != "" {
		cfg.DestinationMode = mode
	}
	cfg.LocalPath = strings.TrimSpace(r.FormValue("local_path"))
	cfg.EncryptEnabled = r.FormValue("encrypt_enabled") == "on"
	cfg.EncryptionKeyRef = strings.TrimSpace(r.FormValue("encryption_key_ref"))
	if v, err := strconv.Atoi(r.FormValue("upload_timeout_sec")); err == nil {
		cfg.UploadTimeoutSec = v
	}
	if v, err := strconv.Atoi(r.FormValue("max_upload_size_mb")); err == nil {
		cfg.MaxUploadSizeMB = v
	}

	if err := s.db.SaveBackupConfig(cfg); err != nil {
		log.Printf("settings: failed to save backup config: %v", err)
		http.Error(w, "Failed to save backup settings", http.StatusInternalServerError)
		return
	}
	s.ApplyBackupConfig(cfg)

	w.Header().Set("HX-Trigger", `{"showToast": {"message": "Backup settings updated", "type": "success"}}`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<div class="text-sm text-green-400 py-2 flex items-center gap-2">` +
		`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"></polyline></svg>` +
		`Backup settings applied</div>`))
}

func (s *Server) handleSettingsBackup(w http.ResponseWriter, r *http.Request) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("ultron-backup-%d.db", time.Now().Unix()))
	defer os.Remove(tmpFile)

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
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast": {"message": "Backup failed: %s", "type": "error"}}`, html.EscapeString(err.Error())))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"showToast": {"message": "Backup created and sent to Telegram", "type": "success"}}`)
	w.WriteHeader(http.StatusOK)
}
