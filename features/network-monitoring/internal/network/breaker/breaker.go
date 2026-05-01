// Package breaker is the resource-budget circuit breaker for network probes.
//
// SKELETON-ONLY. See spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package breaker

import "errors"

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/breaker: skeleton-only — not implemented")

// Class identifies a probe class for budget accounting.
type Class int

const (
	ClassICMP Class = iota
	ClassDNS
	ClassPath
	ClassSpeedtest
	ClassWiFi
	ClassDiscovery
)

// State is the breaker's current snapshot, exposed at /api/network/cost.
//
// @aitri-trace FR-ID: FR-023
type State struct {
	Active bool
	Reason string
	CPUPct float64
	RSSMB  int
	BW24h  int64 // bytes consumed in trailing 24h
}

// Breaker enforces FR-023 CPU/RAM/bandwidth budgets and pauses lowest-priority
// classes when budgets are exceeded.
//
// @aitri-trace FR-ID: FR-023
type Breaker interface {
	AllowSample(class Class) bool
	AllowSpeedtest() (allowed bool, reason string)
	ConsumeBytes(class Class, n int)
	State() State
}

// CPUHighThresholdPct is the 5-minute CPU usage above which the breaker
// engages the cpu_high condition. FR-023 budgets a 5% baseline; the
// threshold leaves a 1-point slack for harmless transients.
const CPUHighThresholdPct = 6.0

// Decide is the pure decision function for the CPU-high condition: it
// returns the State a stateful Tick would produce given the current 5-minute
// CPU mean. Strictly greater-than the threshold, so 6.0 itself does not
// engage — only sustained excess does.
//
// @aitri-trace FR-ID: FR-023, TC-ID: TC-NM-008h
func Decide(cpuPct5m float64) State {
	if cpuPct5m > CPUHighThresholdPct {
		return State{Active: true, Reason: "cpu_high", CPUPct: cpuPct5m}
	}
	return State{CPUPct: cpuPct5m}
}

// New returns a no-op Breaker that allows everything. Real budget logic lives
// in a future build phase.
func New() Breaker { return &noopBreaker{} }

type noopBreaker struct{}

func (n *noopBreaker) AllowSample(Class) bool                  { return true }
func (n *noopBreaker) AllowSpeedtest() (bool, string)          { return true, "" }
func (n *noopBreaker) ConsumeBytes(Class, int)                 {}
func (n *noopBreaker) State() State                            { return State{} }
