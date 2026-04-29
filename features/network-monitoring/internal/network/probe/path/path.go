// Package path runs low-cadence traceroute probes through the privileged helper.
//
// SKELETON-ONLY. The helper invocation, hop diff, and path_changed event emission
// are not implemented — see spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package path

import (
	"context"
	"errors"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/probe/path: skeleton-only — not implemented")

// Hop is one entry in a traceroute result.
//
// @aitri-trace FR-ID: FR-029
type Hop struct {
	IP    string
	RTTMs float64
}

// Result is one traceroute outcome row destined for net_path.
//
// @aitri-trace FR-ID: FR-029
type Result struct {
	TargetID int64
	HopCount int
	Hops     []Hop
}

// Worker is the path/traceroute scheduler lifecycle surface.
type Worker interface {
	Run(ctx context.Context) error
	Stop() error
}

// New returns a skeleton path Worker.
func New() Worker { return &skeletonWorker{} }

type skeletonWorker struct{}

func (s *skeletonWorker) Run(context.Context) error { return ErrSkeleton }
func (s *skeletonWorker) Stop() error               { return nil }
