// Package ups reads a NUT-managed UPS over the native NUT TCP protocol and
// exposes its state to the Ultron-AP dashboard, history, alert and insight
// subsystems.
//
// Module:       internal/ups
// Purpose:      UPS state model + raw ups.status parsing (read-only).
// Dependencies: standard library only.
package ups

import "strings"

// State is the derived, single-valued UPS state. The raw ups.status can be a
// compound flag set (e.g. "OB LB"); ParseStatus collapses it to the most
// severe State using a fixed precedence.
type State string

const (
	StateOnline      State = "online"      // OL      — on mains
	StateOnBattery   State = "onbattery"   // OB      — on battery
	StateLowBatt     State = "lowbatt"     // LB      — battery low
	StateCharging    State = "charging"    // OL CHRG — charging on mains
	StateReplace     State = "replace"     // RB      — replace battery
	StateBypass      State = "bypass"      // BYPASS
	StateOff         State = "off"         // OFF
	StateAlarm       State = "alarm"       // ALARM
	StateUnreachable State = "unreachable" // no NUT data (set by the poller, not by ups.status)
)

// spanishLabel maps a State to the dashboard label (FR-017 status map).
var spanishLabel = map[State]string{
	StateOnline:      "En red",
	StateOnBattery:   "En batería",
	StateLowBatt:     "Batería baja",
	StateCharging:    "Cargando",
	StateReplace:     "Reemplazar batería",
	StateBypass:      "Bypass",
	StateOff:         "Apagado",
	StateAlarm:       "Alarma",
	StateUnreachable: "Sin datos",
}

// Label returns the Spanish dashboard label for the state.
//
// Returns "Sin datos" for an unknown state so the card never renders a blank.
func (s State) Label() string {
	if l, ok := spanishLabel[s]; ok {
		return l
	}
	return spanishLabel[StateUnreachable]
}

// IsAlert reports whether the state warrants operator attention (used by the
// card severity border and, later, the alert engine).
func (s State) IsCritical() bool {
	return s == StateLowBatt || s == StateOff || s == StateAlarm
}

// OnBattery reports whether the state means the UPS is carrying the load on
// battery (a mains outage) — the condition the "cortes" chart timelines.
func (s State) OnBattery() bool { return s == StateOnBattery || s == StateLowBatt }

// IsWarning reports the warning tier (OB / RB / BYPASS).
func (s State) IsWarning() bool {
	return s == StateOnBattery || s == StateReplace || s == StateBypass
}

// ParseStatus maps a raw ups.status value to a single State.
//
// The device may report a compound status such as "OB LB"; the most severe
// flag wins, in this precedence:
//
//	LB > OFF > ALARM > OB > RB > BYPASS > (OL + CHRG) > OL
//
// An empty or wholly unknown status defaults to StateOnline (a present-but-
// unrecognised UPS is assumed on mains); "unreachable" is never derived here —
// it is set by the poller when NUT does not answer.
//
// raw: the raw ups.status string. Returns the derived State.
func ParseStatus(raw string) State {
	flags := map[string]bool{}
	for _, tok := range strings.Fields(strings.ToUpper(raw)) {
		flags[tok] = true
	}
	switch {
	case flags["LB"]:
		return StateLowBatt
	case flags["OFF"]:
		return StateOff
	case flags["ALARM"]:
		return StateAlarm
	case flags["OB"]:
		return StateOnBattery
	case flags["RB"]:
		return StateReplace
	case flags["BYPASS"]:
		return StateBypass
	case flags["OL"] && flags["CHRG"]:
		return StateCharging
	default:
		return StateOnline
	}
}
