package network

// UptimePct returns the percentage of "up" samples over the total. The result
// is always in [0,100]. totalSamples ≤ 0 returns 0 (no observation window);
// samplesUp is clamped into [0,totalSamples] to keep the result well-defined
// even if the caller mis-counts.
//
// @aitri-trace FR-ID: FR-018, TC-ID: TC-NM-003e
func UptimePct(samplesUp, totalSamples int64) float64 {
	if totalSamples <= 0 {
		return 0
	}
	switch {
	case samplesUp < 0:
		samplesUp = 0
	case samplesUp > totalSamples:
		samplesUp = totalSamples
	}
	return float64(samplesUp) * 100.0 / float64(totalSamples)
}
