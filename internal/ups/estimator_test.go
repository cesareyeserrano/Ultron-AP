package ups

import "testing"

// TC-UPS-010h (FR-018): battery estimate midpoint is ~50%.
func TestTC_UPS_010h_EstimateMidpoint(t *testing.T) {
	// @aitri-tc TC-UPS-010h
	got := EstimateBatteryPct(24.2, 21.0, 27.4)
	if got < 48 || got > 52 {
		t.Fatalf("EstimateBatteryPct(24.2) = %.2f, want ~50 (48–52)", got)
	}
}

// TC-UPS-011e (FR-018): estimate clamps at the boundaries.
func TestTC_UPS_011e_EstimateClamps(t *testing.T) {
	// @aitri-tc TC-UPS-011e
	if got := EstimateBatteryPct(20.5, 21.0, 27.4); got != 0 {
		t.Errorf("below-low clamp: got %.2f, want 0", got)
	}
	if got := EstimateBatteryPct(28.0, 21.0, 27.4); got != 100 {
		t.Errorf("above-high clamp: got %.2f, want 100", got)
	}
	// Exact bounds.
	if got := EstimateBatteryPct(21.0, 21.0, 27.4); got != 0 {
		t.Errorf("at-low: got %.2f, want 0", got)
	}
	if got := EstimateBatteryPct(27.4, 21.0, 27.4); got != 100 {
		t.Errorf("at-high: got %.2f, want 100", got)
	}
}

// TC-UPS-012f (FR-018): the estimate is always tagged 'estimado' and never
// emitted as a device battery.charge. buildSnapshot must set BattPctEst (the
// estimate) and must not populate any charge field from raw vars lacking one.
func TestTC_UPS_012f_EstimateAlwaysTagged(t *testing.T) {
	// @aitri-tc TC-UPS-012f
	p := NewPoller(nil, Config{BattLowV: 21.0, BattHighV: 27.4})
	// Raw vars deliberately contain NO battery.charge (this UPS never publishes it).
	snap := p.buildSnapshot(map[string]string{
		"ups.status":      "OL",
		"battery.voltage": "24.2",
	})
	if snap.BattPctEst == nil {
		t.Fatal("BattPctEst is nil, want an estimate")
	}
	if !snap.Estimated() {
		t.Fatal("Estimated() = false, want true (estimate must be tagged)")
	}
	// The presentation layer always renders it with the estimado label; the
	// value is derived from voltage, not from a device charge reading.
	if got := snap.BattPctStr(); got == "—" {
		t.Fatal("BattPctStr = '—', want a rendered estimate")
	}
}
