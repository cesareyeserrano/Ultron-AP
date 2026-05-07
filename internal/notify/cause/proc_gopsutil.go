// proc_gopsutil.go — production ProcReader implementation backed by
// gopsutil/v4/process. Provides FR-029 top-by-CPU and top-by-RSS data
// for the probable-cause line on resource alerts.
//
// gopsutil computes per-process CPU% as the average over the interval
// between two consecutive Percent() calls. We avoid blocking inside the
// 200ms cause-derivation budget by maintaining a background sampler
// goroutine that refreshes the snapshot every 30 seconds. Cause
// derivation reads the cached snapshot in O(N) where N = active PIDs
// (typically <500 on a Pi) — well under 5ms.
//
// @aitri-trace FR-029 NFR-005
package cause

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// SamplerInterval is how often the background goroutine refreshes the
// process snapshot. 30s is enough to keep "top: ffmpeg (78%)" honest
// without wasting CPU on a Pi.
const SamplerInterval = 30 * time.Second

// ProcessSampler is a goroutine-safe ProcReader that snapshots
// /proc-derived per-process CPU% + RSS in the background.
type ProcessSampler struct {
	mu       sync.RWMutex
	snapshot []ProcSample
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewProcessSampler constructs a sampler in stopped state. Call Start to
// begin the background refresh; Stop to stop. TopProcesses returns the
// most recent snapshot regardless of running/stopped state — first call
// before Start returns an empty slice.
func NewProcessSampler() *ProcessSampler {
	return &ProcessSampler{stop: make(chan struct{})}
}

// Start launches the background sampler. Call once at process startup.
func (p *ProcessSampler) Start() {
	p.refresh() // prime so the first cause derivation has data
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		t := time.NewTicker(SamplerInterval)
		defer t.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-t.C:
				p.refresh()
			}
		}
	}()
}

// Stop gracefully shuts down the sampler.
func (p *ProcessSampler) Stop() {
	close(p.stop)
	p.wg.Wait()
}

// TopProcesses returns the n most-resource-hungry processes along the
// requested axis from the most recent snapshot. Honors ctx cancellation
// (returns whatever has been collected when ctx is done).
func (p *ProcessSampler) TopProcesses(ctx context.Context, axis Axis, n int) ([]ProcSample, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	src := make([]ProcSample, len(p.snapshot))
	copy(src, p.snapshot)
	switch axis {
	case AxisCPU:
		sort.SliceStable(src, func(i, j int) bool { return src[i].CPUPct > src[j].CPUPct })
	case AxisRSS:
		sort.SliceStable(src, func(i, j int) bool { return src[i].RSSMB > src[j].RSSMB })
	}
	if n > 0 && len(src) > n {
		src = src[:n]
	}
	return src, nil
}

// refresh reads gopsutil's process list and recomputes CPU% + RSS for
// every PID. The first call seeds the gopsutil per-process CPU baseline;
// subsequent calls (after ≥ ~100ms elapsed) return real CPU% values.
func (p *ProcessSampler) refresh() {
	procs, err := process.Processes()
	if err != nil {
		return
	}
	out := make([]ProcSample, 0, len(procs))
	for _, pr := range procs {
		comm, err := pr.Name()
		if err != nil || comm == "" {
			continue
		}
		// Percent(0) returns CPU% computed since the last Percent call
		// for this *process.Process instance. The very first call after
		// a sampler restart returns 0; subsequent ticks return real
		// values once the process has accumulated CPU between samples.
		cpuPct, _ := pr.Percent(0)
		mem, _ := pr.MemoryInfo()
		var rssMB int
		if mem != nil {
			rssMB = int(mem.RSS / (1 << 20))
		}
		out = append(out, ProcSample{Comm: comm, CPUPct: cpuPct, RSSMB: rssMB})
	}
	p.mu.Lock()
	p.snapshot = out
	p.mu.Unlock()
}
