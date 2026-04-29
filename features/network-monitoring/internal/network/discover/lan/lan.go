// Package lan reads /proc/net/arp and listens to mDNS announcements on
// 224.0.0.251:5353 to discover LAN devices.
//
// SKELETON-ONLY. The ARP reader, mDNS listener with SO_REUSEPORT, and virtual
// interface filter are not implemented — see spec/04_IMPLEMENTATION_MANIFEST.json.
//
// Hard package boundary: this package MUST NOT import gopacket or any pcap
// library. LAN discovery is passive only (FR-027 + security design).
package lan

import (
	"context"
	"errors"
	"time"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/discover/lan: skeleton-only — not implemented")

// DeviceObserved is one observation emitted by the discovery worker.
//
// @aitri-trace FR-ID: FR-027
type DeviceObserved struct {
	MAC       string // canonical lowercase
	Hostname  string
	IP        string
	Vendor    string
	FirstSeen time.Time
	LastSeen  time.Time
}

// Worker is the LAN discovery lifecycle surface.
type Worker interface {
	Run(ctx context.Context) error
	Stop() error
}

// New returns a skeleton LAN discovery Worker.
func New() Worker { return &skeletonWorker{} }

type skeletonWorker struct{}

func (s *skeletonWorker) Run(context.Context) error { return ErrSkeleton }
func (s *skeletonWorker) Stop() error               { return nil }
