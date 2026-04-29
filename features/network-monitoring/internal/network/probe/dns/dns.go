// Package dns is the DNS resolver probe worker.
//
// SKELETON for the worker loop. RandomCacheBypassLabel is implemented in full
// because it is pure logic and a unit test (TC-NM-002e) pins it.
package dns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/probe/dns: skeleton-only — not implemented")

// labelLenBytes is the entropy length of the cache-bypass prefix. 8 bytes →
// 16 hex chars, far below the 63-char DNS label limit.
const labelLenBytes = 8

// RandomCacheBypassLabel returns a 16-character lowercase hex string suitable
// to prepend as a DNS label so that each lookup misses the upstream resolver
// cache. The same configured domain produces a unique FQDN per call.
//
// The returned label is also guaranteed to start with a letter (the prefix
// "n") so the label is RFC 1035-valid even when followed by a digit-only host.
//
// Pass crypto/rand.Reader in production. Tests may pass a deterministic Reader.
//
// @aitri-trace FR-ID: FR-017, TC-ID: TC-NM-002e
func RandomCacheBypassLabel(rng io.Reader) (string, error) {
	if rng == nil {
		rng = rand.Reader
	}
	buf := make([]byte, labelLenBytes)
	if _, err := io.ReadFull(rng, buf); err != nil {
		return "", err
	}
	// "n" prefix keeps the label letter-leading per RFC 1035.
	return "n" + hex.EncodeToString(buf), nil
}

// Worker is the DNS probe lifecycle surface owned by the orchestrator.
//
// @aitri-trace FR-ID: FR-017
type Worker interface {
	Run(ctx context.Context) error
	Stop() error
}

// New returns a skeleton DNS probe Worker.
func New() Worker { return &skeletonWorker{} }

type skeletonWorker struct{}

func (s *skeletonWorker) Run(context.Context) error { return ErrSkeleton }
func (s *skeletonWorker) Stop() error               { return nil }
