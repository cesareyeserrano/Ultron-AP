package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

type settingsData struct {
	Rules    []database.AlertConfig
	Telegram *notifDisplay
	Email    *notifDisplay
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

	s.renderRulesTable(w)
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

	s.renderRulesTable(w)
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

	s.renderRulesTable(w)
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

	config := make(map[string]string)

	switch channel {
	case "telegram":
		config["bot_token"] = r.FormValue("bot_token")
		config["chat_id"] = r.FormValue("chat_id")
	case "email":
		config["smtp_host"] = r.FormValue("smtp_host")
		config["smtp_port"] = r.FormValue("smtp_port")
		config["smtp_user"] = r.FormValue("smtp_user")
		config["smtp_password"] = r.FormValue("smtp_password")
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
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<div class="text-sm text-green-400 py-2">Saved successfully</div>`))
}

func (s *Server) renderRulesTable(w http.ResponseWriter) {
	rules, _ := s.db.ListAlertConfigs()

	tmpl, err := template.ParseFS(s.templates, "templates/partials/alert-rules-table.html")
	if err != nil {
		log.Printf("settings: parse error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "alert-rules-table", rules); err != nil {
		log.Printf("settings: render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
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
