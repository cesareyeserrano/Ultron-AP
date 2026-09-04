// Module:       internal/ups
// Purpose:      Presentation helpers the dashboard template calls on a Snapshot.
// Dependencies: standard library only. All returned strings are plain text and
//
//	are HTML-escaped by html/template at render time (NFR-019).
package ups

import (
	"fmt"
	"math"
)

// unreachableReason is shown on the card when the UPS does not answer.
const unreachableReason = "UPS sin comunicación"

// SeverityClass returns the CSS modifier class for the card border, reusing the
// existing dashboard tile classes so the UPS card reads like every other tile.
func (s Snapshot) SeverityClass() string {
	switch {
	case !s.Reachable:
		return "metric-ups-muted"
	case s.State.IsCritical():
		return "metric-critical"
	case s.State.IsWarning():
		return "metric-warning"
	default:
		return ""
	}
}

// Reason returns the explanatory line for the unreachable state, empty otherwise.
func (s Snapshot) Reason() string {
	if !s.Reachable {
		return unreachableReason
	}
	return ""
}

// Summary is the one-line subtitle for the compact dashboard tile — the
// at-a-glance detail under the status headline.
func (s Snapshot) Summary() string {
	if !s.Reachable {
		return "sin comunicación"
	}
	if s.BattPctEst == nil {
		return "batería —"
	}
	return "batería " + s.BattPctStr() + "% estimado"
}

// battTier classifies the estimated battery level for the history-chart colour,
// mirroring the ok/warn/crit tiers the network latency tiles use.
func (s Snapshot) battTier() string {
	if s.BattPctEst == nil {
		return ""
	}
	switch {
	case *s.BattPctEst < 30:
		return "crit"
	case *s.BattPctEst < 60:
		return "warn"
	default:
		return "ok"
	}
}

// BattSeriesClass returns the text-colour class for the battery chart's current
// value, matching latencySeriesClass' palette.
func (s Snapshot) BattSeriesClass() string {
	switch s.battTier() {
	case "crit":
		return "text-danger"
	case "warn":
		return "text-yellow-400"
	case "ok":
		return "text-green-400"
	default:
		return "text-text-muted"
	}
}

// BattSeriesStroke returns the sparkline stroke colour for the battery chart,
// matching latencySeriesStroke's palette.
func (s Snapshot) BattSeriesStroke() string {
	switch s.battTier() {
	case "crit":
		return "var(--color-danger)"
	case "warn":
		return "var(--color-yellow-400)"
	case "ok":
		return "var(--color-green-400)"
	default:
		return "var(--color-accent)"
	}
}

// InOutage reports whether the snapshot is running on battery (used by the
// server to decide when to show the "en red desde hace" counter).
func (s Snapshot) InOutage() bool { return isOutage(s.State) }

// inputTier classifies the input voltage against the configured alert range.
func (s Snapshot) inputTier() string {
	if s.InputV == nil {
		return ""
	}
	if s.InLowV > 0 && s.InHighV > s.InLowV && (*s.InputV < s.InLowV || *s.InputV > s.InHighV) {
		return "crit"
	}
	return "ok"
}

// InputSeriesClass returns the text-colour class for the input-voltage chart's
// current value (green in range, red out of range — like the latency tiles).
func (s Snapshot) InputSeriesClass() string {
	switch s.inputTier() {
	case "crit":
		return "text-danger"
	case "ok":
		return "text-green-400"
	default:
		return "text-text-muted"
	}
}

// InputSeriesStroke returns the sparkline stroke colour for the input-voltage chart.
func (s Snapshot) InputSeriesStroke() string {
	switch s.inputTier() {
	case "crit":
		return "var(--color-danger)"
	case "ok":
		return "var(--color-green-400)"
	default:
		return "var(--color-accent)"
	}
}

// HeadlineTextClass returns the Tailwind text-colour class for the status
// headline, matching the severity colours used across the dashboard tiles.
func (s Snapshot) HeadlineTextClass() string {
	switch {
	case !s.Reachable:
		return "text-text-muted"
	case s.State.IsCritical():
		return "text-danger"
	case s.State.IsWarning():
		return "text-yellow-400"
	default:
		return "text-accent"
	}
}

// LoadStr renders the UPS load, or "—" when absent (never a fabricated 0).
func (s Snapshot) LoadStr() string { return fmtFloat(s.LoadPct, "%.0f %%") }

// InputStr renders the input voltage, or "—".
func (s Snapshot) InputStr() string { return fmtFloat(s.InputV, "%.0f V") }

// BatteryStr renders the battery voltage, or "—".
func (s Snapshot) BatteryStr() string { return fmtFloat(s.BatteryV, "%.1f V") }

// BattPctStr renders the estimated battery percentage as a whole number, or
// "—". The template always pairs it with the visible "estimado" label.
func (s Snapshot) BattPctStr() string {
	if s.BattPctEst == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f", math.Round(*s.BattPctEst))
}

// BattPctWidth returns the battery bar fill width as a whole-number percentage
// (0 when unknown) for an inline style="width:NN%".
func (s Snapshot) BattPctWidth() int {
	if s.BattPctEst == nil {
		return 0
	}
	return int(math.Round(*s.BattPctEst))
}

// DelayShutStr renders ups.delay.shutdown in seconds, or "no disponible" (FR-023).
func (s Snapshot) DelayShutStr() string { return fmtInt(s.DelayShut) }

// DelayStartStr renders ups.delay.start in seconds, or "no disponible".
func (s Snapshot) DelayStartStr() string { return fmtInt(s.DelayStart) }

// CutoffStr renders the configured low-battery cutoff shown as the shutdown
// point (FR-023). Sourced from config, never from a privileged file read.
func (s Snapshot) CutoffStr() string { return fmt.Sprintf("%.1f V", s.CutoffV) }

func fmtFloat(p *float64, format string) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf(format, *p)
}

func fmtInt(p *int) string {
	if p == nil {
		return "no disponible"
	}
	return fmt.Sprintf("%d s", *p)
}
