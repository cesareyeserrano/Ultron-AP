package auth

import (
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
