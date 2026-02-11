package metrics

import (
	"context"
	"log"
	"sync"
	"time"
)

// Collector periodically reads system metrics and stores them in a ring buffer.
type Collector struct {
	reader   Reader
	buffer   *RingBuffer
	interval time.Duration
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewCollector creates a collector with the given reader, interval, and retention period.
// Buffer capacity is calculated as retention / interval.
func NewCollector(reader Reader, interval time.Duration, retention time.Duration) *Collector {
	capacity := int(retention / interval)
	if capacity < 1 {
		capacity = 1
	}

	return &Collector{
		reader:   reader,
		buffer:   NewRingBuffer(capacity),
		interval: interval,
	}
}

// Start begins collecting metrics in a background goroutine.
func (c *Collector) Start(ctx context.Context) {
	ctx, c.cancel = context.WithCancel(ctx)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.run(ctx)
	}()

	log.Printf("Metrics collector started (interval=%v)", c.interval)
}

// Stop cancels the collection loop and waits for it to exit.
func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	log.Println("Metrics collector stopped")
}

// Latest returns the most recent metrics snapshot.
func (c *Collector) Latest() *Snapshot {
	return c.buffer.Latest()
}

// History returns the last n snapshots in chronological order.
func (c *Collector) History(n int) []Snapshot {
	return c.buffer.History(n)
}

// Len returns the number of stored snapshots.
func (c *Collector) Len() int {
	return c.buffer.Len()
}

func (c *Collector) run(ctx context.Context) {
	// Collect once immediately
	c.collect(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

func (c *Collector) collect(ctx context.Context) {
	snapshot, err := c.reader.Read(ctx)
	if err != nil {
		log.Printf("metrics: collection error: %v", err)
		return
	}
	c.buffer.Add(*snapshot)
}
