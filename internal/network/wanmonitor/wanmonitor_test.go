package wanmonitor

import (
	"testing"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
)

func okSnap(label string, ts time.Time) gatewayprobe.Snapshot {
	return gatewayprobe.Snapshot{
		Label:     label,
		Status:    gatewayprobe.StatusOK,
		Target:    "1.1.1.1",
		RTTMs:     10,
		LastProbe: ts,
	}
}

func failSnap(label string, ts time.Time, status gatewayprobe.Status) gatewayprobe.Snapshot {
	return gatewayprobe.Snapshot{
		Label:     label,
		Status:    status,
		Target:    "1.1.1.1",
		LastProbe: ts,
	}
}

func TestNew_DefaultsThresholdTo3(t *testing.T) {
	m := New("public", "gateway", 0, nil)
	if m.threshold != 3 {
		t.Errorf("threshold = %d, want 3", m.threshold)
	}
}

func TestObserve_FirstOKTransitionsUnknownToUp(t *testing.T) {
	var seen []Event
	m := New("public", "gateway", 3, func(e Event) { seen = append(seen, e) })
	m.Observe(okSnap("gateway", time.Now()))
	m.Observe(okSnap("public", time.Now()))
	if got := m.Snapshot().State; got != StateUp {
		t.Errorf("state = %q, want %q", got, StateUp)
	}
	if len(seen) != 1 || seen[0].Kind != "wan_up" {
		t.Errorf("expected one wan_up event, got %+v", seen)
	}
}

func TestObserve_3ConsecutivePublicFailsTransitionsToDown(t *testing.T) {
	var seen []Event
	m := New("public", "gateway", 3, func(e Event) { seen = append(seen, e) })
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	// Establish UP first.
	m.Observe(okSnap("gateway", now))
	m.Observe(okSnap("public", now))
	// Now 3 failures on public, gateway still ok.
	for i := 1; i <= 3; i++ {
		m.Observe(failSnap("public", now.Add(time.Duration(i)*time.Second), gatewayprobe.StatusTimeout))
	}
	if got := m.Snapshot().State; got != StateDown {
		t.Errorf("state = %q, want %q", got, StateDown)
	}
	// Two events: wan_up then wan_down.
	if len(seen) != 2 || seen[1].Kind != "wan_down" {
		t.Errorf("expected wan_up then wan_down, got %+v", seen)
	}
}

func TestObserve_DoesNotTransitionDownIfGatewayAlsoFailing(t *testing.T) {
	var seen []Event
	m := New("public", "gateway", 3, func(e Event) { seen = append(seen, e) })
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	m.Observe(okSnap("gateway", now))
	m.Observe(okSnap("public", now))
	// Gateway goes down.
	m.Observe(failSnap("gateway", now.Add(time.Second), gatewayprobe.StatusTimeout))
	// 3 public failures while gateway is also down — should NOT trigger wan_down.
	for i := 2; i <= 4; i++ {
		m.Observe(failSnap("public", now.Add(time.Duration(i)*time.Second), gatewayprobe.StatusTimeout))
	}
	if got := m.Snapshot().State; got != StateUp {
		t.Errorf("state = %q, want still %q (LAN outage, not WAN)", got, StateUp)
	}
}

func TestObserve_OneOKAfterDownTransitionsBackToUp(t *testing.T) {
	var seen []Event
	m := New("public", "gateway", 3, func(e Event) { seen = append(seen, e) })
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	// UP → DOWN
	m.Observe(okSnap("gateway", now))
	m.Observe(okSnap("public", now))
	for i := 1; i <= 3; i++ {
		m.Observe(failSnap("public", now.Add(time.Duration(i)*time.Second), gatewayprobe.StatusTimeout))
	}
	// One OK on public should recover.
	m.Observe(okSnap("public", now.Add(10*time.Second)))
	if got := m.Snapshot().State; got != StateUp {
		t.Errorf("state = %q, want %q", got, StateUp)
	}
	// Last event should be wan_up.
	last := seen[len(seen)-1]
	if last.Kind != "wan_up" {
		t.Errorf("last event = %+v, want wan_up", last)
	}
}

func TestObserve_IgnoresUnrelatedTargets(t *testing.T) {
	m := New("public", "gateway", 3, nil)
	// Some other target probing — must not affect state.
	for i := 0; i < 5; i++ {
		m.Observe(failSnap("dns-public", time.Now(), gatewayprobe.StatusTimeout))
	}
	if got := m.Snapshot().State; got != StateUnknown {
		t.Errorf("state = %q, want unknown", got)
	}
}

func TestObserve_DoesNotDoubleEmitConsecutiveFailures(t *testing.T) {
	var seen []Event
	m := New("public", "gateway", 3, func(e Event) { seen = append(seen, e) })
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	m.Observe(okSnap("gateway", now))
	m.Observe(okSnap("public", now))
	// Trigger DOWN.
	for i := 1; i <= 3; i++ {
		m.Observe(failSnap("public", now.Add(time.Duration(i)*time.Second), gatewayprobe.StatusTimeout))
	}
	// More failures while DOWN — must not re-emit wan_down.
	for i := 4; i <= 8; i++ {
		m.Observe(failSnap("public", now.Add(time.Duration(i)*time.Second), gatewayprobe.StatusTimeout))
	}
	wanDownCount := 0
	for _, e := range seen {
		if e.Kind == "wan_down" {
			wanDownCount++
		}
	}
	if wanDownCount != 1 {
		t.Errorf("wan_down emitted %d times, want 1", wanDownCount)
	}
}
