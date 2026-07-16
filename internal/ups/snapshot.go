// Module:       internal/ups
// Purpose:      Immutable Snapshot of UPS state served to SSE / history / alerts.
// Dependencies: standard library only.
package ups

import "time"

// Snapshot is the immutable view of the UPS at one poll. Nil pointer fields
// mean "no value" (rendered as "—"), never a fabricated zero — important when
// the UPS is unreachable (FR-017: no zeros, no blank card).
type Snapshot struct {
	State      State    // derived state
	RawStatus  string   // raw ups.status, stored verbatim for audit
	LoadPct    *float64 // ups.load (%)
	InputV     *float64 // input.voltage
	InputHz    *float64 // input.frequency (Hz)
	BatteryV   *float64 // battery.voltage
	BattPctEst *float64 // estimated %, ALWAYS labelled "estimado"; never battery.charge
	Beeper     string   // raw ups.beeper.status ("enabled"|"muted"|"disabled"|"")
	DelayShut  *int     // ups.delay.shutdown (s); nil => "no disponible" (FR-023)
	DelayStart *int     // ups.delay.start (s); nil => "no disponible"
	CutoffV    float64  // configured low-battery cutoff shown as "punto de apagado" (FR-023)
	InLowV     float64  // configured input-voltage alert range (chart colour tier)
	InHighV    float64
	LastGood   time.Time // time of the last successful poll
	Reachable  bool      // false => card shows "Sin datos"
}

// beeperLabel maps the raw NUT beeper status to a Spanish label.
var beeperLabel = map[string]string{
	"enabled":  "activado",
	"muted":    "silenciado",
	"disabled": "deshabilitado",
}

// BeeperLabel returns the Spanish label for the beeper state, "—" when the UPS
// did not publish it, or the raw value for an unrecognised status. The raw
// value is returned as plain text and is HTML-escaped by the template
// (NFR-019) — it is never treated as markup.
func (s Snapshot) BeeperLabel() string {
	if l, ok := beeperLabel[s.Beeper]; ok {
		return l
	}
	if s.Beeper == "" {
		return "—"
	}
	return s.Beeper
}

// StateLabel returns the Spanish status label for the card.
func (s Snapshot) StateLabel() string { return s.State.Label() }

// Estimated returns true when a battery percentage estimate is available.
func (s Snapshot) Estimated() bool { return s.BattPctEst != nil }
