package metrics

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockReader implements Reader for testing.
type mockReader struct {
	callCount atomic.Int64
	cpuValue  float64
}

func (m *mockReader) Read(ctx context.Context) (*Snapshot, error) {
	n := m.callCount.Add(1)
	return &Snapshot{
		Timestamp: time.Now(),
		CPU:       CPUMetrics{TotalPercent: m.cpuValue + float64(n)},
		RAM:       RAMMetrics{Total: 8 * 1024 * 1024 * 1024, Used: 4 * 1024 * 1024 * 1024, Available: 4 * 1024 * 1024 * 1024, Percent: 50.0},
	}, nil
}

func TestCollector_NewCalculatesCapacity(t *testing.T) {
	reader := &mockReader{}
	c := NewCollector(reader, 5*time.Second, 24*time.Hour)

	// 24h / 5s = 17280
	assert.Equal(t, 17280, c.buffer.capacity)
}

func TestCollector_NewMinimumCapacity(t *testing.T) {
	reader := &mockReader{}
	c := NewCollector(reader, 25*time.Hour, 24*time.Hour)

	// Would be 0, but minimum is 1
	assert.Equal(t, 1, c.buffer.capacity)
}

func TestCollector_StartCollectsImmediately(t *testing.T) {
	reader := &mockReader{}
	c := NewCollector(reader, 1*time.Hour, 24*time.Hour)

	ctx := context.Background()
	c.Start(ctx)
	defer c.Stop()

	// Give it a moment to collect the first reading
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, c.Len())
	latest := c.Latest()
	require.NotNil(t, latest)
	assert.Greater(t, latest.CPU.TotalPercent, 0.0)
}

func TestCollector_CollectsAtInterval(t *testing.T) {
	reader := &mockReader{}
	c := NewCollector(reader, 100*time.Millisecond, 1*time.Hour)

	ctx := context.Background()
	c.Start(ctx)

	// Wait for ~3 intervals + initial
	time.Sleep(350 * time.Millisecond)
	c.Stop()

	// Should have 3-5 readings (initial + 2-3 ticks, timing dependent)
	count := c.Len()
	assert.GreaterOrEqual(t, count, 3)
	assert.LessOrEqual(t, count, 6)
}

func TestCollector_StopIsClean(t *testing.T) {
	reader := &mockReader{}
	c := NewCollector(reader, 100*time.Millisecond, 1*time.Hour)

	ctx := context.Background()
	c.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Stop should return without blocking indefinitely
	done := make(chan struct{})
	go func() {
		c.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good — stopped cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return in time")
	}
}

func TestCollector_HistoryReturnsChrono(t *testing.T) {
	reader := &mockReader{}
	c := NewCollector(reader, 50*time.Millisecond, 1*time.Hour)

	ctx := context.Background()
	c.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	c.Stop()

	history := c.History(c.Len())
	require.NotEmpty(t, history)

	// Verify chronological order
	for i := 1; i < len(history); i++ {
		assert.True(t, !history[i].Timestamp.Before(history[i-1].Timestamp),
			"history[%d] timestamp should be >= history[%d]", i, i-1)
	}
}

func TestCollector_LatestReturnsNilWhenEmpty(t *testing.T) {
	reader := &mockReader{}
	c := NewCollector(reader, 1*time.Hour, 24*time.Hour)

	// Don't start — buffer is empty
	assert.Nil(t, c.Latest())
}
