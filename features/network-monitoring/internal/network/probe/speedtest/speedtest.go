// Package speedtest dispatches on-demand and scheduled speedtests through the
// privileged helper using `librespeed-cli`.
//
// SKELETON for the dispatcher. GradeBufferbloat is implemented in full because
// it is pure logic and unit tests (TC-NM-010e/h/f) pin the grade boundaries.
package speedtest

import (
	"context"
	"errors"
	"sync/atomic"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/probe/speedtest: skeleton-only — not implemented")

// Trigger identifies why a speedtest ran.
type Trigger string

const (
	TriggerManual    Trigger = "manual"
	TriggerScheduled Trigger = "scheduled"
)

// Result is one speedtest outcome destined for net_speedtests.
//
// @aitri-trace FR-ID: FR-024, FR-025
type Result struct {
	Trigger                Trigger
	DownMbps               float64
	UpMbps                 float64
	IdleRTTMs              float64
	LoadedRTTDownMs        float64
	LoadedRTTUpMs          float64
	BufferbloatAddedDownMs float64
	BufferbloatAddedUpMs   float64
	BufferbloatGrade       string // "A".."F"
	BytesUsed              int64
	Status                 string // "ok"|"failed"|"aborted"|"budget_exhausted"
}

// Bufferbloat grade thresholds, in milliseconds of added latency under load.
// A grade is the upper-inclusive bound of its band.
const (
	GradeABoundary = 30.0
	GradeBBoundary = 60.0
	GradeCBoundary = 100.0
	GradeDBoundary = 250.0
)

// GradeBufferbloat returns the bufferbloat letter grade for a measured added
// latency in milliseconds. Bands are upper-inclusive:
//
//	addedMs ≤ 30   → A
//	addedMs ≤ 60   → B
//	addedMs ≤ 100  → C
//	addedMs ≤ 250  → D
//	addedMs >  250 → F
//
// Negative values clamp to 0 (treated as A) — measurement noise.
//
// @aitri-trace FR-ID: FR-025, TC-ID: TC-NM-010e, TC-NM-010h, TC-NM-010f
func GradeBufferbloat(addedMs float64) string {
	switch {
	case addedMs <= GradeABoundary:
		return "A"
	case addedMs <= GradeBBoundary:
		return "B"
	case addedMs <= GradeCBoundary:
		return "C"
	case addedMs <= GradeDBoundary:
		return "D"
	default:
		return "F"
	}
}

// Dispatcher is the on-demand+scheduled speedtest surface.
type Dispatcher interface {
	Run(ctx context.Context, t Trigger) (Result, error)
}

// New returns a skeleton Dispatcher.
func New() Dispatcher { return &skeletonDispatcher{} }

type skeletonDispatcher struct{}

func (s *skeletonDispatcher) Run(context.Context, Trigger) (Result, error) {
	return Result{}, ErrSkeleton
}

// RunGuard mediates the FR-024 "only one speedtest at a time" invariant.
// TryAcquire returns true exactly once between Release calls; concurrent
// callers see false, which the HTTP layer translates to 409 already_running.
//
// @aitri-trace FR-ID: FR-024
type RunGuard struct {
	running atomic.Bool
}

// TryAcquire returns true iff no speedtest is currently in flight. The
// caller MUST Release when finished. A second TryAcquire while held
// returns false.
//
// @aitri-trace FR-ID: FR-024, TC-ID: TC-NM-009f
func (g *RunGuard) TryAcquire() bool { return g.running.CompareAndSwap(false, true) }

// Release marks the guarded operation finished. Idempotent — calling
// Release without a prior TryAcquire is a no-op.
func (g *RunGuard) Release() { g.running.Store(false) }
