package server

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// handleAlertRuleCreate handles POST /api/alerts/rules
func (s *Server) handleAlertRuleCreate(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	metric := r.FormValue("metric")
	if !isValidMetric(metric) {
		http.Error(w, "Invalid metric", http.StatusBadRequest)
		return
	}

	severity := r.FormValue("severity")
	if !isValidSeverity(severity) {
		http.Error(w, "Invalid severity", http.StatusBadRequest)
		return
	}

	cooldown, err := strconv.Atoi(r.FormValue("cooldown"))
	if err != nil || cooldown < 0 {
		cooldown = 15
	}

	sustained, err := strconv.Atoi(r.FormValue("sustained_duration"))
	if r.FormValue("sustained_duration") == "" {
		sustained = 0
		err = nil
	}
	if err != nil || sustained < 0 || sustained > 3600 {
		http.Error(w, "Invalid sustained duration", http.StatusBadRequest)
		return
	}

	operator := r.FormValue("operator")
	threshold := 0.0
	var target *string

	if isThresholdMetric(metric) {
		if !isValidOperator(operator) {
			http.Error(w, "Invalid operator", http.StatusBadRequest)
			return
		}
		threshold, err = strconv.ParseFloat(r.FormValue("threshold"), 64)
		if err != nil || threshold < 0 {
			http.Error(w, "Invalid threshold", http.StatusBadRequest)
			return
		}
	} else {
		operator = "=="
		sustained = 0
	}

	if isTargetMetric(metric) {
		t := strings.TrimSpace(r.FormValue("target"))
		if t == "" || !s.isValidAlertTarget(t) {
			http.Error(w, "Invalid target", http.StatusBadRequest)
			return
		}
		target = &t
	}

	switch metric {
	case "wan_outage":
		if severity != "critical" {
			http.Error(w, "WAN outage alerts use critical severity", http.StatusBadRequest)
			return
		}
	case "public_ip_change":
		if severity != "info" {
			http.Error(w, "Public IP change alerts use info severity", http.StatusBadRequest)
			return
		}
		if r.FormValue("cooldown") == "" {
			cooldown = 60
		}
	}

	ac := &database.AlertConfig{
		Name:              r.FormValue("name"),
		Metric:            metric,
		Operator:          operator,
		Threshold:         threshold,
		Target:            target,
		SustainedDuration: sustained,
		Severity:          severity,
		Enabled:           true,
		CooldownMinutes:   cooldown,
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
