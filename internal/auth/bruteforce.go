// Package auth's brute-force tracker counts failed login attempts per
// source IP and locks the IP out after MaxAttempts within LockoutWindow.
//
// Two backing stores are supported:
//
//   - in-memory (NewBruteForceTracker): legacy behaviour, used in tests
//     and dev. State evaporates on restart — fine for tests, NOT for
//     production where it lets an attacker keep retrying forever.
//   - SQLite-backed (NewPersistentBruteForceTracker): production posture.
//     State survives systemctl restart so an attacker cannot reset the
//     accumulated failure count by bouncing the service.
//
// @aitri-trace BG-022 BL-009
package auth

import (
	"log"
	"sync"
	"time"
)

const (
	MaxAttempts   = 5
	LockoutWindow = 15 * time.Minute
)

// Store is the persistence surface used by the tracker. The interface uses
// primitive return tuples instead of a custom type so the auth package and
// the database package stay decoupled — internal/database satisfies this
// interface via its BruteForce* methods without importing auth, and
// internal/server wires the two together at construction time.
//
// Any error returned by these methods is logged at the call-site and
// treated as "no record" so a transient DB failure cannot lock out a
// legitimate operator — fail-open for availability, never fail-closed
// for the legitimate user. Failure cases are still rate-bounded by the
// underlying network/IP topology.
type Store interface {
	// BruteForceLookup returns (count, firstAt, true, nil) when ip has a
	// row, (_, _, false, nil) when the row does not exist, and a non-nil
	// error on a database failure.
	BruteForceLookup(ip string) (count int, firstAt time.Time, found bool, err error)
	// BruteForceRecordFailure increments the count atomically (with window
	// rollover) and returns the post-update record.
	BruteForceRecordFailure(ip string, window time.Duration, now time.Time) (count int, firstAt time.Time, err error)
	BruteForceReset(ip string) error
	BruteForcePruneBefore(cutoff time.Time) (int64, error)
}

type attempt struct {
	count   int
	firstAt time.Time
}

// BruteForceTracker is the public façade. Both in-memory and persistent
// constructors return *BruteForceTracker with different internals.
type BruteForceTracker struct {
	mu       sync.Mutex
	attempts map[string]*attempt // populated only in the in-memory variant
	store    Store               // nil means in-memory mode
}

// NewBruteForceTracker constructs the legacy in-memory tracker. Existing
// tests and dev callers keep working; production should call
// NewPersistentBruteForceTracker instead.
func NewBruteForceTracker() *BruteForceTracker {
	return &BruteForceTracker{
		attempts: make(map[string]*attempt),
	}
}

// NewPersistentBruteForceTracker wraps a Store so failures survive a
// process restart.
func NewPersistentBruteForceTracker(store Store) *BruteForceTracker {
	return &BruteForceTracker{store: store}
}

// RecordFailure increments the failure count for ip. If the existing
// record is older than the lockout window, the count resets to 1.
func (t *BruteForceTracker) RecordFailure(ip string) {
	if t.store != nil {
		if _, _, err := t.store.BruteForceRecordFailure(ip, LockoutWindow, time.Now()); err != nil {
			log.Printf("bruteforce: record failure ip=%s db=%v", ip, err)
		}
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	a, exists := t.attempts[ip]
	if !exists || time.Since(a.firstAt) > LockoutWindow {
		t.attempts[ip] = &attempt{count: 1, firstAt: time.Now()}
		return
	}
	a.count++
}

// IsLocked reports whether ip has hit the failure limit within the window.
func (t *BruteForceTracker) IsLocked(ip string) bool {
	if t.store != nil {
		count, firstAt, found, err := t.store.BruteForceLookup(ip)
		if err != nil {
			log.Printf("bruteforce: lookup ip=%s db=%v (treating as not locked)", ip, err)
			return false
		}
		if !found {
			return false
		}
		if time.Since(firstAt) > LockoutWindow {
			// Stale row — let the periodic pruner remove it; reporting
			// 'not locked' here is correct.
			return false
		}
		return count >= MaxAttempts
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	a, exists := t.attempts[ip]
	if !exists {
		return false
	}
	if time.Since(a.firstAt) > LockoutWindow {
		delete(t.attempts, ip)
		return false
	}
	return a.count >= MaxAttempts
}

// Reset clears the record for ip — called on a successful login so an
// operator who fat-fingered earlier is not held by their own typos.
func (t *BruteForceTracker) Reset(ip string) {
	if t.store != nil {
		if err := t.store.BruteForceReset(ip); err != nil {
			log.Printf("bruteforce: reset ip=%s db=%v", ip, err)
		}
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, ip)
}

// CleanupExpired removes attempts older than the lockout window, bounding
// tracker growth for IPs that never retry. Safe to call from any goroutine.
func (t *BruteForceTracker) CleanupExpired() {
	if t.store != nil {
		cutoff := time.Now().Add(-LockoutWindow)
		if _, err := t.store.BruteForcePruneBefore(cutoff); err != nil {
			log.Printf("bruteforce: prune db=%v", err)
		}
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for ip, a := range t.attempts {
		if now.Sub(a.firstAt) > LockoutWindow {
			delete(t.attempts, ip)
		}
	}
}
