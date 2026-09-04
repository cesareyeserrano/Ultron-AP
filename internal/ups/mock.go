// Module:       internal/ups
// Purpose:      In-process mock UPS client for local dev rendering + tests (NFR-022, RS-5).
// Dependencies: standard library only.
//
// SAFETY: the mock is only ever used when ULTRON_UPS_MOCK is set, which is
// absent from the production systemd unit. With it unset, NewClient always
// returns the real read-only TCP client (verified by TC-UPS-054f).
package ups

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
)

// errMockUnreachable simulates a UPS/NUT that does not answer.
var errMockUnreachable = errors.New("ups: mock unreachable")

// mockClient serves canned UPS variables so the real card, history and alerts
// can be exercised locally with no physical UPS and no deploy.
//
// The mode (from ULTRON_UPS_MOCK) selects behaviour:
//   - "OL","OB","LB","RB","BYPASS","OFF","ALARM": a fixed status
//   - "unreachable": List always errors (drives the "Sin datos" state)
//   - "1" / "cycle": advances OL → OB → LB → unreachable on each poll
type mockClient struct {
	mode string
	tick atomic.Uint64
}

// cycleStates is the on-demand state walk for ULTRON_UPS_MOCK=1 (NFR-022:
// drive the card through every state locally).
var cycleStates = []string{"OL", "OB", "LB", "unreachable"}

// newMockClient builds a mock client from the ULTRON_UPS_MOCK mode string.
func newMockClient(mode string) *mockClient {
	return &mockClient{mode: strings.TrimSpace(mode)}
}

// List returns canned variables for the configured mode, or an error for the
// unreachable mode. The input frequency wobbles slightly per poll so the
// grid-health chart is visibly alive during local validation.
func (m *mockClient) List(ctx context.Context) (map[string]string, error) {
	n := m.tick.Add(1) - 1
	status := m.mode
	if m.mode == "1" || strings.EqualFold(m.mode, "cycle") {
		status = cycleStates[int(n)%len(cycleStates)]
	}
	if strings.EqualFold(status, "unreachable") {
		return nil, errMockUnreachable
	}
	vars := mockVars(status)
	vars["input.frequency"] = fmt.Sprintf("%.1f", 59.9+0.1*float64(n%3))
	return vars, nil
}

// Close is a no-op for the mock.
func (m *mockClient) Close() error { return nil }

// mockVars returns a representative variable map for a given raw ups.status.
// Battery voltage tracks the status so the estimated % looks plausible per state.
func mockVars(status string) map[string]string {
	batt := "27.1"
	switch strings.ToUpper(status) {
	case "OB":
		batt = "25.4"
	case "LB":
		batt = "21.4"
	}
	return map[string]string{
		"ups.status":         strings.ToUpper(status),
		"ups.load":           "2",
		"input.voltage":      "122.0",
		"battery.voltage":    batt,
		"ups.beeper.status":  "enabled",
		"ups.delay.shutdown": "30",
		"ups.delay.start":    "60",
		"ups.type":           "offline",
	}
}
