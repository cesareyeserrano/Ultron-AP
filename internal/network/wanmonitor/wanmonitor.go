// Package wanmonitor watches gateway-probe snapshots and detects WAN
// up/down transitions: the public-target probe failing N times in a row
// while the LAN gateway still responds = WAN_DOWN; one OK sample on the
// public target = WAN_UP.
//
// Scope (FR-018 MVP):
//   - One public-internet target (e.g. "cloudflare") and one LAN gateway
//     target (e.g. "gateway"); both labels supplied at construction.
//   - State transitions are emitted to the optional EventSink (for DB
//     persistence + structured logging at the call site).
//   - The current state and the last event are exposed via Snapshot() for
//     UI rendering.
//
// Out of scope: multi-public-target consensus, hysteresis windows, alerts.
// Those belong to BG-015 and beyond.
package wanmonitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
)

// State is the monitor's current verdict on WAN reachability.
type State string

const (
	StateUnknown State = "unknown" // before any public sample observed
	StateUp      State = "up"
	StateDown    State = "down"
)

// Event is one state transition.
type Event struct {
	TS     time.Time
	Kind   string // "wan_up" | "wan_down"
	Detail string
}

// EventSink receives transitions for persistence / structured logging.
// It is invoked synchronously from Observe; callers must keep work cheap
// or hand off to a goroutine.
type EventSink func(e Event)

// Snapshot is the renderable view of the monitor.
type Snapshot struct {
	State        State
	Since        time.Time
	PublicLabel  string
	GatewayLabel string
	LastEvent    *Event // nil before the first transition
}

// Monitor implements the FR-018 MVP state machine.
type Monitor struct {
	publicLabel  string
	gatewayLabel string
	threshold    int
	sink         EventSink

	mu             sync.Mutex
	state          State
	since          time.Time
	publicFailures int
	lastGatewayOK  bool
	lastEvent      *Event
}

// New constructs a Monitor. publicLabel must match a configured probe
// target Label (e.g. "cloudflare"). gatewayLabel is the LAN gateway label
// used to disambiguate WAN outage from full LAN outage. threshold is the
// number of consecutive public-target failures required to declare WAN
// down (default 3 if <= 0).
func New(publicLabel, gatewayLabel string, threshold int, sink EventSink) *Monitor {
	if threshold <= 0 {
		threshold = 3
	}
	return &Monitor{
		publicLabel:  publicLabel,
		gatewayLabel: gatewayLabel,
		threshold:    threshold,
		sink:         sink,
		state:        StateUnknown,
	}
}

// Observe is fed every probe snapshot. Samples for unrelated targets are
// ignored. Safe for concurrent use.
func (m *Monitor) Observe(snap gatewayprobe.Snapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch snap.Label {
	case m.gatewayLabel:
		m.lastGatewayOK = snap.Status == gatewayprobe.StatusOK
		return
	case m.publicLabel:
		// fall through to state machine below
	default:
		return
	}

	if snap.Status == gatewayprobe.StatusOK {
		m.publicFailures = 0
		switch m.state {
		case StateDown:
			m.transitionLocked(StateUp, snap.LastProbe,
				fmt.Sprintf("recovered on %s (rtt=%.1fms)", snap.Target, snap.RTTMs))
		case StateUnknown:
			m.transitionLocked(StateUp, snap.LastProbe, "first OK sample")
		}
		return
	}

	// Public target sample failed.
	m.publicFailures++
	if m.publicFailures >= m.threshold && m.state != StateDown && m.lastGatewayOK {
		gwHint := "gateway still ok"
		m.transitionLocked(StateDown, snap.LastProbe,
			fmt.Sprintf("%d consecutive failures to %s (%s); %s",
				m.publicFailures, m.publicLabel, snap.Target, gwHint))
	}
}

func (m *Monitor) transitionLocked(newState State, ts time.Time, detail string) {
	if m.state == newState {
		return
	}
	m.state = newState
	m.since = ts
	if ts.IsZero() {
		m.since = time.Now()
	}
	kind := ""
	switch newState {
	case StateDown:
		kind = "wan_down"
	case StateUp:
		kind = "wan_up"
	default:
		return
	}
	ev := &Event{TS: m.since, Kind: kind, Detail: detail}
	m.lastEvent = ev
	if m.sink != nil {
		m.sink(*ev)
	}
}

// Snapshot returns the current state for UI rendering.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	var le *Event
	if m.lastEvent != nil {
		copy := *m.lastEvent
		le = &copy
	}
	return Snapshot{
		State:        m.state,
		Since:        m.since,
		PublicLabel:  m.publicLabel,
		GatewayLabel: m.gatewayLabel,
		LastEvent:    le,
	}
}
