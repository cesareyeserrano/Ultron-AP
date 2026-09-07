// Tests for the two network-retention settings.
//
// The theme running through them: an invalid value must never reach the
// database layer, and must never stop the panel from booting.
//
// @aitri-trace FR-096 FR-100 NFR-102 NFR-106 NFR-110
package config

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/ups"
)

// captureLog redirects the package's warning seam into a buffer for the test.
func captureLog(t *testing.T) *strings.Builder {
	t.Helper()
	var sb strings.Builder
	prev := logf
	logf = func(format string, args ...any) {
		sb.WriteString(strings.TrimSpace(fmt.Sprintf(format, args...)) + "\n")
	}
	t.Cleanup(func() { logf = prev })
	return &sb
}

// upsRetentionDaysForTest reads the UPS window through its own loader, which is
// the point: the two settings must be independent, so the assertion has to go
// through the real UPS path rather than a copy of its default.
func upsRetentionDaysForTest(t *testing.T) int {
	t.Helper()
	return ups.Load().RetentionDays
}

// loadForTest calls Load with the minimum environment it needs to succeed.
func loadForTest(t *testing.T) *Config {
	t.Helper()
	t.Setenv("ULTRON_DB_PATH", t.TempDir()+"/t.db")
	cfg, err := Load()
	require.NoError(t, err, "Load must not fail on a retention/interval value")
	return cfg
}

// @aitri-tc TC-NSR-001h — no variable means a 30-day window (AC-096-001).
func TestTC_NSR_001h(t *testing.T) {
	t.Setenv("ULTRON_NET_RETENTION_DAYS", "")
	assert.Equal(t, 30, loadForTest(t).NetRetentionDays)
}

// @aitri-tc TC-NSR-002e — a valid value is taken verbatim (AC-096-002).
func TestTC_NSR_002e(t *testing.T) {
	t.Setenv("ULTRON_NET_RETENTION_DAYS", "7")
	assert.Equal(t, 7, loadForTest(t).NetRetentionDays)
}

// @aitri-tc TC-NSR-003f — a non-numeric value warns and defaults, and above
// all does NOT abort the boot (AC-096-003).
func TestTC_NSR_003f(t *testing.T) {
	logged := captureLog(t)
	t.Setenv("ULTRON_NET_RETENTION_DAYS", "abc")

	cfg := loadForTest(t) // require.NoError inside: a typo must not stop the panel
	assert.Equal(t, 30, cfg.NetRetentionDays)
	assert.Contains(t, logged.String(), "ULTRON_NET_RETENTION_DAYS")
	assert.Contains(t, logged.String(), "abc", "the warning must name the value received")
}

// @aitri-tc TC-NSR-004f — 0 and negatives cannot wipe the history
// (AC-096-004).
//
// A window of 0 means "delete everything older than now"; a negative one puts
// the cutoff in the future, which also deletes everything. Both must be
// neutralised before the value leaves Load.
func TestTC_NSR_004f(t *testing.T) {
	for _, v := range []string{"0", "-5"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("ULTRON_NET_RETENTION_DAYS", v)
			assert.Equal(t, 30, loadForTest(t).NetRetentionDays,
				"%q must not survive as a retention window", v)
		})
	}
}

// @aitri-tc TC-NSR-040h — no variable means the previous 5s (AC-100-001).
func TestTC_NSR_040h(t *testing.T) {
	t.Setenv("ULTRON_NET_INTERVAL_SECONDS", "")
	assert.Equal(t, 5*time.Second, loadForTest(t).NetInterval)
}

// @aitri-tc TC-NSR-041e — a valid interval is taken verbatim (AC-100-002).
func TestTC_NSR_041e(t *testing.T) {
	t.Setenv("ULTRON_NET_INTERVAL_SECONDS", "15")
	assert.Equal(t, 15*time.Second, loadForTest(t).NetInterval)
}

// @aitri-tc TC-NSR-042f — 0, negative and non-numeric all warn and fall back
// to 5s (AC-100-003).
func TestTC_NSR_042f(t *testing.T) {
	for _, v := range []string{"0", "-1", "xyz"} {
		t.Run(v, func(t *testing.T) {
			logged := captureLog(t)
			t.Setenv("ULTRON_NET_INTERVAL_SECONDS", v)

			assert.Equal(t, 5*time.Second, loadForTest(t).NetInterval)
			assert.Contains(t, logged.String(), "ULTRON_NET_INTERVAL_SECONDS",
				"%q must produce a warning that names the variable", v)
		})
	}
}

// @aitri-tc TC-NSR-043e — the probe receives the CONFIGURED interval, proving
// the value travels from config rather than sitting in a literal (AC-100-004).
func TestTC_NSR_043e(t *testing.T) {
	t.Setenv("ULTRON_NET_INTERVAL_SECONDS", "15")
	cfg := loadForTest(t)

	p := gatewayprobe.New(cfg.NetInterval, nil, nil)
	assert.Equal(t, 15*time.Second, p.Interval(),
		"the probe must run at the configured interval, not a hardcoded one")
}

// @aitri-tc TC-NSR-100h — unconfigured, the probe is built at 5s (NFR-110).
func TestTC_NSR_100h(t *testing.T) {
	t.Setenv("ULTRON_NET_INTERVAL_SECONDS", "")
	cfg := loadForTest(t)

	p := gatewayprobe.New(cfg.NetInterval, nil, nil)
	assert.Equal(t, 5*time.Second, p.Interval(),
		"an unchanged environment must sample exactly as before this feature")
}

// @aitri-tc TC-NSR-101e — the constructor keeps its own 5s fallback as a
// second net, independent of config (AC-100-004).
func TestTC_NSR_101e(t *testing.T) {
	p := gatewayprobe.New(0, nil, nil)
	assert.Equal(t, 5*time.Second, p.Interval())
}

// @aitri-tc TC-NSR-102f — a corrupt environment does not change observable
// sampling (AC-100-003).
func TestTC_NSR_102f(t *testing.T) {
	logged := captureLog(t)
	t.Setenv("ULTRON_NET_INTERVAL_SECONDS", "not-a-number")
	cfg := loadForTest(t)

	p := gatewayprobe.New(cfg.NetInterval, nil, nil)
	assert.Equal(t, 5*time.Second, p.Interval())
	assert.Contains(t, logged.String(), "ULTRON_NET_INTERVAL_SECONDS")
}

// @aitri-tc TC-NSR-060h — UPS and network retention are independent
// (NFR-106).
func TestTC_NSR_060h(t *testing.T) {
	t.Setenv("ULTRON_UPS_RETENTION_DAYS", "45")
	t.Setenv("ULTRON_NET_RETENTION_DAYS", "7")

	assert.Equal(t, 7, loadForTest(t).NetRetentionDays)
	assert.Equal(t, 45, upsRetentionDaysForTest(t), "the UPS window must be its own")
}

// @aitri-tc TC-NSR-061f — the network variable does not leak into the UPS
// window (NFR-106).
func TestTC_NSR_061f(t *testing.T) {
	t.Setenv("ULTRON_NET_RETENTION_DAYS", "1")
	t.Setenv("ULTRON_UPS_RETENTION_DAYS", "")

	assert.Equal(t, 1, loadForTest(t).NetRetentionDays)
	assert.NotEqual(t, 1, upsRetentionDaysForTest(t),
		"UPS must keep its own default, not inherit the network window")
}
