package server

import (
	"errors"
	"log"
	"net/http"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// handleHardwareSave handles POST /api/settings/hardware (FR-082 / FR-083).
//
// It persists the fan mode and the OLED configuration. It drives NO hardware:
// no GPIO, no I2C, no daemon connection, no goroutine. That is the owner's
// explicit constraint (a previous hardware-control attempt cost the Pi
// significant CPU and memory) and is declared in the feature's no-go zone —
// the stored values are what a future actuator would read.
func (s *Server) handleHardwareSave(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	cfg := database.HardwareConfig{
		FanMode: r.FormValue("fan_mode"),
		// Standard HTML checkbox semantics: an omitted checkbox is "off".
		OLEDEnabled: r.FormValue("oled_enabled") == "on",
		OLEDMetric:  r.FormValue("oled_metric"),
	}

	if err := s.db.SaveHardwareConfig(cfg); err != nil {
		// An invalid enum is the client's error, not the server's: reject it
		// with the message the settings form maps to the offending field, and
		// leave the stored value untouched (AC-082-003 / AC-083-003).
		if errors.Is(err, database.ErrInvalidFanMode) || errors.Is(err, database.ErrInvalidOLEDMetric) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("settings: failed to save hardware config: %v", err)
		setToast(w, "Failed to save hardware settings", "error")
		http.Error(w, "Failed to save hardware settings", http.StatusInternalServerError)
		return
	}

	setToast(w, "Hardware settings updated", "success")
	writeSettingsSaved(w)
}

// writeSettingsSaved renders the shared "Saved successfully" status fragment
// the settings forms swap into their status target.
func writeSettingsSaved(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<div class="text-sm text-green-400 py-2 flex items-center gap-2">` +
		`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>` +
		`Saved successfully</div>`))
}
