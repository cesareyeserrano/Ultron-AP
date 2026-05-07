// Package storm implements the storm-protection cache for Telegram alert
// notifications: when the same rule fires again within FireWindow of its last
// send, the dispatcher should call editMessageText against the cached message
// instead of sending a fresh chat row.
//
// State is in-memory only (no SQLite — explicit no_go_zone item). A janitor
// goroutine evicts entries older than EvictAfter as a safety net; reads also
// check the FireWindow TTL and treat stale entries as absent regardless of
// physical eviction.
//
// @aitri-trace FR-024 NFR-006
package storm

import (
	"sync"
	"time"
)

// FireWindow is the duration during which subsequent fires for the same rule
// collapse into editMessageText updates instead of new sendMessage calls.
const FireWindow = 60 * time.Second

// EvictAfter is the safety-net retention window used by the janitor goroutine.
// It is significantly larger than FireWindow because read paths already guard
// against stale entries; this only protects long-running processes from
// unbounded growth when rules are deleted.
const EvictAfter = 10 * time.Minute

// JanitorPeriod is how often the janitor sweeps. Set to slightly less than the
// retention window so a deleted rule is reclaimed within ~5 min.
const JanitorPeriod = 5 * time.Minute

// Decision describes how the dispatcher should deliver a fire event.
type Decision struct {
	// Send is true ⇒ issue sendMessage; false ⇒ issue editMessageText.
	Send bool
	// EditTarget is the Telegram message_id to edit. Valid only when Send is
	// false.
	EditTarget int64
	// FireCount is the cumulative count of fires in the current window. The
	// renderer should append "(N fires)" to the subject when FireCount >= 2.
	FireCount int
}

type entry struct {
	messageID    int64
	firstFiredAt time.Time
	lastFiredAt  time.Time
	fireCount    int
}

// Cache is a goroutine-safe in-memory store of in-flight fire chat rows.
type Cache struct {
	now     func() time.Time
	mu      sync.Mutex
	entries map[int64]*entry
}

// New constructs an empty Cache. nowFn must be non-nil and may be replaced in
// tests with a controllable clock.
func New(nowFn func() time.Time) *Cache {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &Cache{
		now:     nowFn,
		entries: make(map[int64]*entry),
	}
}

// Decide returns the dispatcher's send-vs-edit decision for a fresh fire of
// ruleID. It does NOT mutate the cache; the caller must call RecordSend after
// a successful Telegram send.
//
// @aitri-trace FR-024 AC-024-001
func (c *Cache) Decide(ruleID int64) Decision {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[ruleID]
	if !ok {
		return Decision{Send: true, FireCount: 1}
	}
	if c.now().Sub(e.lastFiredAt) > FireWindow {
		// Window expired — the next send is a new chat row.
		return Decision{Send: true, FireCount: 1}
	}
	return Decision{Send: false, EditTarget: e.messageID, FireCount: e.fireCount + 1}
}

// RecordSend remembers that a sendMessage for ruleID succeeded with the given
// Telegram message_id. Called once per fresh chat row.
func (c *Cache) RecordSend(ruleID, messageID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.entries[ruleID] = &entry{
		messageID:    messageID,
		firstFiredAt: now,
		lastFiredAt:  now,
		fireCount:    1,
	}
}

// RecordEdit increments the in-window fire counter and updates lastFiredAt.
// Called once per successful editMessageText call.
func (c *Cache) RecordEdit(ruleID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[ruleID]; ok {
		e.fireCount++
		e.lastFiredAt = c.now()
	}
}

// Clear drops the cache entry for ruleID. Called on resolve events and on
// edit failures (e.g. Telegram returns 400 "message to edit not found").
func (c *Cache) Clear(ruleID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, ruleID)
}

// Snapshot exposes the cached state for a rule for tests and metrics. Returns
// (messageID, fireCount, ok).
func (c *Cache) Snapshot(ruleID int64) (int64, int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[ruleID]
	if !ok {
		return 0, 0, false
	}
	return e.messageID, e.fireCount, true
}

// Sweep removes entries whose lastFiredAt is older than EvictAfter. Returns
// the number of entries evicted. Intended to be called by a long-running
// janitor goroutine — see RunJanitor.
func (c *Cache) Sweep() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := c.now().Add(-EvictAfter)
	evicted := 0
	for k, e := range c.entries {
		if e.lastFiredAt.Before(cutoff) {
			delete(c.entries, k)
			evicted++
		}
	}
	return evicted
}

// RunJanitor blocks until ctx is done or stop is closed, periodically
// invoking Sweep. Spawn it once per process from the dispatcher.
func (c *Cache) RunJanitor(stop <-chan struct{}) {
	ticker := time.NewTicker(JanitorPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.Sweep()
		}
	}
}
