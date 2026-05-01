package breaker

import "testing"

// TestTC_NM_008h_BreakerEngagesAt6_5PctCPU asserts that a 5-min mean of 6.5%
// crosses the 6% threshold and engages the breaker with reason=cpu_high.
//
// @aitri-tc TC-NM-008h
func TestTC_NM_008h_BreakerEngagesAt6_5PctCPU(t *testing.T) {
	t.Parallel()
	s := Decide(6.5)
	if !s.Active {
		t.Errorf("Decide(6.5).Active = false, want true")
	}
	if s.Reason != "cpu_high" {
		t.Errorf("Decide(6.5).Reason = %q, want cpu_high", s.Reason)
	}
	if s.CPUPct != 6.5 {
		t.Errorf("Decide(6.5).CPUPct = %v, want 6.5", s.CPUPct)
	}
}

// TestTC_NM_008h_BreakerStaysOpenAtOrBelowThreshold is the negative control:
// strict greater-than, so 6.0 itself does not engage, and 5.0 surely does
// not.
//
// @aitri-tc TC-NM-008h
func TestTC_NM_008h_BreakerStaysOpenAtOrBelowThreshold(t *testing.T) {
	t.Parallel()
	for _, cpu := range []float64{0.0, 5.0, 6.0} {
		s := Decide(cpu)
		if s.Active {
			t.Errorf("Decide(%v).Active = true, want false (threshold is %v, strict)", cpu, CPUHighThresholdPct)
		}
	}
}
