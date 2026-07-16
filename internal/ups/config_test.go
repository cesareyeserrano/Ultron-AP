package ups

import (
	"testing"
	"time"
)

// TC-UPS-031h (FR-024): env override replaces the default poll interval.
func TestTC_UPS_031h_EnvOverridePollInterval(t *testing.T) {
	// @aitri-tc TC-UPS-031h
	t.Setenv("ULTRON_UPS_POLL_SECONDS", "30")
	cfg := Load()
	if cfg.PollInterval != 30*time.Second {
		t.Fatalf("PollInterval = %s, want 30s", cfg.PollInterval)
	}
}

// TC-UPS-032e (FR-024): module inert when ULTRON_UPS_ENABLED is unset.
func TestTC_UPS_032e_DisabledByDefault(t *testing.T) {
	// @aitri-tc TC-UPS-032e
	// Ensure the flag is unset for this test.
	t.Setenv("ULTRON_UPS_ENABLED", "")
	cfg := Load()
	if cfg.Enabled {
		t.Fatalf("Enabled = true with ULTRON_UPS_ENABLED unset, want false (module must be inert)")
	}
	if cfg.Mock != "" {
		t.Fatalf("Mock = %q with nothing set, want empty (no mock in default config)", cfg.Mock)
	}
}

// TC-UPS-033f (FR-024): invalid config values are rejected, defaults used, no crash.
func TestTC_UPS_033f_InvalidConfigRejected(t *testing.T) {
	// @aitri-tc TC-UPS-033f
	// Capture warnings emitted by the loader.
	var warnings []string
	orig := logger
	logger = func(format string, args ...any) { warnings = append(warnings, format) }
	defer func() { logger = orig }()

	t.Setenv("ULTRON_UPS_BATT_LOW_V", "30") // low >= high is invalid
	t.Setenv("ULTRON_UPS_BATT_HIGH_V", "27.4")
	t.Setenv("ULTRON_UPS_POLL_SECONDS", "abc") // non-numeric

	cfg := Load() // must not panic

	if cfg.BattLowV != defaultBattLowV || cfg.BattHighV != defaultBattHighV {
		t.Fatalf("battery range = %.2f/%.2f, want defaults %.1f/%.1f",
			cfg.BattLowV, cfg.BattHighV, defaultBattLowV, defaultBattHighV)
	}
	if cfg.PollInterval != defaultPollInterval {
		t.Fatalf("PollInterval = %s, want default %s", cfg.PollInterval, defaultPollInterval)
	}
	if len(warnings) < 2 {
		t.Fatalf("expected a warning for each invalid value, got %d: %v", len(warnings), warnings)
	}
}
