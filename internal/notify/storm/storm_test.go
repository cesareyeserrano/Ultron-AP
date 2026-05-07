package storm

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a controllable clock used to drive the cache's TTL semantics
// deterministically. All TCs in this file advance time explicitly rather than
// sleeping.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(initial time.Time) *fakeClock {
	return &fakeClock{t: initial}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

var t0 = time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)

// TestTC_TMU_024h covers FR-024 / AC-024-001: the first fire for a rule
// must yield Decide{Send: true, FireCount: 1}.
//
// @aitri-trace FR-024 US-024 AC-024-001 TC-TMU-024h
func TestTC_TMU_024h_FirstFireYieldsSend(t *testing.T) {
	clk := newFakeClock(t0)
	c := New(clk.Now)

	d := c.Decide(7)

	if !d.Send {
		t.Fatalf("first fire: Send=false; want true")
	}
	if d.FireCount != 1 {
		t.Fatalf("first fire: FireCount=%d; want 1", d.FireCount)
	}
	if d.EditTarget != 0 {
		t.Fatalf("first fire: EditTarget=%d; want 0", d.EditTarget)
	}
	// Cache must remain empty until RecordSend is called.
	if _, _, ok := c.Snapshot(7); ok {
		t.Fatalf("Decide must not mutate cache; entry exists for rule 7")
	}
}

// TestTC_TMU_024e covers FR-024 / AC-024-001: a second fire 45s after the
// first (within the 60s window) must edit, not send, and bump FireCount to 2.
//
// @aitri-trace FR-024 US-024 AC-024-001 TC-TMU-024e
func TestTC_TMU_024e_SecondFireWithinWindowEdits(t *testing.T) {
	clk := newFakeClock(t0)
	c := New(clk.Now)

	c.RecordSend(7, 555)
	clk.Advance(45 * time.Second)

	d := c.Decide(7)

	if d.Send {
		t.Fatalf("fire #2 within window: Send=true; want false (edit)")
	}
	if d.EditTarget != 555 {
		t.Fatalf("fire #2: EditTarget=%d; want 555", d.EditTarget)
	}
	if d.FireCount != 2 {
		t.Fatalf("fire #2: FireCount=%d; want 2", d.FireCount)
	}
}

// TestTC_TMU_024w covers FR-024 / AC-024-002: a fire arriving after the 60s
// window must produce a fresh send and reset the counter.
//
// @aitri-trace FR-024 US-024 AC-024-002 TC-TMU-024w
func TestTC_TMU_024w_FireAfterWindowResets(t *testing.T) {
	clk := newFakeClock(t0)
	c := New(clk.Now)

	c.RecordSend(7, 555)
	clk.Advance(70 * time.Second) // > FireWindow

	d := c.Decide(7)

	if !d.Send {
		t.Fatalf("fire after window: Send=false; want true")
	}
	if d.FireCount != 1 {
		t.Fatalf("fire after window: FireCount=%d; want 1", d.FireCount)
	}
}

// TestTC_TMU_024r covers FR-024 / AC-024-003: a resolve must Clear the cache,
// and a subsequent fire must produce a fresh send.
//
// @aitri-trace FR-024 US-024 AC-024-003 TC-TMU-024r
func TestTC_TMU_024r_ResolveClearsCacheAndNextFireIsFreshSend(t *testing.T) {
	clk := newFakeClock(t0)
	c := New(clk.Now)

	c.RecordSend(7, 555)
	clk.Advance(50 * time.Second)
	c.Clear(7) // resolve

	if _, _, ok := c.Snapshot(7); ok {
		t.Fatalf("Clear: entry still present for rule 7")
	}

	clk.Advance(1 * time.Second)
	d := c.Decide(7)
	if !d.Send {
		t.Fatalf("fire after resolve: Send=false; want true (fresh row)")
	}
	if d.FireCount != 1 {
		t.Fatalf("fire after resolve: FireCount=%d; want 1", d.FireCount)
	}
}

// TestTC_TMU_024_RecordEditIncrementsFireCount confirms RecordEdit updates the
// in-window counter so Decide returns FireCount=3 for fire #3.
//
// @aitri-trace FR-024 US-024 AC-024-001 TC-TMU-024-counter
func TestTC_TMU_024_RecordEditIncrementsFireCount(t *testing.T) {
	clk := newFakeClock(t0)
	c := New(clk.Now)

	c.RecordSend(7, 555)
	clk.Advance(20 * time.Second)
	c.RecordEdit(7) // simulate fire #2 edit
	clk.Advance(20 * time.Second)

	d := c.Decide(7)
	if d.Send {
		t.Fatalf("fire #3 still in window: Send=true; want false")
	}
	if d.FireCount != 3 {
		t.Fatalf("fire #3: FireCount=%d; want 3", d.FireCount)
	}
}

// TestTC_TMU_024_SweepEvictsStaleEntries confirms the janitor's safety net:
// entries older than EvictAfter (10 min) are physically removed.
//
// @aitri-trace FR-024 NFR-006 TC-TMU-024-sweep
func TestTC_TMU_024_SweepEvictsStaleEntries(t *testing.T) {
	clk := newFakeClock(t0)
	c := New(clk.Now)

	c.RecordSend(1, 100)
	c.RecordSend(2, 200)

	clk.Advance(11 * time.Minute) // > EvictAfter
	evicted := c.Sweep()
	if evicted != 2 {
		t.Fatalf("Sweep evicted %d; want 2", evicted)
	}
	if _, _, ok := c.Snapshot(1); ok {
		t.Fatalf("rule 1 still present after Sweep")
	}
	if _, _, ok := c.Snapshot(2); ok {
		t.Fatalf("rule 2 still present after Sweep")
	}
}

// TestTC_TMU_024_SweepKeepsFreshEntries confirms Sweep does not evict entries
// whose lastFiredAt is within the EvictAfter window.
//
// @aitri-trace FR-024 TC-TMU-024-sweep-keep
func TestTC_TMU_024_SweepKeepsFreshEntries(t *testing.T) {
	clk := newFakeClock(t0)
	c := New(clk.Now)

	c.RecordSend(1, 100)
	clk.Advance(2 * time.Minute) // < EvictAfter

	if evicted := c.Sweep(); evicted != 0 {
		t.Fatalf("Sweep evicted %d fresh entries; want 0", evicted)
	}
	if _, _, ok := c.Snapshot(1); !ok {
		t.Fatalf("fresh entry was evicted")
	}
}

// TestTC_TMU_024_ConcurrentDecideAndRecord checks goroutine safety. The
// race-detector flags any data race; the assertions confirm no entry gets
// duplicated or lost under concurrent access.
//
// @aitri-trace FR-024 TC-TMU-024-race
func TestTC_TMU_024_ConcurrentDecideAndRecord(t *testing.T) {
	clk := newFakeClock(t0)
	c := New(clk.Now)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		ruleID := int64(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Decide(ruleID)
			c.RecordSend(ruleID, ruleID*10)
			c.RecordEdit(ruleID)
		}()
	}
	wg.Wait()

	for i := int64(0); i < 50; i++ {
		mid, fc, ok := c.Snapshot(i)
		if !ok {
			t.Errorf("rule %d missing", i)
			continue
		}
		if mid != i*10 {
			t.Errorf("rule %d: messageID=%d; want %d", i, mid, i*10)
		}
		if fc != 2 {
			t.Errorf("rule %d: fireCount=%d; want 2", i, fc)
		}
	}
}
