package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

type settingsData struct {
	Rules          []database.AlertConfig
	NetworkTargets []string
	Telegram       *notifDisplay
	Email          *notifDisplay
	Perf           database.PerformanceConfig
	Backup         database.BackupConfig
	Flash          string
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

	data := settingsData{Rules: rules, NetworkTargets: s.alertRuleTargets()}

	if tg, err := s.db.GetNotificationConfig("telegram"); err == nil && tg != nil {
		data.Telegram = maskNotifConfig(tg, "telegram")
	}
	if em, err := s.db.GetNotificationConfig("email"); err == nil && em != nil {
		data.Email = maskNotifConfig(em, "email")
	}

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

func isValidMetric(m string) bool {
	switch m {
	case "cpu", "ram", "disk", "temp", "latency", "loss", "dns_failure_rate", "wan_outage", "public_ip_change":
		return true
	}
	return false
}

func isThresholdMetric(m string) bool {
	switch m {
	case "cpu", "ram", "disk", "temp", "latency", "loss", "dns_failure_rate":
		return true
	}
	return false
}

func isTargetMetric(m string) bool {
	return m == "latency" || m == "loss"
}

func (s *Server) alertRuleTargets() []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	add("gateway")
	add("8.8.8.8")
	add("1.1.1.1")
	if s.gateway != nil {
		for _, snap := range s.gateway.Snapshots() {
			add(snap.Label)
			add(snap.Target)
		}
	}
	return out
}

func (s *Server) isValidAlertTarget(target string) bool {
	for _, allowed := range s.alertRuleTargets() {
		if target == allowed {
			return true
		}
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
