// Package gatewayprobe runs network probes (ICMP echo or DNS resolution)
// against a small list of targets on a fixed cadence and exposes the latest
// sample per target plus rolling jitter and loss metrics.
//
// Uses unprivileged ICMP sockets (SOCK_DGRAM with IPPROTO_ICMP), which on
// Linux requires net.ipv4.ping_group_range to include the running user's GID.
//
// Each target runs its own goroutine and ticker; samples are independent.
// Per-target rolling state holds the last historyWindow samples for loss%
// and an EWMA for jitter. Snapshots() returns one *Snapshot per registered
// target in registration order.
//
// This package is the runtime-wired counterpart to the features/network-monitoring
// skeleton ICMP worker (FR-016). The feature subtree lives under internal/
// (Go's internal rule) so it cannot be imported from cmd/.
package gatewayprobe

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Status is the outcome of the most recent probe attempt.
type Status string

const (
	StatusInit      Status = "init"
	StatusOK        Status = "ok"
	StatusTimeout   Status = "timeout"
	StatusNoGateway Status = "no-gateway"
	StatusError     Status = "error"
)

// Kind identifies the probe backend used against a target.
type Kind string

const (
	KindICMP Kind = "icmp" // unprivileged SOCK_DGRAM ICMP echo
	KindDNS  Kind = "dns"  // DNS resolution against a configured resolver
)

// Tunables for the rolling window. Exported as constants (not yet config) so
// tests can reference them without import surface.
const (
	historyWindow = 20  // sliding window for loss%
	jitterAlpha   = 0.2 // EWMA factor for jitter
)

// Target is one configured probe destination.
//
// For Kind=KindICMP (default if zero):
//   - Host is the destination IP/hostname; if empty, the worker resolves
//     the default IPv4 gateway from /proc/net/route at each iteration.
//
// For Kind=KindDNS:
//   - Host is the resolver address (e.g. "1.1.1.1") — port 53 implied.
//   - QueryName is the FQDN to resolve each iteration. If empty, defaults
//     to "cloudflare.com" (a stable, always-resolvable fixture).
type Target struct {
	Label     string
	Host      string
	Kind      Kind
	QueryName string
}

// Snapshot is the latest probe result for one target. Returned by Snapshots().
type Snapshot struct {
	Label     string
	Kind      Kind
	Status    Status
	Target    string  // resolved IP / resolver address at probe time
	RTT       time.Duration
	RTTMs     float64 // milliseconds, pre-formatted for templates
	JitterMs  float64 // EWMA of |Δ RTT|
	LossPct   float64 // % of failures over the last historyWindow samples (0..100)
	LastProbe time.Time
	LastError string
}

// Sink is the optional persistence callback invoked once per probe iteration
// per target with the snapshot just stored. nil = no persistence.
type Sink func(s Snapshot)

// Probe runs N targets in parallel.
type Probe struct {
	interval time.Duration
	timeout  time.Duration
	sink     Sink
	states   []*targetState
	stop     chan struct{}
	doneOnce sync.Once
	wg       sync.WaitGroup
}

// targetState owns the per-target rolling history and the latest snapshot.
type targetState struct {
	cfg  Target
	snap atomic.Pointer[Snapshot]

	mu sync.Mutex
	// rttHistory is a ring buffer of RTT in ms. NaN signals a failed sample
	// for loss% accounting. Length is fixed at historyWindow once filled.
	rttHistory     []float64
	jitterEWMA     float64
	haveJitterBase bool
	prevRTT        float64
}

// New constructs a multi-target Probe. targets is the order-preserving list of
// destinations to probe. sink may be nil. interval defaults to 5s if <= 0.
func New(interval time.Duration, sink Sink, targets []Target) *Probe {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if len(targets) == 0 {
		// Conservative default: ping the gateway only.
		targets = []Target{{Label: "gateway"}}
	}
	p := &Probe{
		interval: interval,
		timeout:  2 * time.Second,
		sink:     sink,
		stop:     make(chan struct{}),
	}
	for _, t := range targets {
		if t.Kind == "" {
			t.Kind = KindICMP
		}
		ts := &targetState{cfg: t}
		ts.snap.Store(&Snapshot{Label: t.Label, Kind: t.Kind, Status: StatusInit, Target: t.Host})
		p.states = append(p.states, ts)
	}
	return p
}

// Snapshots returns the latest snapshot for each registered target, in
// registration order. The returned pointers are immutable; do not mutate them.
func (p *Probe) Snapshots() []*Snapshot {
	out := make([]*Snapshot, len(p.states))
	for i, st := range p.states {
		out[i] = st.snap.Load()
	}
	return out
}

