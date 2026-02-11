package metrics

import "sync"

// RingBuffer is a thread-safe fixed-size circular buffer for Snapshot storage.
type RingBuffer struct {
	mu       sync.RWMutex
	data     []Snapshot
	capacity int
	writeIdx int
	count    int
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data:     make([]Snapshot, capacity),
		capacity: capacity,
	}
}

// Add stores a snapshot, overwriting the oldest entry if at capacity.
func (rb *RingBuffer) Add(s Snapshot) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.data[rb.writeIdx] = s
	rb.writeIdx = (rb.writeIdx + 1) % rb.capacity
	if rb.count < rb.capacity {
		rb.count++
	}
}

// Latest returns the most recent snapshot, or nil if empty.
func (rb *RingBuffer) Latest() *Snapshot {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return nil
	}
	idx := (rb.writeIdx - 1 + rb.capacity) % rb.capacity
	s := rb.data[idx]
	return &s
}

// History returns the last n entries in chronological order (oldest first).
func (rb *RingBuffer) History(n int) []Snapshot {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if n > rb.count {
		n = rb.count
	}
	if n == 0 {
		return nil
	}

	result := make([]Snapshot, n)
	start := (rb.writeIdx - n + rb.capacity) % rb.capacity
	for i := 0; i < n; i++ {
		result[i] = rb.data[(start+i)%rb.capacity]
	}
	return result
}

// All returns all stored entries in chronological order.
func (rb *RingBuffer) All() []Snapshot {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	return rb.History(rb.count)
}

// Len returns the number of stored entries.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	return rb.count
}
