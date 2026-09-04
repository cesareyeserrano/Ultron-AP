// Module:       internal/ups
// Purpose:      UPS power-event alert rules with debounce/dedup (FR-021).
// Dependencies: standard library only. Delivery (Telegram/DB) is injected via a
//
//	sink so this package stays decoupled from internal/notify.
package ups

import (
	"fmt"
	"time"
)

// Severity mirrors the dashboard alert severities.
type Severity string

const (
	SevInfo     Severity = "info"
	SevWarning  Severity = "warning"
	SevCritical Severity = "critical"
)

// Alert is a UPS-derived alert handed to the sink (which turns it into a
// Telegram message and/or a persisted alert row).
type Alert struct {
	Severity Severity
	Source   string // always "ups"
	// Kind is a stable per-rule discriminator ("outage", "lowbatt", ...).
	// The notification layer keys its storm/dedup cache on it so that two
	// semantically different UPS alerts cannot collapse into one another's
	// chat row — a low-battery critical must never be swallowed by an
	// in-flight mains-outage message.
	Kind    string
	Message string
	Resolve bool // true => a recovery/resolve notification, not a new fire
}

// AlertSink delivers an Alert. Injected by main; nil in a bare poller.
type AlertSink func(Alert)

// Alerter applies the FR-021 rules to the stream of snapshots, with debounce and
// deduplication so a mains flicker cannot fire ten messages. One Alerter per
// poller; Observe is called once per poll.
type Alerter struct {
	cfg  Config
	sink AlertSink
	now  func() time.Time

	prevState     State
	inUnreachable bool
	outageStart   time.Time
	lbFired       bool      // low-battery critical already fired this outage
	battCritFired bool      // battery-voltage critical already fired this episode
	voltOutStart  time.Time // when input voltage first left range (debounce)
	voltFired     bool
	lastRB        time.Time // last replace-battery warning (once/day)
}

// NewAlerter builds an Alerter delivering through sink.
func NewAlerter(cfg Config, sink AlertSink) *Alerter {
	return &Alerter{cfg: cfg, sink: sink, now: time.Now}
}

// Observe evaluates one snapshot and fires any alerts it triggers.
// @aitri-trace FR-021 US-021 AC-021-001 TC-UPS-019h
func (a *Alerter) Observe(snap Snapshot) {
	// Unreachable: fire once per outage; recover once when it returns.
	if !snap.Reachable {
		if !a.inUnreachable {
			a.inUnreachable = true
			a.emit(SevWarning, "comms", "UPS sin comunicación", false)
		}
		return
	}
	if a.inUnreachable {
		a.inUnreachable = false
		a.emit(SevInfo, "comms", "UPS: comunicación restablecida", true)
	}

	prevOut := isOutage(a.prevState)
	curOut := isOutage(snap.State)

	switch {
	case curOut && !prevOut: // mains outage begins
		a.outageStart = snap.LastGood
		a.lbFired = false
		if snap.State == StateLowBatt {
			a.lbFired = true
			a.emit(SevCritical, "lowbatt", "Batería baja — UPS en batería crítica", false)
		} else {
			a.emit(SevWarning, "outage", "En batería — corte de red detectado", false)
		}
	case curOut && prevOut && snap.State == StateLowBatt && !a.lbFired: // escalation OB→LB
		a.lbFired = true
		a.emit(SevCritical, "lowbatt", "Batería baja — UPS en batería crítica", false)
	case !curOut && prevOut: // mains restored
		dur := snap.LastGood.Sub(a.outageStart)
		if dur < 0 {
			dur = 0
		}
		a.emit(SevInfo, "outage", fmt.Sprintf("Red eléctrica restablecida tras %s", formatDur(dur)), true)
		a.lbFired = false
	}

	// Battery voltage near the configured cutoff — Critical, once per episode.
	if snap.BatteryV != nil {
		switch {
		case *snap.BatteryV <= a.cfg.BattLowV+0.3:
			if !a.battCritFired {
				a.battCritFired = true
				a.emit(SevCritical, "battvolt", fmt.Sprintf("Voltaje de batería crítico: %.1f V", *snap.BatteryV), false)
			}
		case *snap.BatteryV > a.cfg.BattLowV+0.5:
			a.battCritFired = false // recovered — allow a future episode to alert
		}
	}

	// Input voltage out of range — Warning, only after the debounce window so a
	// single dip that recovers does not alert (FR-021 AC-021-003).
	if snap.InputV != nil {
		if *snap.InputV < a.cfg.InputVLow || *snap.InputV > a.cfg.InputVHigh {
			if a.voltOutStart.IsZero() {
				a.voltOutStart = a.now()
			}
			if !a.voltFired && a.now().Sub(a.voltOutStart) >= a.cfg.Debounce {
				a.voltFired = true
				a.emit(SevWarning, "inputvolt", fmt.Sprintf("Voltaje de entrada fuera de rango: %.0f V", *snap.InputV), false)
			}
		} else {
			a.voltOutStart = time.Time{}
			a.voltFired = false
		}
	}

	// Replace-battery — Warning at most once per day (no spam).
	if snap.State == StateReplace {
		if a.lastRB.IsZero() || a.now().Sub(a.lastRB) >= 24*time.Hour {
			a.lastRB = a.now()
			a.emit(SevWarning, "replace", "Reemplazar batería del UPS", false)
		}
	}

	a.prevState = snap.State
}

func (a *Alerter) emit(sev Severity, kind, msg string, resolve bool) {
	if a.sink != nil {
		a.sink(Alert{Severity: sev, Source: "ups", Kind: kind, Message: msg, Resolve: resolve})
	}
}

// FormatDur exposes the human duration formatter to the dashboard layer.
func FormatDur(d time.Duration) string { return formatDur(d) }

// formatDur renders a human-readable duration in Spanish-friendly units.
func formatDur(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%d s", int(d.Seconds()))
	}
	m := int(d.Minutes())
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	h := m / 60
	m = m % 60
	if h < 24 {
		return fmt.Sprintf("%d h %d min", h, m)
	}
	days := h / 24
	h = h % 24
	return fmt.Sprintf("%d d %d h", days, h)
}
