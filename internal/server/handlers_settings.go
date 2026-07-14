package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

type settingsData struct {
	Rules          []database.AlertConfig
	NetworkTargets []string
	Telegram       *notifDisplay
	Email          *notifDisplay
	Perf           database.PerformanceConfig
	Backup         database.BackupConfig
	Hardware       database.HardwareConfig
	Mute           muteDisplay
	Digest         digestDisplay
	Flash          string
}

// muteDisplay is what the Telegram section renders for FR-079: either the
// chip-preset ("mute for 1h/4h/24h") or the muted state with its countdown.
type muteDisplay struct {
	Muted     bool
	Hours     int    // what the admin picked — 1, 4 or 24
	Remaining string // human-readable, e.g. "3h 59m"
}

// digestDisplay is the FR-080 daily-digest row inside the Email section.
type digestDisplay struct {
	Enabled bool
	Hour    int // 0–23
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

	// FR-082/FR-083: one singleton read. Ultron stores these; it drives no
	// hardware — no goroutine, no GPIO, nothing that could cost the Pi.
	if hw, err := s.db.GetHardwareConfig(); err == nil {
		data.Hardware = hw
	} else {
		log.Printf("settings: hardware config unavailable, using defaults: %v", err)
		data.Hardware = database.DefaultHardwareConfig()
	}

	data.Mute = s.muteDisplay()
	data.Digest = digestFromNotifConfig(data.Email)

	s.render(w, r, "settings.html", "Settings", "settings", data)
}

// muteDisplay reads the FR-079 mute window for rendering. A read error is
// rendered as "not muted" — the same fail-open posture the send path takes, so
// the UI never claims a mute the dispatcher would not honour (NFR-090).
func (s *Server) muteDisplay() muteDisplay {
	now := time.Now()
	expiresAt, muted, err := s.db.NotificationMuteUntil(now)
	if err != nil {
		log.Printf("settings: mute state unreadable: %v", err)
		return muteDisplay{}
	}
	if !muted {
		return muteDisplay{}
	}
	hours, err := s.db.MuteHours()
	if err != nil {
		log.Printf("settings: mute hours unreadable: %v", err)
	}
	return muteDisplay{
		Muted:     true,
		Hours:     hours,
		Remaining: formatUptime(expiresAt.Sub(now)),
	}
}

// digestFromNotifConfig pulls the two digest keys out of the (already masked)
// email config. They are not secrets, so masking leaves them intact.
func digestFromNotifConfig(email *notifDisplay) digestDisplay {
	d := digestDisplay{Hour: 8} // the default the scheduler assumes
	if email == nil {
		return d
	}
	if strings.EqualFold(email.Fields["digest_enabled"], "true") {
		d.Enabled = true
	}
	if h, err := strconv.Atoi(email.Fields["digest_hour"]); err == nil && h >= 0 && h <= 23 {
		d.Hour = h
	}
	return d
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
