// Production sweep.Transport: opens an unprivileged SOCK_DGRAM ICMP socket per
// probe via golang.org/x/net/icmp — same kernel path as gatewayprobe.pingICMP,
// gated by net.ipv4.ping_group_range on the host (NFR-011).
//
// @aitri-trace FR-031 NFR-011 BG-018
package sweep

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// DefaultTransport returns the production icmp Transport. The orchestrator
// uses this when no Transport was injected via Config (production path).
func DefaultTransport() Transport { return icmpTransport{} }

type icmpTransport struct{}

// sweepEchoPayload is the echo body sent per probe. The responder echoes it
// back verbatim, so matching it (plus the source address) on read confirms a
// reply belongs to our request rather than a stray/foreign packet (BG-048).
const sweepEchoPayload = "ultron-ap-sweep"

// Probe sends one unprivileged ICMP echo to ip and waits up to timeout for a
// reply. The returned error distinguishes ErrUnprivilegedICMPUnavailable
// (kernel rejected the SOCK_DGRAM open — usually a ping_group_range
// misconfiguration) from per-host failures (timeout, no reply, parse error).
func (icmpTransport) Probe(ctx context.Context, ip string, timeout time.Duration) error {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		// EPERM/EACCES ⇒ kernel rejected the unprivileged ICMP path
		// (ping_group_range misconfigured). Surface a typed error so the
		// orchestrator can fail-closed per NFR-011 AC-002.
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			return fmt.Errorf("%w: %v", ErrUnprivilegedICMPUnavailable, err)
		}
		return fmt.Errorf("listen icmp: %w", err)
	}
	defer conn.Close()

	addr, err := net.ResolveIPAddr("ip4", ip)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", ip, err)
	}

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			Seq:  1,
			Data: []byte(sweepEchoPayload),
		},
	}
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return fmt.Errorf("marshal icmp: %w", err)
	}

	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if _, err := conn.WriteTo(msgBytes, &net.UDPAddr{IP: addr.IP}); err != nil {
		return fmt.Errorf("write icmp: %w", err)
	}

	if err := conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	reply := make([]byte, 1500)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, peer, err := conn.ReadFrom(reply)
		if err != nil {
			return err
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
		if !ok || string(echo.Data) != sweepEchoPayload {
			continue
		}
		return nil
	}
}

