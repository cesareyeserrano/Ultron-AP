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

// New returns a no-op Breaker that allows everything. Real budget logic lives
// in a future build phase.
func New() Breaker { return &noopBreaker{} }

type noopBreaker struct{}

func (n *noopBreaker) AllowSample(Class) bool                  { return true }
func (n *noopBreaker) AllowSpeedtest() (bool, string)          { return true, "" }
func (n *noopBreaker) ConsumeBytes(Class, int)                 {}
func (n *noopBreaker) State() State                            { return State{} }
