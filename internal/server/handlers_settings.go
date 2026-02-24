package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify"
)

type settingsData struct {
	Rules    []database.AlertConfig
	Telegram *notifDisplay
	Email    *notifDisplay
	Perf     database.PerformanceConfig
	Backup   database.BackupConfig
	Flash    string
}

type notifDisplay struct {
	Enabled bool
	Fields  map[string]string // display values (masked)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	rules, err := s.db.ListAlertConfigs()
	if err != nil {
		log.Printf("settings: failed to list rules: %v", err)
	}

	data := settingsData{Rules: rules}

	// Load notification configs
	if tg, err := s.db.GetNotificationConfig("telegram"); err == nil && tg != nil {
		data.Telegram = maskNotifConfig(tg, "telegram")
	}
	if em, err := s.db.GetNotificationConfig("email"); err == nil && em != nil {
		data.Email = maskNotifConfig(em, "email")
	}

	// Load performance config
	if perf, err := s.db.GetPerformanceConfig(); err == nil {
		data.Perf = perf
	} else {
		data.Perf = database.DefaultPerformanceConfig()
	}
	if backupCfg, err := s.db.GetBackupConfig(); err == nil {
		data.Backup = backupCfg
	} else {
		data.Backup = database.DefaultBackupConfig()
	}

	s.render(w, r, "settings.html", "Settings", "settings", data)
}

func maskNotifConfig(nc *database.NotificationConfig, channel string) *notifDisplay {
	nd := &notifDisplay{Enabled: nc.Enabled, Fields: make(map[string]string)}

	var raw map[string]string
	if err := json.Unmarshal([]byte(nc.Config), &raw); err != nil {
		return nd
	}

	for k, v := range raw {
		if v == "" {
			nd.Fields[k] = ""
			continue
		}
		// Mask sensitive fields
		switch {
		case strings.Contains(k, "token"), strings.Contains(k, "password"), strings.Contains(k, "pass"):
			if len(v) > 4 {
				nd.Fields[k] = strings.Repeat("*", len(v)-4) + v[len(v)-4:]
			} else {
				nd.Fields[k] = "****"
			}
		default:
			nd.Fields[k] = v
		}
	}
	return nd
}

// handleAlertRuleCreate handles POST /api/alerts/rules
func (s *Server) handleAlertRuleCreate(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	threshold, err := strconv.ParseFloat(r.FormValue("threshold"), 64)
	if err != nil || threshold < 0 {
		http.Error(w, "Invalid threshold", http.StatusBadRequest)
		return
	}

	cooldown, err := strconv.Atoi(r.FormValue("cooldown"))
	if err != nil || cooldown < 0 {
		cooldown = 15
	}

	metric := r.FormValue("metric")
	if !isValidMetric(metric) {
		http.Error(w, "Invalid metric", http.StatusBadRequest)
		return
	}

	operator := r.FormValue("operator")
	if !isValidOperator(operator) {
		http.Error(w, "Invalid operator", http.StatusBadRequest)
		return
	}

	severity := r.FormValue("severity")
	if !isValidSeverity(severity) {
		http.Error(w, "Invalid severity", http.StatusBadRequest)
		return
	}

	ac := &database.AlertConfig{
		Name:            r.FormValue("name"),
		Metric:          metric,
		Operator:        operator,
		Threshold:       threshold,
		Severity:        severity,
		Enabled:         true,
		CooldownMinutes: cooldown,
	}

	if ac.Name == "" {
		ac.Name = fmt.Sprintf("%s %s %.0f", strings.ToUpper(metric), operator, threshold)
	}

	if err := s.db.CreateAlertConfig(ac); err != nil {
		log.Printf("settings: failed to create rule: %v", err)
		http.Error(w, "Failed to create rule", http.StatusInternalServerError)
		return
	}

	s.renderRulesTable(w, r)
}

// handleAlertRuleToggle handles POST /api/alerts/rules/{id}/toggle
func (s *Server) handleAlertRuleToggle(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.db.ToggleAlertConfig(id); err != nil {
		log.Printf("settings: failed to toggle rule: %v", err)
		http.Error(w, "Failed to toggle rule", http.StatusInternalServerError)
		return
	}

	s.renderRulesTable(w, r)
}

// handleAlertRuleDelete handles DELETE /api/alerts/rules/{id}
func (s *Server) handleAlertRuleDelete(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.db.DeleteAlertConfig(id); err != nil {
		log.Printf("settings: failed to delete rule: %v", err)
		http.Error(w, "Failed to delete rule", http.StatusInternalServerError)
		return
	}

	s.renderRulesTable(w, r)
}

