package pironman

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
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
		RGBColor      string          `json:"rgb_color"`
		RGBBrightness int             `json:"rgb_brightness"`
		RGBStyle      string          `json:"rgb_style"`
		RGBSpeed      int             `json:"rgb_speed"`
		RGBEnable     json.RawMessage `json:"rgb_enable"`
		FanMode       int             `json:"gpio_fan_mode"`
		FanLED        string          `json:"gpio_fan_led"`
		OLEDEnable    json.RawMessage `json:"oled_enable"`
		OLEDRotation  int             `json:"oled_rotation"`
		OLEDSleep     int             `json:"oled_sleep_timeout"`
	} `json:"system"`
}

// parseBoolOrString handles cases where pironman5 returns a bool (true/false)
// or a string ("on"/"off").
func parseBoolOrString(raw json.RawMessage) bool {
	s := string(raw)
	// If it's a JSON boolean
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	// If it's a JSON string, it will be wrapped in quotes
	s = strings.Trim(s, "\"")
	return s == "on" || s == "1" || s == "true"
}

func ParseConfigJSON(out []byte) (*Config, error) {
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
		RGBEnable:     parseBoolOrString(raw.System.RGBEnable),
		FanMode:       raw.System.FanMode,
		FanLED:        raw.System.FanLED,
		OLEDEnable:    parseBoolOrString(raw.System.OLEDEnable),
		OLEDRotation:  raw.System.OLEDRotation,
		OLEDSleep:     raw.System.OLEDSleep,
	}, nil
}

// ReadConfig gets pironman settings through the privileged helper.
func ReadConfig() (*Config, error) {
	cli := privileged.NewClientFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := cli.PironmanRead(ctx)
	if err != nil {
		return nil, fmt.Errorf("pironman5 -c via helper: %w", err)
	}
	return ParseConfigJSON([]byte(raw))
}

// BuildApplyArgs builds arguments for `pironman5 restart`.
func BuildApplyArgs(cfg Config) []string {
	rgbEnable := "off"
	if cfg.RGBEnable {
		rgbEnable = "on"
	}
	oledEnable := "off"
	if cfg.OLEDEnable {
		oledEnable = "on"
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
	return args
}

// ApplyConfig applies settings through the privileged helper.
func ApplyConfig(cfg Config) error {
	cli := privileged.NewClientFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := cli.PironmanApply(ctx, privileged.PironmanConfig{
		RGBColor:      cfg.RGBColor,
		RGBBrightness: cfg.RGBBrightness,
		RGBStyle:      cfg.RGBStyle,
		RGBSpeed:      cfg.RGBSpeed,
		RGBEnable:     cfg.RGBEnable,
		FanMode:       cfg.FanMode,
		FanLED:        cfg.FanLED,
		OLEDEnable:    cfg.OLEDEnable,
		OLEDRotation:  cfg.OLEDRotation,
		OLEDSleep:     cfg.OLEDSleep,
	})
	if err != nil {
		return fmt.Errorf("pironman5 apply via helper: %w", err)
	}
	return nil
}

// Available reports whether pironman5 is installed on the system.
func Available() bool {
	cli := privileged.NewClientFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return cli.Ping(ctx) == nil
}
