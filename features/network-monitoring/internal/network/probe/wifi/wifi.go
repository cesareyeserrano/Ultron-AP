// Package wifi samples /proc/net/wireless and `iw dev wlanX link` (via helper).
//
// SKELETON-ONLY. /proc parsing, helper invocation, and "Not applicable" detection
// are not implemented — see spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package wifi

import (
	"context"
	"errors"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/probe/wifi: skeleton-only — not implemented")

// Sample is a single WiFi link snapshot (FR-028).
//
// @aitri-trace FR-ID: FR-028
type Sample struct {
	Applicable    bool
	RSSIDBm       int
	LinkQuality   int
	BitrateMbps   float64
	Channel       int
	Band          string
	Retries       int
	CRCErrors     int
	TSUnixMillis  int64
}

// Worker is the WiFi sampler lifecycle surface owned by the orchestrator.
type Worker interface {
	Run(ctx context.Context) error
	Stop() error
}

// New returns a skeleton WiFi Worker.
func New() Worker { return &skeletonWorker{} }

type skeletonWorker struct{}

func (s *skeletonWorker) Run(context.Context) error { return ErrSkeleton }
func (s *skeletonWorker) Stop() error               { return nil }
