// Module:       internal/ups
// Purpose:      Estimated battery percentage from battery.voltage (FR-018).
// Dependencies: standard library only.
package ups

// EstimateBatteryPct estimates the battery charge percentage by linear
// interpolation of battery.voltage between the configured low and high bounds,
// clamped to [0,100].
//
// This UPS does NOT publish battery.charge, so the value is always an estimate
// and every consumer must present it labelled "estimado" — never as a device
// reading (FR-018 / no_go_zone).
//
// v:    the measured battery.voltage.
// low:  the voltage mapped to 0%   (default 21.0 V).
// high: the voltage mapped to 100% (default 27.4 V).
//
// Returns a percentage in [0,100]. If low >= high (misconfiguration that the
// config loader already rejects) the function returns 0 rather than dividing by
// a non-positive span.
// @aitri-trace FR-018 US-018 AC-018-001 TC-UPS-010h
func EstimateBatteryPct(v, low, high float64) float64 {
	if high <= low {
		return 0
	}
	pct := (v - low) / (high - low) * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}
