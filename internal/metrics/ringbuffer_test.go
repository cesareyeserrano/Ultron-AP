package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeSnapshot(i int) Snapshot {
	return Snapshot{
		Timestamp: time.Now().Add(time.Duration(i) * time.Second),
		CPU:       CPUMetrics{TotalPercent: float64(i)},
	}
}

func TestRingBuffer_EmptyBuffer(t *testing.T) {
	rb := NewRingBuffer(10)

	assert.Equal(t, 0, rb.Len())
	assert.Nil(t, rb.Latest())
	assert.Nil(t, rb.History(5))
	assert.Nil(t, rb.All())
}

func TestRingBuffer_AddWithinCapacity(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 0; i < 5; i++ {
		rb.Add(makeSnapshot(i))
	}

	assert.Equal(t, 5, rb.Len())
	latest := rb.Latest()
	require.NotNil(t, latest)
	assert.Equal(t, float64(4), latest.CPU.TotalPercent)
}

func TestRingBuffer_AddAtCapacity(t *testing.T) {
	rb := NewRingBuffer(5)

	for i := 0; i < 8; i++ {
		rb.Add(makeSnapshot(i))
	}

	// Should have exactly 5 entries (capacity)
	assert.Equal(t, 5, rb.Len())

	// Latest should be the last added (7)
	latest := rb.Latest()
	require.NotNil(t, latest)
	assert.Equal(t, float64(7), latest.CPU.TotalPercent)

	// All should be [3,4,5,6,7] — oldest evicted
	all := rb.All()
	require.Len(t, all, 5)
	for i, s := range all {
		assert.Equal(t, float64(i+3), s.CPU.TotalPercent)
	}
}

func TestRingBuffer_HistoryOrder(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 0; i < 5; i++ {
		rb.Add(makeSnapshot(i))
	}

	history := rb.History(3)
	require.Len(t, history, 3)
	// Should be [2, 3, 4] — last 3, oldest first
	assert.Equal(t, float64(2), history[0].CPU.TotalPercent)
	assert.Equal(t, float64(3), history[1].CPU.TotalPercent)
	assert.Equal(t, float64(4), history[2].CPU.TotalPercent)
}

func TestRingBuffer_HistoryMoreThanAvailable(t *testing.T) {
	rb := NewRingBuffer(10)

	for i := 0; i < 3; i++ {
		rb.Add(makeSnapshot(i))
	}

	history := rb.History(100)
	require.Len(t, history, 3)
}

func TestRingBuffer_WrapAroundOrder(t *testing.T) {
	rb := NewRingBuffer(3)

	// Add 5 items to a buffer of 3
	for i := 0; i < 5; i++ {
		rb.Add(makeSnapshot(i))
	}

	all := rb.All()
	require.Len(t, all, 3)
	// Should be [2, 3, 4]
	assert.Equal(t, float64(2), all[0].CPU.TotalPercent)
	assert.Equal(t, float64(3), all[1].CPU.TotalPercent)
	assert.Equal(t, float64(4), all[2].CPU.TotalPercent)
}

func TestRingBuffer_ConcurrentAccess(t *testing.T) {
	rb := NewRingBuffer(100)
	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rb.Add(makeSnapshot(id*100 + j))
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				rb.Latest()
				rb.History(10)
				rb.Len()
			}
		}()
	}

	wg.Wait()

	// Buffer should be at capacity after 1000 writes
	assert.Equal(t, 100, rb.Len())
}

func TestRingBuffer_SingleEntry(t *testing.T) {
	rb := NewRingBuffer(5)
	rb.Add(makeSnapshot(42))

	assert.Equal(t, 1, rb.Len())
	assert.Equal(t, float64(42), rb.Latest().CPU.TotalPercent)

	all := rb.All()
	require.Len(t, all, 1)
	assert.Equal(t, float64(42), all[0].CPU.TotalPercent)
}