// Start spawns one goroutine per target. Safe to call once.
func (p *Probe) Start(ctx context.Context) {
	for _, st := range p.states {
		p.wg.Add(1)
		go p.runTarget(ctx, st)
	}
}

// Stop signals all per-target loops to exit and waits for them.
func (p *Probe) Stop() {
	p.doneOnce.Do(func() { close(p.stop) })
	p.wg.Wait()
}

func (p *Probe) runTarget(ctx context.Context, st *targetState) {
	defer p.wg.Done()
	t := time.NewTicker(p.interval)
	defer t.Stop()
	p.probeOnce(ctx, st)
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stop:
			return
		case <-t.C:
			p.probeOnce(ctx, st)
		}
	}
}

func (p *Probe) probeOnce(ctx context.Context, st *targetState) {
	now := time.Now()
	host := st.cfg.Host
	if st.cfg.Kind == KindICMP && host == "" {
		gw, err := DefaultGateway()
		if err != nil {
			snap := &Snapshot{
				Label:     st.cfg.Label,
				Kind:      st.cfg.Kind,
				Status:    StatusNoGateway,
				LastProbe: now,
				LastError: err.Error(),
			}
			st.recordFailure(snap)
			p.publish(st, snap)
			return
		}
		host = gw
	}

	var rtt time.Duration
	var err error
	switch st.cfg.Kind {
	case KindDNS:
		rtt, err = lookupDNS(ctx, host, st.cfg.QueryName, p.timeout)
	default: // KindICMP
		rtt, err = pingICMP(ctx, host, p.timeout)
	}

	if err != nil {
		status := StatusError
		if isTimeout(err) {
			status = StatusTimeout
		}
		snap := &Snapshot{
			Label:     st.cfg.Label,
			Kind:      st.cfg.Kind,
			Status:    status,
			Target:    host,
			LastProbe: now,
			LastError: err.Error(),
		}
		st.recordFailure(snap)
		p.publish(st, snap)
		return
	}

	rttMs := float64(rtt.Microseconds()) / 1000.0
	snap := &Snapshot{
		Label:     st.cfg.Label,
		Kind:      st.cfg.Kind,
		Status:    StatusOK,
		Target:    host,
		RTT:       rtt,
		RTTMs:     rttMs,
		LastProbe: now,
	}
	st.recordSuccess(snap, rttMs)
	p.publish(st, snap)
}

// lookupDNS resolves queryName via the resolver at resolverAddr (port 53
// implied) and returns the wall-clock duration of the lookup. The resolver
// is contacted directly via UDP; we don't fall back to /etc/resolv.conf so
// the measurement reflects the configured target only.
func lookupDNS(ctx context.Context, resolverAddr, queryName string, timeout time.Duration) (time.Duration, error) {
	if queryName == "" {
		queryName = "cloudflare.com"
	}
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: timeout}
			return d.DialContext(ctx, network, resolverAddr+":53")
		},
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	if _, err := r.LookupHost(cctx, queryName); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (p *Probe) publish(st *targetState, snap *Snapshot) {
	st.snap.Store(snap)
	if p.sink != nil {
		p.sink(*snap)
	}
}

// recordFailure appends a NaN to the history and refreshes loss%. Jitter is
// not updated on failure (no RTT to compare against).
func (s *targetState) recordFailure(snap *Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendRTTLocked(math.NaN())
	snap.LossPct = s.lossPctLocked()
	snap.JitterMs = s.jitterEWMA
}

// recordSuccess appends the RTT and refreshes loss% + jitter EWMA.
func (s *targetState) recordSuccess(snap *Snapshot, rttMs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendRTTLocked(rttMs)
	if s.haveJitterBase {
		delta := math.Abs(rttMs - s.prevRTT)
		s.jitterEWMA = jitterAlpha*delta + (1-jitterAlpha)*s.jitterEWMA
	} else {
		s.haveJitterBase = true
		s.jitterEWMA = 0
	}
	s.prevRTT = rttMs
	snap.LossPct = s.lossPctLocked()
	snap.JitterMs = s.jitterEWMA
}

func (s *targetState) appendRTTLocked(v float64) {
	if len(s.rttHistory) < historyWindow {
		s.rttHistory = append(s.rttHistory, v)
		return
	}
	// Shift left by one — historyWindow is small (20) so cost is negligible.
	copy(s.rttHistory, s.rttHistory[1:])
	s.rttHistory[len(s.rttHistory)-1] = v
}

