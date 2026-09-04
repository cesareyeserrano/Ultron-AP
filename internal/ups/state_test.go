package ups

import "testing"

// TC-UPS-051e (NFR-021): parser+state map is table-driven over all documented flags.
func TestTC_UPS_051e_StatusMapTable(t *testing.T) {
	// @aitri-tc TC-UPS-051e
	cases := []struct {
		raw  string
		want State
	}{
		{"OL", StateOnline},
		{"OB", StateOnBattery},
		{"LB", StateLowBatt},
		{"OL CHRG", StateCharging},
		{"RB", StateReplace},
		{"OFF", StateOff},
		{"BYPASS", StateBypass},
		{"ALARM", StateAlarm},
		{"OB LB", StateLowBatt}, // compound: LB dominates
	}
	for _, c := range cases {
		if got := ParseStatus(c.raw); got != c.want {
			t.Errorf("ParseStatus(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

// TC-UPS-052f (NFR-021): the LB-precedence invariant a mutation would break.
// Inverting the LB precedence in ParseStatus makes this assertion fail, so a
// broken build exits non-zero and blocks the pipeline.
func TestTC_UPS_052f_LBPrecedenceInvariant(t *testing.T) {
	// @aitri-tc TC-UPS-052f
	if got := ParseStatus("OB LB"); got != StateLowBatt {
		t.Fatalf("compound 'OB LB' must map to %q (LB dominates), got %q", StateLowBatt, got)
	}
	if got := ParseStatus("LB"); got != StateLowBatt {
		t.Fatalf("'LB' must map to %q, got %q", StateLowBatt, got)
	}
	// A plain OL must NOT be misclassified as low battery.
	if got := ParseStatus("OL"); got == StateLowBatt {
		t.Fatalf("'OL' must not map to low battery")
	}
}
