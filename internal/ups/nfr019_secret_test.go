package ups

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// TC-UPS-046f (NFR-019): the NUT credential never appears in the logs nor in a
// rendered snapshot. A poll runs with a password configured; the secret must not
// leak into any log line or any Snapshot field.
func TestTC_UPS_046f_SecretNotLeaked(t *testing.T) {
	// @aitri-tc TC-UPS-046f
	const secret = "s3cr3t-nut-pass"

	f := startFakeUpsd(t, map[string]string{
		"ups.status":      "OL",
		"battery.voltage": "27.0",
	})

	// Capture everything the module logs during the poll cycle. Render the args
	// too — a leak like logf("auth failed for %s", pass) hides in the args, not
	// the format string.
	var logs []string
	orig := logger
	logger = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	defer func() { logger = orig }()

	cfg := Config{Addr: f.addr, UPSName: "powest", User: "ultron", Pass: secret, BattLowV: 21, BattHighV: 27.4}
	p := NewPoller(NewClient(cfg), cfg)
	snap := p.PollNow(context.Background())

	for _, l := range logs {
		if strings.Contains(l, secret) {
			t.Errorf("secret leaked into a log line: %q", l)
		}
	}
	// The secret must not be carried in any rendered snapshot string.
	rendered := strings.Join([]string{
		snap.RawStatus, snap.Beeper, snap.StateLabel(), snap.BeeperLabel(),
		snap.LoadStr(), snap.InputStr(), snap.BatteryStr(), snap.BattPctStr(),
		snap.DelayShutStr(), snap.DelayStartStr(), snap.CutoffStr(), snap.Reason(),
	}, " ")
	if strings.Contains(rendered, secret) {
		t.Errorf("secret leaked into a rendered snapshot field: %q", rendered)
	}
}
