// Package gatewayprobe runs a single ICMP echo against the default gateway on
// a fixed cadence and exposes the latest sample via Latest().
//
// MVP scope: one target (default gateway), in-memory snapshot only, no
// persistence, no alerts. Uses unprivileged ICMP sockets (SOCK_DGRAM with
// IPPROTO_ICMP), which on Linux requires net.ipv4.ping_group_range to include
// the running user's GID.
//
// This is the runtime-wired counterpart to features/network-monitoring's
// skeleton ICMP worker (FR-016). The feature subtree lives under internal/
// (Go's internal rule) so it cannot be imported from cmd/; this package
// provides the same behavior at a path the main binary can consume.
package gatewayprobe

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Status is the outcome of the most recent probe attempt.
type Status string

const (
	StatusInit       Status = "init"
	StatusOK         Status = "ok"
	StatusTimeout    Status = "timeout"
	StatusNoGateway  Status = "no-gateway"
	StatusError      Status = "error"
)

// Snapshot is the latest probe result. Always non-nil after construction.
type Snapshot struct {
	Status    Status
	Target    string
	RTT       time.Duration
	RTTMs     float64 // RTT pre-formatted in milliseconds for templates.
	LastProbe time.Time
	LastError string
}

// Probe periodically pings the default gateway and stores the latest sample.
type Probe struct {
	interval time.Duration
	timeout  time.Duration
	snap     atomic.Pointer[Snapshot]
	stop     chan struct{}
	done     chan struct{}
}

// New returns a Probe that will sample every interval. Call Start to begin.
func New(interval time.Duration) *Probe {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	p := &Probe{
		interval: interval,
		timeout:  2 * time.Second,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	p.snap.Store(&Snapshot{Status: StatusInit})
	return p
}

// Latest returns the most recent snapshot. Never nil.
func (p *Probe) Latest() *Snapshot { return p.snap.Load() }

// Start begins the probe loop. Safe to call once.
func (p *Probe) Start(ctx context.Context) {
	go p.run(ctx)
}

// Stop signals the probe loop to exit and waits for it.
func (p *Probe) Stop() {
	select {
	case <-p.stop:
		// already stopped
	default:
		close(p.stop)
	}
	<-p.done
}

func (p *Probe) run(ctx context.Context) {
	defer close(p.done)
	t := time.NewTicker(p.interval)
	defer t.Stop()
	p.probeOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stop:
			return
		case <-t.C:
			p.probeOnce(ctx)
		}
	}
}

func (p *Probe) probeOnce(ctx context.Context) {
	now := time.Now()
	gw, err := DefaultGateway()
	if err != nil {
		p.snap.Store(&Snapshot{
			Status:    StatusNoGateway,
			LastProbe: now,
			LastError: err.Error(),
		})
		return
	}
	rtt, err := pingICMP(ctx, gw, p.timeout)
	if err != nil {
		status := StatusError
		if isTimeout(err) {
			status = StatusTimeout
		}
		p.snap.Store(&Snapshot{
			Status:    status,
			Target:    gw,
			LastProbe: now,
			LastError: err.Error(),
		})
		return
	}
	p.snap.Store(&Snapshot{
		Status:    StatusOK,
		Target:    gw,
		RTT:       rtt,
		RTTMs:     float64(rtt.Microseconds()) / 1000.0,
		LastProbe: now,
	})
}

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

	id := os.Getpid() & 0xffff
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  1,
			Data: []byte("ultron-ap"),
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
		n, _, err := conn.ReadFrom(reply)
		if err != nil {
			return 0, err
		}
		parsed, err := icmp.ParseMessage(1, reply[:n])
		if err != nil {
			return 0, fmt.Errorf("parse icmp reply: %w", err)
		}
		if parsed.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		echo, ok := parsed.Body.(*icmp.Echo)
		if !ok || echo.ID != id {
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
