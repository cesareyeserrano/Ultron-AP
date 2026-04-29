// Package icmp is the ICMP/UDP probe worker.
//
// SKELETON-ONLY. The pro-bing-backed worker, jitter EWMA, loss window and
// helper-fallback path are not implemented — see
// spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package icmp

import (
	"context"
	"errors"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/probe/icmp: skeleton-only — not implemented")

// Worker is the ICMP probe lifecycle surface owned by the orchestrator.
//
// @aitri-trace FR-ID: FR-016
type Worker interface {
	Run(ctx context.Context) error
	Stop() error
}

// New returns a skeleton ICMP probe Worker.
func New() Worker { return &skeletonWorker{} }

type skeletonWorker struct{}

func (s *skeletonWorker) Run(context.Context) error { return ErrSkeleton }
func (s *skeletonWorker) Stop() error               { return nil }
