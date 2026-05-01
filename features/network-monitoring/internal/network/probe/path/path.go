// Package path runs low-cadence traceroute probes through the privileged helper.
//
// SKELETON-ONLY. The helper invocation, hop diff, and path_changed event emission
// are not implemented — see spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package path

import (
	"context"
	"errors"
	"net"
	"strconv"
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

// ErrInvalidTracerouteTarget signals an unparseable or unsafe target. The
// target must be a single IP literal — no hostnames, no shell metacharacters,
// no flag-shaped strings.
var ErrInvalidTracerouteTarget = errors.New("network-monitoring/probe/path: target must be a single IP literal")

// MaxTracerouteHops caps -m for traceroute helper calls. Pinned in code so
// any change goes through review.
const MaxTracerouteHops = 30

// BuildTracerouteArgs returns the closed-form helper argv for a traceroute
// against target. The shape is exactly:
//
//	["-n", "-m", "30", target]
//
// target is validated as a single IP literal (IPv4 or IPv6) — anything that
// could be reinterpreted by the helper allow-list grammar (hostnames,
// metacharacters, flag-shaped strings) is rejected before reaching the
// socket. This function is the single producer of traceroute argv for the
// helper; the allow-list grammar in package helper accepts exactly this
// shape.
//
// @aitri-trace FR-ID: FR-029, TC-ID: TC-NM-014e
func BuildTracerouteArgs(target string) ([]string, error) {
	if net.ParseIP(target) == nil {
		return nil, ErrInvalidTracerouteTarget
	}
	return []string{"-n", "-m", strconv.Itoa(MaxTracerouteHops), target}, nil
}

// New returns a skeleton path Worker.
func New() Worker { return &skeletonWorker{} }

type skeletonWorker struct{}

func (s *skeletonWorker) Run(context.Context) error { return ErrSkeleton }
func (s *skeletonWorker) Stop() error               { return nil }
