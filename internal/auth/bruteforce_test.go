package auth

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBruteForce_AllowsUnderLimit(t *testing.T) {
	tracker := NewBruteForceTracker()

	for i := 0; i < MaxAttempts-1; i++ {
		tracker.RecordFailure("192.168.1.1")
	}

	assert.False(t, tracker.IsLocked("192.168.1.1"))
}

func TestBruteForce_LocksAtLimit(t *testing.T) {
	tracker := NewBruteForceTracker()

	for i := 0; i < MaxAttempts; i++ {
		tracker.RecordFailure("192.168.1.1")
	}

	assert.True(t, tracker.IsLocked("192.168.1.1"))
}

func TestBruteForce_DifferentIPsIndependent(t *testing.T) {
	tracker := NewBruteForceTracker()

	for i := 0; i < MaxAttempts; i++ {
		tracker.RecordFailure("192.168.1.1")
	}

	assert.True(t, tracker.IsLocked("192.168.1.1"))
	assert.False(t, tracker.IsLocked("192.168.1.2"))
}

func TestBruteForce_ResetClearsLockout(t *testing.T) {
	tracker := NewBruteForceTracker()

	for i := 0; i < MaxAttempts; i++ {
		tracker.RecordFailure("192.168.1.1")
	}
	assert.True(t, tracker.IsLocked("192.168.1.1"))

	tracker.Reset("192.168.1.1")
	assert.False(t, tracker.IsLocked("192.168.1.1"))
}

func TestBruteForce_ExpiresAfterWindow(t *testing.T) {
	tracker := NewBruteForceTracker()

	// Manually set an expired attempt
	tracker.mu.Lock()
	tracker.attempts["192.168.1.1"] = &attempt{
		count:   MaxAttempts,
		firstAt: time.Now().Add(-LockoutWindow - time.Second),
	}
	tracker.mu.Unlock()

	assert.False(t, tracker.IsLocked("192.168.1.1"))
}

func TestBruteForce_UnknownIPNotLocked(t *testing.T) {
	tracker := NewBruteForceTracker()
	assert.False(t, tracker.IsLocked("10.0.0.1"))
}

func TestBruteForce_NewWindowAfterExpiry(t *testing.T) {
	tracker := NewBruteForceTracker()

	// Set an expired attempt
	tracker.mu.Lock()
	tracker.attempts["192.168.1.1"] = &attempt{
		count:   MaxAttempts,
		firstAt: time.Now().Add(-LockoutWindow - time.Second),
	}
	tracker.mu.Unlock()

	// New failure should start a fresh window
	tracker.RecordFailure("192.168.1.1")
	assert.False(t, tracker.IsLocked("192.168.1.1"))
}

func TestBruteForce_CleanupExpired(t *testing.T) {
	tracker := NewBruteForceTracker()

	tracker.mu.Lock()
	tracker.attempts["stale"] = &attempt{
		count:   MaxAttempts,
		firstAt: time.Now().Add(-LockoutWindow - time.Second),
	}
	tracker.attempts["fresh"] = &attempt{
		count:   1,
		firstAt: time.Now(),
	}
	tracker.mu.Unlock()

	tracker.CleanupExpired()

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	_, staleExists := tracker.attempts["stale"]
	_, freshExists := tracker.attempts["fresh"]
	assert.False(t, staleExists)
	assert.True(t, freshExists)
}

// failingStore is a Store whose every method returns an error, used to verify
// the tracker's degraded-state signalling (BL-032).
type failingStore struct{}

func (failingStore) BruteForceLookup(string) (int, time.Time, bool, error) {
	return 0, time.Time{}, false, errBruteForceDB
}
func (failingStore) BruteForceRecordFailure(string, time.Duration, time.Time) (int, time.Time, error) {
	return 0, time.Time{}, errBruteForceDB
}
func (failingStore) BruteForceReset(string) error               { return errBruteForceDB }
func (failingStore) BruteForcePruneBefore(time.Time) (int64, error) { return 0, errBruteForceDB }

var errBruteForceDB = fmt.Errorf("simulated db failure")

// TestBruteForce_DBErrorsSurfaced is a regression test for BL-032: store
// failures must fail open (never lock out a legitimate operator) AND be
// observable via DBErrorCount so a silently-degraded lockout is visible.
func TestBruteForce_DBErrorsSurfaced(t *testing.T) {
	tr := NewPersistentBruteForceTracker(failingStore{})

	assert.Equal(t, int64(0), tr.DBErrorCount())

	// IsLocked must fail open on a lookup error and bump the counter.
	assert.False(t, tr.IsLocked("1.2.3.4"))
	tr.RecordFailure("1.2.3.4")
	tr.Reset("1.2.3.4")

	assert.GreaterOrEqual(t, tr.DBErrorCount(), int64(3),
		"each store failure must increment the surfaced error counter")
}
