package network

import (
	"math"
	"testing"
)

// TestTC_NM_003e_UptimePctSevenDayWindow verifies the 7-day uptime
// calculation: samples_up=60345 / total_samples=60480 → 99.7768% ±0.1.
//
// @aitri-tc TC-NM-003e
func TestTC_NM_003e_UptimePctSevenDayWindow(t *testing.T) {
	t.Parallel()
	got := UptimePct(60345, 60480)
	want := 99.7768
	if math.Abs(got-want) > 0.1 {
		t.Errorf("UptimePct(60345, 60480) = %.4f, want within ±0.1 of %.4f", got, want)
	}
}

// TestTC_NM_003e_UptimePctEdgeCases pins the well-defined edges so a future
// refactor cannot silently introduce NaN or out-of-range values.
//
// @aitri-tc TC-NM-003e
func TestTC_NM_003e_UptimePctEdgeCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name              string
		up, total         int64
		want              float64
	}{
		{"all up", 100, 100, 100.0},
		{"all down", 0, 100, 0.0},
		{"zero total", 5, 0, 0.0},
		{"negative up clamps to 0", -3, 100, 0.0},
		{"up > total clamps to 100", 200, 100, 100.0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := UptimePct(tc.up, tc.total); got != tc.want {
				t.Errorf("UptimePct(%d, %d) = %.4f, want %.4f", tc.up, tc.total, got, tc.want)
			}
		})
	}
}
