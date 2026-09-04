// Module:       internal/ups
// Purpose:      UPS module configuration from environment (FR-024, RS-1/RS-3).
// Dependencies: standard library only.
package ups

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default configuration values (documented in .env.example).
const (
	defaultAddr               = "127.0.0.1:3493"
	defaultUPSName            = "powest"
	defaultPollInterval       = 10 * time.Second
	defaultUnreachableTimeout = 2 * time.Minute
	defaultBattLowV           = 21.0
	defaultBattHighV          = 27.4
	defaultRetentionDays      = 30
	defaultInputVLow          = 100.0
	defaultInputVHigh         = 140.0
	defaultDebounce           = 30 * time.Second
)

// Config holds the UPS module settings. All values come from ULTRON_UPS_* env
// vars; invalid values are rejected with a logged warning and the default is
// used — the process never crashes on bad config (FR-024 AC-024-002).
type Config struct {
	Enabled            bool          // ULTRON_UPS_ENABLED — gates the whole module (RS-3)
	Addr               string        // NUT server address (localhost only in prod)
	UPSName            string        // NUT device name
	User               string        // ULTRON_NUT_USER (read-only credential, RS-1)
	Pass               string        // ULTRON_NUT_PASS (never logged)
	PollInterval       time.Duration // ULTRON_UPS_POLL_SECONDS
	UnreachableTimeout time.Duration // ULTRON_UPS_UNREACHABLE_SECONDS
	BattLowV           float64       // ULTRON_UPS_BATT_LOW_V  (estimate 0%, cutoff)
	BattHighV          float64       // ULTRON_UPS_BATT_HIGH_V (estimate 100%)
	RetentionDays      int           // ULTRON_UPS_RETENTION_DAYS
	InputVLow          float64       // ULTRON_UPS_INPUT_LOW_V  (alert threshold)
	InputVHigh         float64       // ULTRON_UPS_INPUT_HIGH_V (alert threshold)
	Debounce           time.Duration // ULTRON_UPS_DEBOUNCE_SECONDS
	Mock               string        // ULTRON_UPS_MOCK — dev only; "" in prod (NFR-022)
}

// logger is overridable in tests to capture warnings; defaults to the standard
// logger (matching the rest of the codebase).
var logger = log.Printf

// Load reads the UPS configuration from the environment, applying defaults and
// rejecting invalid values with a warning.
//
// Returns a Config that is always usable — never an error — so a
// misconfiguration degrades to defaults instead of failing boot (FR-024).
func Load() Config {
	cfg := Config{
		Enabled:            envBool("ULTRON_UPS_ENABLED", false),
		Addr:               envStr("ULTRON_UPS_ADDR", defaultAddr),
		UPSName:            envStr("ULTRON_UPS_NAME", defaultUPSName),
		User:               os.Getenv("ULTRON_NUT_USER"),
		Pass:               os.Getenv("ULTRON_NUT_PASS"),
		PollInterval:       envDurationSeconds("ULTRON_UPS_POLL_SECONDS", defaultPollInterval),
		UnreachableTimeout: envDurationSeconds("ULTRON_UPS_UNREACHABLE_SECONDS", defaultUnreachableTimeout),
		BattLowV:           envFloat("ULTRON_UPS_BATT_LOW_V", defaultBattLowV),
		BattHighV:          envFloat("ULTRON_UPS_BATT_HIGH_V", defaultBattHighV),
		RetentionDays:      envInt("ULTRON_UPS_RETENTION_DAYS", defaultRetentionDays),
		InputVLow:          envFloat("ULTRON_UPS_INPUT_LOW_V", defaultInputVLow),
		InputVHigh:         envFloat("ULTRON_UPS_INPUT_HIGH_V", defaultInputVHigh),
		Debounce:           envDurationSeconds("ULTRON_UPS_DEBOUNCE_SECONDS", defaultDebounce),
		Mock:               strings.TrimSpace(os.Getenv("ULTRON_UPS_MOCK")),
	}

	// Cross-field validation: the battery range must be strictly increasing,
	// otherwise the estimate is meaningless. Reject and fall back to defaults.
	if cfg.BattLowV >= cfg.BattHighV {
		logger("ups: invalid battery range low=%.2f >= high=%.2f, using defaults %.1f/%.1f",
			cfg.BattLowV, cfg.BattHighV, defaultBattLowV, defaultBattHighV)
		cfg.BattLowV = defaultBattLowV
		cfg.BattHighV = defaultBattHighV
	}
	return cfg
}

func envStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		logger("ups: invalid %s=%q, using default %d", key, v, def)
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		logger("ups: invalid %s=%q, using default %.2f", key, v, def)
		return def
	}
	return f
}

func envDurationSeconds(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		logger("ups: invalid %s=%q, using default %s", key, v, def)
		return def
	}
	return time.Duration(n) * time.Second
}
