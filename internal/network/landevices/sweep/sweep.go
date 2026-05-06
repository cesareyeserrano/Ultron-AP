// Package sweep runs an ICMP echo against every host address in a /24 and
// returns the set of IPs that responded within the per-host timeout.
//
// Concurrency is bounded by a worker pool (default 32). Sweep stays fully
// unprivileged: the production Transport opens a single SOCK_DGRAM ICMP
// socket via golang.org/x/net/icmp (same kernel path as gatewayprobe).
//
// @aitri-trace FR-031 US-031 AC-031-001 AC-031-002 TC-LD-002h TC-LD-002f TC-LD-002e
package sweep

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// ErrUnprivilegedICMPUnavailable is returned by the production transport when
// the kernel rejects unprivileged ICMP (ping_group_range misconfigured).
var ErrUnprivilegedICMPUnavailable = errors.New("unprivileged ICMP unavailable")

// Transport probes a single IP. Implementations:
//   - icmpTransport (production) — opens a SOCK_DGRAM ICMP socket per probe.
//   - fakeTransport (tests) — deterministic per-host reply map.
//
// The Probe call must respect ctx and the per-call timeout.
type Transport interface {
	Probe(ctx context.Context, ip string, timeout time.Duration) error
}

// Result is the outcome of a single sweep cycle.
type Result struct {
	StartedAt    time.Time
	FinishedAt   time.Time
	Duration     time.Duration
	Sent         int
	Responders   []string // IPs that replied within timeout, in lexicographic order
	overrunBegun bool     // not exposed; orchestrator tracks overruns
}

// Config controls a sweep cycle.
type Config struct {
	CIDR       string               // e.g. "192.168.1.0/24"
	Transport  Transport            // defaults to production ICMP transport when nil
	Workers    int                  // default 32 if <= 0
	Timeout    time.Duration        // per-host wait; default 1 s
	HostFilter func(ip string) bool // optional; skip IPs returning false
}

// Sweep runs one sweep cycle synchronously. Cancellation via ctx aborts
// the in-flight cycle but only after currently-issued probes finish or hit
// their timeouts (bounded by Timeout).
func Sweep(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Transport == nil {
		cfg.Transport = DefaultTransport()
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 32
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = time.Second
	}
	hosts, err := expandCIDR(cfg.CIDR)
	if err != nil {
		return Result{}, err
	}
	if cfg.HostFilter != nil {
		filtered := hosts[:0]
		for _, ip := range hosts {
			if cfg.HostFilter(ip) {
				filtered = append(filtered, ip)
			}
		}
		hosts = filtered
	}

	res := Result{StartedAt: time.Now()}
	res.Sent = len(hosts)

	var (
		mu         sync.Mutex
		responders []string
	)

	jobs := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := cfg.Transport.Probe(ctx, ip, cfg.Timeout); err == nil {
					mu.Lock()
					responders = append(responders, ip)
					mu.Unlock()
				}
			}
		}()
	}
	for _, h := range hosts {
		select {
		case <-ctx.Done():
		case jobs <- h:
		}
	}
	close(jobs)
	wg.Wait()

	// Stable order (helps tests and downstream consumers)
	sortIPs(responders)
	res.Responders = responders
	res.FinishedAt = time.Now()
	res.Duration = res.FinishedAt.Sub(res.StartedAt)
	return res, nil
}

// expandCIDR returns every host address inside the /24 (or narrower) CIDR.
// For a /24 that's .1 through .254. /25 → .1 through .126, etc.
func expandCIDR(cidr string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	prefixLen, _ := ipnet.Mask.Size()
	if prefixLen < 24 {
		// Caller is expected to pre-clamp via subnet.Detect; defensive.
		return nil, errors.New("sweep refuses CIDR wider than /24")
	}
	first := ipnet.IP.To4()
	if first == nil {
		return nil, errors.New("ipv6 not supported")
	}
	totalHosts := 1 << uint(32-prefixLen)
	if totalHosts < 4 {
		return nil, errors.New("CIDR too small for sweep")
	}
	hosts := make([]string, 0, totalHosts-2)
	// Skip network (.0) and broadcast (last) addresses.
	for i := 1; i < totalHosts-1; i++ {
		ip := make(net.IP, 4)
		copy(ip, first)
		// Add i to the host portion (low bytes).
		ip[3] = first[3] + byte(i&0xff)
		ip[2] = first[2] + byte((i>>8)&0xff)
		ip[1] = first[1] + byte((i>>16)&0xff)
		ip[0] = first[0] + byte((i>>24)&0xff)
		hosts = append(hosts, ip.String())
	}
	return hosts, nil
}

func sortIPs(ips []string) {
	// Sort by 4-byte numeric value so .2 < .10 < .200.
	parse := func(s string) [4]byte {
		ip := net.ParseIP(s).To4()
		var out [4]byte
		if ip != nil {
			copy(out[:], ip)
		}
		return out
	}
	for i := 1; i < len(ips); i++ {
		for j := i; j > 0; j-- {
			a, b := parse(ips[j-1]), parse(ips[j])
			if less4(b, a) {
				ips[j-1], ips[j] = ips[j], ips[j-1]
			} else {
				break
			}
		}
	}
}

func less4(a, b [4]byte) bool {
	for i := 0; i < 4; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
