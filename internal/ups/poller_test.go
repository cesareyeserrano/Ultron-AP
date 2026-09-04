package ups

import (
	"context"
	"errors"
	"testing"
	"time"
)

// onceThenFail succeeds on the first poll, then always errors — simulating a
// UPS that was reachable and then lost communication.
type onceThenFail struct{ n int }

func (c *onceThenFail) List(ctx context.Context) (map[string]string, error) {
	c.n++
	if c.n == 1 {
		return mockVars("OL"), nil
	}
	return nil, errors.New("down")
}
func (c *onceThenFail) Close() error { return nil }

// TC-UPS-002e (FR-016): client marks the UPS unreachable after the timeout, with
// backoff, and never panics.
func TestTC_UPS_002e_UnreachableAfterTimeout(t *testing.T) {
	// @aitri-tc TC-UPS-002e
	cfg := Config{
		PollInterval:       10 * time.Millisecond,
		UnreachableTimeout: 50 * time.Millisecond,
		BattLowV:           21.0,
		BattHighV:          27.4,
	}
	p := NewPoller(&onceThenFail{}, cfg)

	// Injected clock so the test controls "elapsed since last good poll".
	now := time.Unix(1000, 0)
	p.now = func() time.Time { return now }

	ctx := context.Background()

	// Poll 1: success at t=1000 → online, lastGood recorded.
	p.pollOnce(ctx)
	if got := p.Current().State; got != StateOnline {
		t.Fatalf("after first poll State = %q, want online", got)
	}

	// Poll 2 at t=1000+10ms: failure, but within the 50ms timeout → still online.
	now = now.Add(10 * time.Millisecond)
	p.pollOnce(ctx)
	if got := p.Current().State; got == StateUnreachable {
		t.Fatalf("went unreachable before timeout elapsed")
	}

	// Poll 3 at t=1000+60ms: failure past the timeout → unreachable.
	now = now.Add(50 * time.Millisecond)
	p.pollOnce(ctx)
	if got := p.Current().State; got != StateUnreachable {
		t.Fatalf("after timeout State = %q, want unreachable", got)
	}
	if got := p.Current().Reachable; got {
		t.Fatal("Reachable = true, want false when unreachable")
	}
	if p.Reconnects() < 2 {
		t.Fatalf("reconnect attempts = %d, want >= 2", p.Reconnects())
	}

	// Backoff must be non-decreasing in the failure count.
	p.failures = 1
	d1 := p.currentBackoff()
	p.failures = 3
	d3 := p.currentBackoff()
	if d3 < d1 {
		t.Fatalf("backoff decreased: f=1 -> %s, f=3 -> %s", d1, d3)
	}
}

// TC-UPS-036f (NFR-016): a panic in the poll path is contained and does not
// crash the process; the snapshot degrades to unreachable.
type panicClient struct{}

func (panicClient) List(ctx context.Context) (map[string]string, error) { panic("boom") }
func (panicClient) Close() error                                        { return nil }

func TestTC_UPS_036f_PanicContained(t *testing.T) {
	// @aitri-tc TC-UPS-036f
	p := NewPoller(panicClient{}, Config{BattLowV: 21, BattHighV: 27.4, UnreachableTimeout: time.Millisecond})
	// safePoll must recover — this call returning at all proves containment.
	p.safePoll(context.Background())
	if got := p.Current().State; got != StateUnreachable {
		t.Fatalf("after contained panic State = %q, want unreachable", got)
	}
}
