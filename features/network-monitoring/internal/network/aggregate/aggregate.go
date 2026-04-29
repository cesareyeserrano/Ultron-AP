// Package aggregate downsamples raw samples into minute/hour/day buckets and
// picks the right resolution for series queries.
//
// SKELETON-ONLY for the bucket writers and compaction job. PickResolution is
// implemented in full because it is pure logic and a unit test (TC-NM-005e)
// pins it.
package aggregate

import (
	"errors"
	"time"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/aggregate: skeleton-only — not implemented")

// Resolution is the bucket size returned by PickResolution.
type Resolution string

const (
	ResolutionRaw    Resolution = "raw"
	ResolutionMinute Resolution = "minute"
	ResolutionHour   Resolution = "hour"
	ResolutionDay    Resolution = "day"
)

// PickResolution chooses the lowest-cardinality bucket size that still keeps
// the response under the 600-point chart bound (system design §Performance).
//
// Bands (inclusive upper):
//   ≤ 1h   → raw
//   ≤ 6h   → minute
//   ≤ 7d   → hour
//   > 7d   → day
//
// @aitri-trace FR-ID: FR-020, TC-ID: TC-NM-005e
func PickResolution(window time.Duration) Resolution {
	switch {
	case window <= time.Hour:
		return ResolutionRaw
	case window <= 6*time.Hour:
		return ResolutionMinute
	case window <= 7*24*time.Hour:
		return ResolutionHour
	default:
		return ResolutionDay
	}
}

// Downsampler runs the periodic raw → minute → hour → day compaction job.
//
// @aitri-trace FR-ID: FR-020
type Downsampler interface {
	RunOnce() error
	Stop() error
}

// New returns a skeleton Downsampler.
func New() Downsampler { return &skeletonDownsampler{} }

type skeletonDownsampler struct{}

func (s *skeletonDownsampler) RunOnce() error { return ErrSkeleton }
func (s *skeletonDownsampler) Stop() error    { return nil }
