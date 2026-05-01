package alerts

import (
	"testing"
	"time"
)

// TestTC_NM_007e_FlappingDoesNotFire asserts that a 30-min stream
// alternating 200 ms and 50 ms every 60 s does NOT trigger a 5-min
// sustained latency rule (>150 ms). Flapping must be silent.
//
// @aitri-tc TC-NM-007e
func TestTC_NM_007e_FlappingDoesNotFire(t *testing.T) {
	t.Parallel()
	const period = time.Minute
	t0 := time.Unix(0, 0)
	samples := make([]SamplePoint, 0, 30)
	for i := 0; i < 30; i++ {
		v := 200.0
		if i%2 == 1 {
			v = 50.0
		}
		samples = append(samples, SamplePoint{TS: t0.Add(time.Duration(i) * period), Value: v})
	}
	if IsSustainedAbove(samples, 150.0, 5*time.Minute) {
		t.Fatal("flapping 200/50 every 60s must not fire a 5-min-sustained 150 ms rule")
	}
}

// TestTC_NM_007e_SustainedAboveFires is the positive control: a clean run of
// 12 samples at 200 ms × 30 s spacing exceeds the 5-min sustained window
// and must fire — confirming the negative test above is not vacuous.
//
// @aitri-tc TC-NM-007e
func TestTC_NM_007e_SustainedAboveFires(t *testing.T) {
	t.Parallel()
	t0 := time.Unix(0, 0)
	samples := make([]SamplePoint, 0, 12)
	for i := 0; i < 12; i++ {
		samples = append(samples, SamplePoint{TS: t0.Add(time.Duration(i*30) * time.Second), Value: 200.0})
	}
	if !IsSustainedAbove(samples, 150.0, 5*time.Minute) {
		t.Fatal("12 samples × 30 s above 150 must fire a 5-min-sustained rule")
	}
}
