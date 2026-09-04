package ups

import (
	"strings"
	"testing"
	"time"
)

// alertCfg is a config with thresholds set so unrelated rules stay quiet.
func alertCfg() Config {
	return Config{
		BattLowV: 21.0, BattHighV: 27.4,
		InputVLow: 100.0, InputVHigh: 140.0,
		Debounce:           30 * time.Second,
		UnreachableTimeout: 2 * time.Minute,
	}
}

// collect returns an alerter whose sink appends to the returned slice pointer.
func collect(cfg Config) (*Alerter, *[]Alert) {
	var got []Alert
	a := NewAlerter(cfg, func(al Alert) { got = append(got, al) })
	fixed := time.Unix(1_000_000, 0)
	a.now = func() time.Time { return fixed }
	return a, &got
}

func snap(state State, reachable bool, batt, input float64, ts time.Time) Snapshot {
	b, in := batt, input
	return Snapshot{State: state, RawStatus: string(state), Reachable: reachable, BatteryV: &b, InputV: &in, LastGood: ts}
}

func fires(got []Alert) (n int) {
	for _, a := range got {
		if !a.Resolve {
			n++
		}
	}
	return
}

// TC-UPS-019h (FR-021): a transition to OB fires a Warning (→ Telegram via sink).
func TestTC_UPS_019h_OBFiresWarning(t *testing.T) {
	// @aitri-tc TC-UPS-019h
	a, got := collect(alertCfg())
	t0 := time.Unix(5000, 0)
	a.Observe(snap(StateOnline, true, 27.1, 122, t0))
	a.Observe(snap(StateOnBattery, true, 25.4, 122, t0))
	if fires(*got) != 1 {
		t.Fatalf("expected 1 fire, got %d: %+v", fires(*got), *got)
	}
	last := (*got)[len(*got)-1]
	if last.Severity != SevWarning || last.Resolve {
		t.Fatalf("expected a Warning fire, got %+v", last)
	}
	if last.Source != "ups" {
		t.Fatalf("source = %q, want ups", last.Source)
	}
}

// TC-UPS-020e (FR-021): the return to OL emits an Info resolve with the outage duration.
func TestTC_UPS_020e_OLResolveWithDuration(t *testing.T) {
	// @aitri-tc TC-UPS-020e
	a, got := collect(alertCfg())
	t0 := time.Unix(5000, 0)
	a.Observe(snap(StateOnline, true, 27.1, 122, t0))
	a.Observe(snap(StateOnBattery, true, 25.4, 122, t0))                   // outage starts at t0
	a.Observe(snap(StateOnline, true, 27.0, 122, t0.Add(300*time.Second))) // restored 5 min later

	var resolve *Alert
	for i := range *got {
		if (*got)[i].Resolve {
			resolve = &(*got)[i]
		}
	}
	if resolve == nil {
		t.Fatalf("no resolve emitted: %+v", *got)
	}
	if resolve.Severity != SevInfo {
		t.Fatalf("resolve severity = %q, want info", resolve.Severity)
	}
	if !strings.Contains(resolve.Message, "5 min") {
		t.Fatalf("resolve message %q must include the outage duration '5 min'", resolve.Message)
	}
}

// TC-UPS-021f (FR-021): a single out-of-range voltage sample within the debounce
// window does not alert.
func TestTC_UPS_021f_VoltageDebounce(t *testing.T) {
	// @aitri-tc TC-UPS-021f
	a, got := collect(alertCfg()) // clock is fixed → debounce window never elapses
	t0 := time.Unix(5000, 0)
	a.Observe(snap(StateOnline, true, 27.1, 120, t0)) // in range
	a.Observe(snap(StateOnline, true, 27.1, 145, t0)) // one out-of-range sample
	a.Observe(snap(StateOnline, true, 27.1, 121, t0)) // recovered
	if len(*got) != 0 {
		t.Fatalf("expected no alerts for a single transient dip, got %+v", *got)
	}
}

// TC-UPS-022f (FR-021): an LB status fires a Critical alert.
func TestTC_UPS_022f_LBFiresCritical(t *testing.T) {
	// @aitri-tc TC-UPS-022f
	a, got := collect(alertCfg())
	t0 := time.Unix(5000, 0)
	a.Observe(snap(StateOnBattery, true, 24.0, 122, t0)) // warning
	a.Observe(snap(StateLowBatt, true, 21.4, 122, t0))   // escalation → critical
	var crit *Alert
	for i := range *got {
		if (*got)[i].Severity == SevCritical {
			crit = &(*got)[i]
		}
	}
	if crit == nil {
		t.Fatalf("expected a Critical alert on LB, got %+v", *got)
	}
}

// TC-UPS-056f (FR-021, AC-021-004): a persistent RB (replace-battery) condition
// fires at most one Warning per day.
func TestTC_UPS_056f_RBOncePerDay(t *testing.T) {
	// @aitri-tc TC-UPS-056f
	a, got := collect(alertCfg())
	base := time.Unix(1_000_000, 0)
	clock := base
	a.now = func() time.Time { return clock }

	rb := func() Snapshot { return snap(StateReplace, true, 27.0, 122, clock) }
	a.Observe(rb()) // first RB → one warning
	a.Observe(rb()) // same day → no repeat
	clock = base.Add(2 * time.Hour)
	a.Observe(rb()) // still same day → no repeat

	rbWarnings := 0
	for _, al := range *got {
		if al.Severity == SevWarning && strings.Contains(al.Message, "Reemplazar batería") {
			rbWarnings++
		}
	}
	if rbWarnings != 1 {
		t.Fatalf("expected exactly 1 RB warning within a day, got %d", rbWarnings)
	}

	// A day later, it may warn again.
	clock = base.Add(25 * time.Hour)
	a.Observe(rb())
	rbWarnings = 0
	for _, al := range *got {
		if al.Severity == SevWarning && strings.Contains(al.Message, "Reemplazar batería") {
			rbWarnings++
		}
	}
	if rbWarnings != 2 {
		t.Fatalf("expected a second RB warning after 24h, got %d total", rbWarnings)
	}
}

// TC-UPS-023f (FR-021): unreachable fires exactly one Warning; recovery once.
func TestTC_UPS_023f_UnreachableSingleWarning(t *testing.T) {
	// @aitri-tc TC-UPS-023f
	a, got := collect(alertCfg())
	un := Snapshot{State: StateUnreachable, Reachable: false}
	a.Observe(un)
	a.Observe(un)
	a.Observe(un) // sustained — still one warning
	a.Observe(snap(StateOnline, true, 27.1, 122, time.Unix(6000, 0)))

	warnings, resolves := 0, 0
	for _, al := range *got {
		if al.Resolve {
			resolves++
		} else if al.Severity == SevWarning && strings.Contains(al.Message, "sin comunicación") {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("expected exactly 1 'sin comunicación' warning, got %d: %+v", warnings, *got)
	}
	if resolves != 1 {
		t.Fatalf("expected exactly 1 recovery resolve, got %d: %+v", resolves, *got)
	}
}
