// Package wifi samples /proc/net/wireless and `iw dev wlanX link` (via helper).
//
// SKELETON-ONLY. /proc parsing, helper invocation, and "Not applicable" detection
// are not implemented — see spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package wifi

import (
	"context"
	"errors"
	"strconv"
	"strings"
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

// ErrProcWirelessNotApplicable signals that /proc/net/wireless lists no
// wireless interface (e.g. an Ethernet-only Pi).
var ErrProcWirelessNotApplicable = errors.New("network-monitoring/probe/wifi: /proc/net/wireless lists no wlan*")

// ParseProcWireless extracts a Sample from the /proc/net/wireless contents.
// Pure function: no syscalls, no helper invocation. This is intentional —
// TC-NM-013e's invariant is that the WiFi sampler does not need root when
// /proc has the data. Callers are expected to read /proc themselves and
// pass the contents in.
//
// Format (kernel docs, 2 header lines + one line per interface):
//
//	Inter-| sta-|   Quality        |   Discarded packets ...
//	 face | tus | link level noise |  nwid  crypt  frag  retry  ...
//	wlan0:    0   70.  -58.    0       0      0     0    12  ...
//
// @aitri-trace FR-ID: FR-028, TC-ID: TC-NM-013e
func ParseProcWireless(content string) (Sample, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		idx := strings.Index(line, ":")
		if idx <= 0 || !strings.HasPrefix(line, "wlan") {
			continue
		}
		fields := strings.Fields(line[idx+1:])
		// fields layout (kernel 4.x+): status quality level noise nwid crypt frag retry misc beacon
		if len(fields) < 8 {
			return Sample{}, errors.New("network-monitoring/probe/wifi: malformed /proc/net/wireless line")
		}
		quality, err := atoiTrimDot(fields[1])
		if err != nil {
			return Sample{}, err
		}
		level, err := atoiTrimDot(fields[2])
		if err != nil {
			return Sample{}, err
		}
		retry, err := strconv.Atoi(fields[7])
		if err != nil {
			return Sample{}, err
		}
		return Sample{
			Applicable:  true,
			RSSIDBm:     level,
			LinkQuality: quality,
			Retries:     retry,
		}, nil
	}
	return Sample{}, ErrProcWirelessNotApplicable
}

// atoiTrimDot parses ints written like "70." or "-58." in /proc/net/wireless.
// The trailing dot is the kernel's fixed-point indicator; we treat the value
// as an integer because no caller needs the fractional part.
func atoiTrimDot(s string) (int, error) {
	return strconv.Atoi(strings.TrimSuffix(s, "."))
}

// New returns a skeleton WiFi Worker.
func New() Worker { return &skeletonWorker{} }

type skeletonWorker struct{}

func (s *skeletonWorker) Run(context.Context) error { return ErrSkeleton }
func (s *skeletonWorker) Stop() error               { return nil }
