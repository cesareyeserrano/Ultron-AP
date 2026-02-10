package auth

import (
	"sync"
	"time"
)

const (
	MaxAttempts    = 5
	LockoutWindow = 15 * time.Minute
)

type attempt struct {
	count   int
	firstAt time.Time
}

type BruteForceTracker struct {
	mu       sync.Mutex
	attempts map[string]*attempt
}

func NewBruteForceTracker() *BruteForceTracker {
	return &BruteForceTracker{
		attempts: make(map[string]*attempt),
	}
}

func (t *BruteForceTracker) RecordFailure(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	a, exists := t.attempts[ip]
	if !exists || time.Since(a.firstAt) > LockoutWindow {
		t.attempts[ip] = &attempt{count: 1, firstAt: time.Now()}
		return
	}
	a.count++
}

func (t *BruteForceTracker) IsLocked(ip string) bool {
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

func (t *BruteForceTracker) Reset(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, ip)
}
