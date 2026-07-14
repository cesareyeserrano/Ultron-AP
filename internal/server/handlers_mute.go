package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// applyMuteFromForm handles the optional mute_hours field on the Telegram save
// (FR-079). It is folded into the existing POST /api/notifications/telegram
// rather than given its own endpoint because the chip-preset lives inside that
// form and submits with it.
//
// Returns false when it has already written an error response.
func (s *Server) applyMuteFromForm(w http.ResponseWriter, r *http.Request) bool {
	raw := r.FormValue("mute_hours")
	if raw == "" {
		// A save without the field changes no mute state — the same
		// partial-save posture the other settings forms take (BG-061).
		return true
	}

	hours, err := strconv.Atoi(raw)
	if err != nil {
		http.Error(w, database.ErrInvalidMuteHours.Error(), http.StatusBadRequest)
		return false
	}

	expiresAt, err := s.db.SetNotificationMute(hours, time.Now())
	if err != nil {
		if errors.Is(err, database.ErrInvalidMuteHours) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return false
		}
		log.Printf("settings: failed to set mute: %v", err)
		http.Error(w, "Failed to mute Telegram alerts", http.StatusInternalServerError)
		return false
	}

	s.auditLog(r, "settings", "mute", "telegram",
		fmt.Sprintf("%dh — until %s", hours, expiresAt.Format(time.RFC3339)), true)
	return true
}

// handleMuteClear handles POST /api/notifications/mute/clear (FR-079).
// Cancels an open mute window; the next fired alert reaches Telegram again.
func (s *Server) handleMuteClear(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	if err := s.db.ClearNotificationMute(); err != nil {
		log.Printf("settings: failed to clear mute: %v", err)
		setToast(w, "Failed to unmute Telegram alerts", "error")
		http.Error(w, "Failed to clear mute", http.StatusInternalServerError)
		return
	}

	s.auditLog(r, "settings", "mute_clear", "telegram", "", true)

	setToast(w, "Telegram alerts unmuted", "success")

	// Swap the mute row back to its chip-preset state so the section reflects
	// reality without a page reload.
	html := s.renderPartial("partials/settings-mute.html", muteDisplay{})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