func (s *targetState) lossPctLocked() float64 {
	if len(s.rttHistory) == 0 {
		return 0
	}
	failed := 0
	for _, v := range s.rttHistory {
		if math.IsNaN(v) {
			failed++
		}
	}
	return float64(failed) / float64(len(s.rttHistory)) * 100.0
}

// --- low-level ICMP + gateway lookup (unchanged from BG-008/BG-010) ---

func isTimeout(err error) bool {
	var ne net.Error
	if asNetErr(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

func asNetErr(err error, target *net.Error) bool {
	for err != nil {
		if ne, ok := err.(net.Error); ok {
			*target = ne
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// icmpEchoPayload is the echo body sent by pingICMP. The responder echoes it
// back verbatim, so matching it on read confirms a reply belongs to our request
// (BG-048) rather than a stray/foreign packet delivered to the socket.
const icmpEchoPayload = "ultron-ap"

// pingICMP sends one ICMP echo request and returns the round-trip time.
func pingICMP(ctx context.Context, target string, timeout time.Duration) (time.Duration, error) {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return 0, fmt.Errorf("listen icmp: %w", err)
	}
	defer conn.Close()

	addr, err := net.ResolveIPAddr("ip4", target)
	if err != nil {
		return 0, fmt.Errorf("resolve %s: %w", target, err)
	}

	// On Linux unprivileged ICMP the kernel substitutes Echo.ID with the
	// socket's source port; we leave it zero and don't rely on it on read.
	// We DO match the echoed payload + source on read (BG-048).
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			Seq:  1,
			Data: []byte(icmpEchoPayload),
		},
	}
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return 0, fmt.Errorf("marshal icmp: %w", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return 0, fmt.Errorf("set write deadline: %w", err)
	}
	start := time.Now()
	if _, err := conn.WriteTo(msgBytes, &net.UDPAddr{IP: addr.IP}); err != nil {
		return 0, fmt.Errorf("write icmp: %w", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return 0, fmt.Errorf("set read deadline: %w", err)
	}
	reply := make([]byte, 1500)
	for {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		n, peer, err := conn.ReadFrom(reply)
		if err != nil {
			return 0, err
		}
		// Only accept a reply from the host we probed. The kernel demuxes
		// ping-socket replies by ID, but verifying the source guards against
		// stray/foreign packets delivered to the socket (BG-048).
		if pa, ok := peer.(*net.UDPAddr); ok && !pa.IP.Equal(addr.IP) {
			continue
		}
		parsed, err := icmp.ParseMessage(1, reply[:n])
		if err != nil {
			// A malformed/foreign packet must not fail the probe; keep waiting
			// for a valid reply until the read deadline fires.
			continue
		}
		if parsed.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		// Match the echoed payload to confirm this is the reply to our request.
		echo, ok := parsed.Body.(*icmp.Echo)
		if !ok || string(echo.Data) != icmpEchoPayload {
			continue
		}
		return time.Since(start), nil
	}
}

// DefaultGateway returns the IPv4 address of the default route by reading
// /proc/net/route. Returns an error if no default route is found or on non-Linux
// systems where /proc is unavailable.
func DefaultGateway() (string, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return "", fmt.Errorf("open /proc/net/route: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		dest := fields[1]
		gateway := fields[2]
		flags := fields[3]
		if dest != "00000000" {
			continue
		}
		flagsN, err := strconvParseUint(flags)
		if err != nil {
			continue
		}
		// RTF_UP = 0x0001, RTF_GATEWAY = 0x0002 — both must be set.
		if flagsN&0x0003 != 0x0003 {
			continue
		}
		ip, err := decodeProcGatewayHex(gateway)
		if err != nil {
			continue
		}
		return ip, nil
	}
	return "", fmt.Errorf("no default route in /proc/net/route")
}

// decodeProcGatewayHex converts the little-endian hex IPv4 address from
// /proc/net/route (e.g. "0102A8C0" → "192.168.2.1") into a dotted-quad string.
func decodeProcGatewayHex(s string) (string, error) {
	if len(s) != 8 {
		return "", fmt.Errorf("expected 8 hex chars, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return "", err
	}
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, binary.BigEndian.Uint32(b))
	return ip.String(), nil
}

func strconvParseUint(s string) (uint64, error) {
	var n uint64
	for _, c := range s {
		var d uint64
		switch {
		case c >= '0' && c <= '9':
			d = uint64(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint64(c-'A') + 10
		default:
			return 0, fmt.Errorf("invalid hex digit %q", c)
		}
		n = n*16 + d
	}
	return n, nil
}
