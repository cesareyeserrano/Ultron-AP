package server

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify"
)

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
