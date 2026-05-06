package sweep

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTransport answers from a static reply set. delayPerProbe simulates
// per-host wall-clock cost; sendCount is incremented atomically per call.
type fakeTransport struct {
	replies       map[string]bool // ip → true if it answers
	delayPerProbe time.Duration
	sendCount     int64
}

func (f *fakeTransport) Probe(ctx context.Context, ip string, timeout time.Duration) error {
	atomic.AddInt64(&f.sendCount, 1)
	if f.delayPerProbe > 0 {
		select {
		case <-time.After(f.delayPerProbe):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.replies[ip] {
		return nil
	}
	return errors.New("timeout")
}

// TC-LD-002h
//
// @aitri-tc TC-LD-002h
func TestTC_LD_002h_Sweep_EmitsExactly254AndCompletesUnder3Seconds(t *testing.T) {
	// @aitri-tc TC-LD-002h
	transport := &fakeTransport{
		replies: map[string]bool{
			"192.168.1.1":  true,
			"192.168.1.10": true,
			"192.168.1.50": true,
		},
	}
	cfg := Config{
		CIDR:      "192.168.1.0/24",
		Transport: transport,
		Workers:   32,
		Timeout:   500 * time.Millisecond,
	}
	res, err := Sweep(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, 254, res.Sent)
	assert.Equal(t, int64(254), atomic.LoadInt64(&transport.sendCount))
	assert.Less(t, res.Duration, 3*time.Second)
	assert.ElementsMatch(t, []string{"192.168.1.1", "192.168.1.10", "192.168.1.50"}, res.Responders)
}

// TC-LD-013e — worst case: all 254 hosts respond, still under 3 s wall-clock
//
// @aitri-tc TC-LD-013e
func TestTC_LD_013e_Sweep_WorstCaseAllRespond_Under3s(t *testing.T) {
	// @aitri-tc TC-LD-013e
	replies := map[string]bool{}
	for i := 1; i <= 254; i++ {
		replies[ipFromOctet(i)] = true
	}
	transport := &fakeTransport{
		replies:       replies,
		delayPerProbe: 1 * time.Millisecond, // realistic LAN echo
	}
	cfg := Config{
		CIDR:      "192.168.1.0/24",
		Transport: transport,
		Workers:   32,
		Timeout:   500 * time.Millisecond,
	}
	res, err := Sweep(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, 254, len(res.Responders))
	assert.Less(t, res.Duration, 3*time.Second)
}

// Cancellation: ctx done aborts in-flight cycle.
func TestSweep_ContextCancellation(t *testing.T) {
	transport := &fakeTransport{delayPerProbe: 10 * time.Second} // would block forever
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cfg := Config{
		CIDR:      "192.168.1.0/24",
		Transport: transport,
		Workers:   8,
		Timeout:   1 * time.Second,
	}
	start := time.Now()
	_, err := Sweep(ctx, cfg)
	require.NoError(t, err) // sweep doesn't surface ctx error; aborts cleanly
	assert.Less(t, time.Since(start), 1500*time.Millisecond, "sweep should abort quickly under ctx cancel")
}

// Regression: production callers that omit Transport must not panic.
func TestSweep_NilTransportUsesDefault(t *testing.T) {
	cfg := Config{
		CIDR:       "192.168.1.0/24",
		HostFilter: func(string) bool { return false },
	}
	res, err := Sweep(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Sent)
}

// expandCIDR — refuses /16
func TestExpandCIDR_RefusesWideMask(t *testing.T) {
	_, err := expandCIDR("10.0.0.0/16")
	require.Error(t, err)
}

// expandCIDR — /24 yields 254 hosts (no .0 / .255)
func TestExpandCIDR_Slash24Has254Hosts(t *testing.T) {
	hosts, err := expandCIDR("192.168.1.0/24")
	require.NoError(t, err)
	require.Len(t, hosts, 254)
	assert.Equal(t, "192.168.1.1", hosts[0])
	assert.Equal(t, "192.168.1.254", hosts[253])
}

// expandCIDR — /25 yields 126 hosts
func TestExpandCIDR_Slash25Has126Hosts(t *testing.T) {
	hosts, err := expandCIDR("192.168.1.0/25")
	require.NoError(t, err)
	require.Len(t, hosts, 126)
}

func ipFromOctet(n int) string {
	a := byte(n & 0xff)
	return "192.168.1." + itoa(a)
}

func itoa(b byte) string {
	if b == 0 {
		return "0"
	}
	digits := []byte{}
	x := int(b)
	for x > 0 {
		digits = append([]byte{byte('0' + x%10)}, digits...)
		x /= 10
	}
	return string(digits)
}
