package aggregate

import (
	"testing"
	"time"
)

// TestTC_NM_005e_PickResolutionDayFor30dWindow verifies that a 30-day window
// resolves to the 'day' resolution even when raw data is present in the store.
//
// @aitri-tc TC-NM-005e
func TestTC_NM_005e_PickResolutionDayFor30dWindow(t *testing.T) {
	t.Parallel()
	got := PickResolution(30 * 24 * time.Hour)
	if got != ResolutionDay {
		t.Fatalf("30d window → %q, want %q", got, ResolutionDay)
	}
}

// TestTC_NM_005e_PickResolutionBands locks the resolution boundaries so
// regressions that subtly shift bands fail loudly.
//
// @aitri-tc TC-NM-005e
func TestTC_NM_005e_PickResolutionBands(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		window time.Duration
		want   Resolution
	}{
		{"1h boundary", time.Hour, ResolutionRaw},
		{"6h boundary", 6 * time.Hour, ResolutionMinute},
		{"7d boundary", 7 * 24 * time.Hour, ResolutionHour},
		{"30d window", 30 * 24 * time.Hour, ResolutionDay},
		{"365d window", 365 * 24 * time.Hour, ResolutionDay},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := PickResolution(tc.window); got != tc.want {
				t.Errorf("PickResolution(%s) = %q, want %q", tc.window, got, tc.want)
			}
		})
	}
}
