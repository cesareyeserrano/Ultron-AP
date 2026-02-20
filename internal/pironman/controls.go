package pironman

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// RGBStyles lists the valid pironman5 LED animation styles.
var RGBStyles = []string{
	"solid", "breathing", "flow", "flow_reverse",
	"rainbow", "rainbow_reverse", "hue_cycle",
}

// FanModes maps mode number to a human-readable label.
var FanModes = map[int]string{
	0: "Always off",
	1: "Always on",
	2: "Temperature auto",
	3: "Follow CPU temp",
	4: "Custom",
}

// Config holds the Pironman5 runtime configuration.
type Config struct {
	// RGB
	RGBColor      string // hex without #, e.g. "9500ff"
	RGBBrightness int    // 0-100
	RGBStyle      string // solid|breathing|flow|...
	RGBSpeed      int    // 0-100
	RGBEnable     bool
	// Fan
	FanMode int    // 0-4
	FanLED  string // on|off|follow
	// OLED
	OLEDEnable   bool
	OLEDRotation int // 0|180
	OLEDSleep    int // seconds
}

// rawConfig mirrors the JSON returned by `pironman5 -c`.
type rawConfig struct {
	System struct {
		RGBColor      string `json:"rgb_color"`
		RGBBrightness int    `json:"rgb_brightness"`
		RGBStyle      string `json:"rgb_style"`
		RGBSpeed      int    `json:"rgb_speed"`
		RGBEnable     bool   `json:"rgb_enable"`
		FanMode       int    `json:"gpio_fan_mode"`
		FanLED        string `json:"gpio_fan_led"`
		OLEDEnable    bool   `json:"oled_enable"`
		OLEDRotation  int    `json:"oled_rotation"`
		OLEDSleep     int    `json:"oled_sleep_timeout"`
	} `json:"system"`
}

// ReadConfig runs `sudo -n pironman5 -c` and returns the parsed configuration.
func ReadConfig() (*Config, error) {
	out, err := exec.Command("sudo", "-n", "pironman5", "-c").Output()
	if err != nil {
		return nil, fmt.Errorf("pironman5 -c: %w", err)
	}

	var raw rawConfig
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("pironman5 config parse: %w", err)
	}

	color := strings.TrimPrefix(raw.System.RGBColor, "#")

	return &Config{
		RGBColor:      color,
		RGBBrightness: raw.System.RGBBrightness,
		RGBStyle:      raw.System.RGBStyle,
		RGBSpeed:      raw.System.RGBSpeed,
		RGBEnable:     raw.System.RGBEnable,
		FanMode:       raw.System.FanMode,
		FanLED:        raw.System.FanLED,
		OLEDEnable:    raw.System.OLEDEnable,
		OLEDRotation:  raw.System.OLEDRotation,
		OLEDSleep:     raw.System.OLEDSleep,
	}, nil
}

// ApplyConfig applies settings by running `pironman5 restart <flags>`.
func ApplyConfig(cfg Config) error {
	rgbEnable := "false"
	if cfg.RGBEnable {
		rgbEnable = "true"
	}
	oledEnable := "false"
	if cfg.OLEDEnable {
		oledEnable = "true"
	}

	args := []string{
		"restart",
		"-rc", cfg.RGBColor,
		"-rb", strconv.Itoa(cfg.RGBBrightness),
		"-rs", cfg.RGBStyle,
		"-rp", strconv.Itoa(cfg.RGBSpeed),
		"-re", rgbEnable,
		"-gm", strconv.Itoa(cfg.FanMode),
		"-fl", cfg.FanLED,
		"-oe", oledEnable,
		"-or", strconv.Itoa(cfg.OLEDRotation),
		"-os", strconv.Itoa(cfg.OLEDSleep),
	}

	sudoArgs := append([]string{"-n", "pironman5"}, args...)
	out, err := exec.Command("sudo", sudoArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pironman5 restart: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Available reports whether pironman5 is installed on the system.
func Available() bool {
	_, err := exec.LookPath("pironman5")
	return err == nil
}
