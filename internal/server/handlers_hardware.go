package server

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/pironman"
)

type hardwareViewData struct {
	Available bool
	CSRFToken string
	Config    *pironman.Config
	RGBStyles []string
	FanModes  map[int]string
}

func (s *Server) handleHardwarePage(w http.ResponseWriter, r *http.Request) {
	data := hardwareViewData{
		Available: pironman.Available(),
		CSRFToken: s.sessionCSRFToken(r),
		RGBStyles: pironman.RGBStyles,
		FanModes:  pironman.FanModes,
	}
	if data.Available {
		cfg, err := pironman.ReadConfig()
		if err != nil {
			log.Printf("hardware: read config: %v", err)
			// Don't mark as unavailable, just don't provide the config
			// The template should handle nil Config by showing an error message
		} else {
			data.Config = cfg
		}
	}
	s.render(w, r, "hardware.html", "Hardware", "hardware", data)
}

func (s *Server) handleHardwareApply(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	if !pironman.Available() {
		http.Error(w, "pironman5 not available", http.StatusServiceUnavailable)
		return
	}

	cfg := parseHardwareConfig(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := pironman.ApplyConfig(cfg); err != nil {
		log.Printf("hardware: apply config: %v", err)
		toastType := "error"
		if strings.Contains(strings.ToLower(err.Error()), "apply busy") {
			toastType = "warning"
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast": {"message": "Failed: %s", "type": "%s"}}`, html.EscapeString(err.Error()), toastType))
		fmt.Fprintf(w,
			`<div class="rounded-lg bg-danger/10 border border-danger/30 p-3 mb-4 text-sm text-danger">%s</div>`,
			html.EscapeString(err.Error()),
		)
		fmt.Fprint(w, s.renderHardwareContent(&cfg, s.sessionCSRFToken(r)))
		return
	}

	w.Header().Set("HX-Trigger", `{"showToast": {"message": "Hardware settings applied", "type": "success"}}`)
	// Re-read config from pironman5 to confirm applied values.
	applied, err := pironman.ReadConfig()
	if err != nil {
		applied = &cfg
	}

	fmt.Fprint(w, s.renderHardwareContent(applied, s.sessionCSRFToken(r)))
}

func (s *Server) renderHardwareContent(cfg *pironman.Config, csrfToken string) string {
	return s.renderPartial("partials/hardware-form.html", hardwareViewData{
		Available: true,
		CSRFToken: csrfToken,
		Config:    cfg,
		RGBStyles: pironman.RGBStyles,
		FanModes:  pironman.FanModes,
	})
}

func parseHardwareConfig(r *http.Request) pironman.Config {
	return pironman.Config{
		RGBColor:      sanitizeHex(r.FormValue("rgb_color")),
		RGBBrightness: clampInt(formInt(r, "rgb_brightness"), 0, 100),
		RGBStyle:      sanitizeStyle(r.FormValue("rgb_style")),
		RGBSpeed:      clampInt(formInt(r, "rgb_speed"), 0, 100),
		RGBEnable:     r.FormValue("rgb_enable") == "on",
		FanMode:       clampInt(formInt(r, "fan_mode"), 0, 4),
		FanLED:        sanitizeFanLED(r.FormValue("fan_led")),
		OLEDEnable:    r.FormValue("oled_enable") == "on",
		OLEDRotation:  sanitizeRotation(formInt(r, "oled_rotation")),
		OLEDSleep:     clampInt(formInt(r, "oled_sleep"), 0, 3600),
	}
}

// --- Input sanitization helpers ---

func formInt(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.FormValue(key))
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// sanitizeHex accepts a color value from an HTML color input (#rrggbb) and
// returns just the 6 hex digits, falling back to "000000".
func sanitizeHex(s string) string {
	// color input always sends "#rrggbb"
	if len(s) == 7 && s[0] == '#' {
		s = s[1:]
	}
	if len(s) != 6 {
		return "000000"
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return "000000"
		}
	}
	return s
}

func sanitizeStyle(s string) string {
	for _, v := range pironman.RGBStyles {
		if s == v {
			return s
		}
	}
	return "solid"
}

func sanitizeFanLED(s string) string {
	switch s {
	case "on", "off", "follow":
		return s
	}
	return "follow"
}

func sanitizeRotation(v int) int {
	if v == 180 {
		return 180
	}
	return 0
}
