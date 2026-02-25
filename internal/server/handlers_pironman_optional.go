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

type pironmanPageData struct {
	Enabled    bool
	CSRFToken  string
	Status     integrationStatusView
	Config     *pironman.Config
	RGBStyles  []string
	FanModes   map[int]string
	ApplyState string
	ErrorMsg   string
}

func (s *Server) handlePironmanPage(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.FeaturePironman {
		http.NotFound(w, r)
		return
	}

	data := pironmanPageData{
		Enabled:    true,
		CSRFToken:  s.sessionCSRFToken(r),
		Status:     s.currentPironmanIntegrationStatus(r.Context()),
		RGBStyles:  pironman.RGBStyles,
		FanModes:   pironman.FanModes,
		ApplyState: "idle",
	}

	if data.Status.State == "available" {
		cfg, err := pironman.ReadConfig()
		if err != nil {
			data.Status = classifyPironmanRead("", err)
			data.ApplyState = "failed"
			data.ErrorMsg = err.Error()
		} else {
			data.Config = cfg
		}
	}

	s.render(w, r, "integration-pironman.html", "Pironman Integration", "settings", data)
}

func (s *Server) handlePironmanApply(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.FeaturePironman {
		http.NotFound(w, r)
		return
	}
	if !s.validateCSRF(w, r) {
		return
	}

	cfg := parsePironmanConfig(r)
	data := pironmanPageData{
		Enabled:    true,
		CSRFToken:  s.sessionCSRFToken(r),
		Status:     s.currentPironmanIntegrationStatus(r.Context()),
		RGBStyles:  pironman.RGBStyles,
		FanModes:   pironman.FanModes,
		Config:     &cfg,
		ApplyState: "applying",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if data.Status.State != "available" {
		data.ApplyState = "failed"
		data.ErrorMsg = "integration unavailable"
		w.Header().Set("HX-Trigger", `{"showToast": {"message": "Pironman integration unavailable", "type": "error"}}`)
		fmt.Fprint(w, s.renderPartial("partials/pironman-form.html", data))
		return
	}

	if err := pironman.ApplyConfig(cfg); err != nil {
		log.Printf("pironman optional apply: %v", err)
		data.ApplyState = "failed"
		data.ErrorMsg = err.Error()
		toastType := "error"
		if strings.Contains(strings.ToLower(err.Error()), "busy") {
			toastType = "warning"
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast": {"message": "Failed: %s", "type": "%s"}}`, html.EscapeString(err.Error()), toastType))
		fmt.Fprint(w, s.renderPartial("partials/pironman-form.html", data))
		return
	}

	applied, err := pironman.ReadConfig()
	if err == nil {
		data.Config = applied
	}
	data.ApplyState = "applied"
	data.ErrorMsg = ""
	w.Header().Set("HX-Trigger", `{"showToast": {"message": "Pironman settings applied", "type": "success"}}`)
	fmt.Fprint(w, s.renderPartial("partials/pironman-form.html", data))
}

func parsePironmanConfig(r *http.Request) pironman.Config {
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

func sanitizeHex(s string) string {
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