// handleNotificationSave handles POST /api/notifications/{channel}
func (s *Server) handleNotificationSave(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	channel := r.PathValue("channel")
	if channel != "telegram" && channel != "email" {
		http.Error(w, "Invalid channel", http.StatusBadRequest)
		return
	}

	// Load existing config to avoid overwriting sensitive fields with empty values
	existing, err := s.db.GetNotificationConfig(channel)
	config := make(map[string]string)
	if err == nil && existing != nil {
		json.Unmarshal([]byte(existing.Config), &config)
	}

	switch channel {
	case "telegram":
		if v := r.FormValue("bot_token"); v != "" {
			config["bot_token"] = v
		}
		if v := r.FormValue("chat_id"); v != "" {
			config["chat_id"] = v
		}
	case "email":
		config["smtp_host"] = r.FormValue("smtp_host")
		config["smtp_port"] = r.FormValue("smtp_port")
		config["smtp_user"] = r.FormValue("smtp_user")
		if v := r.FormValue("smtp_password"); v != "" {
			config["smtp_password"] = v
		}
		config["from"] = r.FormValue("from")
		config["to"] = r.FormValue("to")
	}

	configJSON, _ := json.Marshal(config)

	nc := &database.NotificationConfig{
		Channel: channel,
		Enabled: r.FormValue("enabled") == "on",
		Config:  string(configJSON),
	}

	if err := s.db.UpsertNotificationConfig(nc); err != nil {
		log.Printf("settings: failed to save %s config: %v", channel, err)
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast": {"message": "Failed to save %s config", "type": "error"}}`, channel))
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast": {"message": "%s notifications updated", "type": "success"}}`, strings.Title(channel)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<div class="text-sm text-green-400 py-2 flex items-center gap-2">` +
		`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>` +
		`Saved successfully</div>`))
}

// handleNotificationTest handles POST /api/notifications/{channel}/test
func (s *Server) handleNotificationTest(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	channel := r.PathValue("channel")
	nc, err := s.db.GetNotificationConfig(channel)
	if err != nil || nc == nil {
		http.Error(w, "Config not found. Save settings first.", http.StatusNotFound)
		return
	}

	var cfg map[string]string
	json.Unmarshal([]byte(nc.Config), &cfg)

	alert := &database.Alert{
		Severity:  "info",
		Message:   "This is a test notification from Ultron-AP.",
		Source:    "test",
		CreatedAt: time.Now(),
	}

	var testErr error
	switch channel {
	case "telegram":
		sender := notify.NewTelegramSender(cfg["bot_token"], cfg["chat_id"])
		testErr = sender.Send(alert)
	case "email":
		sender := notify.NewEmailSender(
			cfg["smtp_host"], cfg["smtp_port"],
			cfg["smtp_user"], cfg["smtp_password"],
			cfg["from"], cfg["to"],
		)
		testErr = sender.Send(alert)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if testErr != nil {
		fmt.Fprintf(w, `<div class="text-sm text-danger py-2">Test failed: %s</div>`, html.EscapeString(testErr.Error()))
	} else {
		w.Write([]byte(`<div class="text-sm text-green-400 py-2">Test message sent! Check your client.</div>`))
	}
}

func (s *Server) renderRulesTable(w http.ResponseWriter, r *http.Request) {
	rules, _ := s.db.ListAlertConfigs()

	csrfToken := ""
	if cookie, err := r.Cookie("session"); err == nil {
		session, _ := s.db.GetSession(cookie.Value)
		if session != nil {
			csrfToken = session.CSRFToken
		}
	}

	tmpl, ok := s.tmplCache["partials/alert-rules-table.html"]
	if !ok {
		log.Printf("settings: alert-rules-table not in cache")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Rules":     rules,
		"CSRFToken": csrfToken,
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "alert-rules-table", data); err != nil {
		log.Printf("settings: render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

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

	log.Printf("settings: saving performance config: SSE=%ds, Disk=%dm, Docker=%ds, Systemd=%ds, Backup=%dh",
		cfg.SSEIntervalSec, cfg.DiskIntervalMin, cfg.DockerIntervalSec, cfg.SystemdIntervalSec, cfg.BackupIntervalHours)

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

func (s *Server) validateCSRF(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	session, err := s.db.GetSession(cookie.Value)
	if err != nil || session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	csrfToken := r.FormValue("csrf_token")
	if csrfToken == "" {
		csrfToken = r.Header.Get("X-CSRF-Token")
	}

	if csrfToken != session.CSRFToken {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}

	// Defense-in-depth: enforce same-origin when Origin/Referer are provided.
	if !isSameOriginRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func isSameOriginRequest(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if origin == "" && referer == "" {
		return true
	}

	expectedScheme := "http"
	if isHTTPSRequest(r) {
		expectedScheme = "https"
	}
	expectedHost := r.Host

	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		if !strings.EqualFold(u.Scheme, expectedScheme) || !strings.EqualFold(u.Host, expectedHost) {
			return false
		}
	}

	if referer != "" {
		u, err := url.Parse(referer)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		if !strings.EqualFold(u.Scheme, expectedScheme) || !strings.EqualFold(u.Host, expectedHost) {
			return false
		}
	}

	return true
}

func isValidMetric(m string) bool {
	switch m {
	case "cpu", "ram", "disk", "temp":
		return true
	}
	return false
}

func isValidOperator(op string) bool {
	switch op {
	case ">", "<", ">=", "<=", "==":
		return true
	}
	return false
}

func isValidSeverity(s string) bool {
	switch s {
	case "critical", "warning", "info":
		return true
	}
	return false
}
