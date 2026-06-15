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
	"github.com/cesareyeserrano/ultron-ap/internal/notify/cause"
)

// buildTestNotificationEvent returns a synthetic CPU-fire Event that
// exercises every resource-surface renderer branch (subject, threshold-aware
// metric line, elapsed-since-breach, trend hint, probable-cause line,
// deep-link footer) per FR-026. Both fields are clearly marked "TEST — " so
// a real operator never mistakes the preview for a production fire.
//
// @aitri-trace FR-026
func buildTestNotificationEvent() *notify.Event {
	value := 92.4
	cfgID := int64(0)
	now := time.Now().UTC()
	alert := &database.Alert{
		Severity:  "critical",
		Message:   "TEST — synthetic CPU fire from settings page",
		Source:    "cpu",
		Value:     &value,
		ConfigID:  &cfgID,
		CreatedAt: now,
	}
	rule := &database.AlertConfig{
		ID:        0,
		Name:      "TEST — High CPU",
		Metric:    "cpu",
		Operator:  ">",
		Threshold: 80.0,
		Severity:  "critical",
	}
	return &notify.Event{
		Alert:        alert,
		Rule:         rule,
		Kind:         notify.EventFire,
		Surface:      notify.SurfaceResource,
		FirstFiredAt: now.Add(-90 * time.Second),
	}
}

// _ keeps cause imported for future surface-specific test message variants.
var _ = cause.SourceProc

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
		if uerr := json.Unmarshal([]byte(existing.Config), &config); uerr != nil {
			// Stored config is corrupt — fail loudly instead of starting from an
			// empty map and silently overwriting previously-saved secrets.
			log.Printf("notification config for %q is corrupt, refusing to overwrite: %v", channel, uerr)
			http.Error(w, "Stored notification config is corrupt; not overwriting", http.StatusInternalServerError)
			return
		}
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
	if err := json.Unmarshal([]byte(nc.Config), &cfg); err != nil {
		log.Printf("notifications: malformed config for channel %q: %v", channel, err)
		http.Error(w, "Notification config is malformed — re-save settings.", http.StatusInternalServerError)
		return
	}

	// Build a synthetic CPU-fire event so the preview exercises every
	// renderer branch a real fire would (FR-026).
	//
	// @aitri-trace FR-026
	evt := buildTestNotificationEvent()

	var testErr error
	switch channel {
	case "telegram":
		sender := notify.NewTelegramSender(cfg["bot_token"], cfg["chat_id"])
		testErr = sender.Notify(r.Context(), evt)
	case "email":
		sender := notify.NewEmailSender(
			cfg["smtp_host"], cfg["smtp_port"],
			cfg["smtp_user"], cfg["smtp_password"],
			cfg["from"], cfg["to"],
		)
		testErr = sender.Notify(r.Context(), evt)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if testErr != nil {
		fmt.Fprintf(w, `<div class="text-sm text-danger py-2">Test failed: %s</div>`, html.EscapeString(testErr.Error()))
	} else {
		w.Write([]byte(`<div class="text-sm text-green-400 py-2">Test message sent! Check your client.</div>`))
	}
}
