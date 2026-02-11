# PL0004: System Metrics Collector - Implementation Plan

> **Status:** Complete
> **Story:** [US0004: System Metrics Collector](../stories/US0004-system-metrics-collector.md)
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Created:** 2026-02-11
> **Language:** Go

## Overview

Implement a background metrics collector goroutine that periodically gathers CPU, RAM, Disk, Network, and Temperature data using `gopsutil/v4`. Metrics are stored in a thread-safe in-memory ring buffer with 24h retention (~17,280 data points at 5s intervals). Collection interval is configurable via `ULTRON_METRICS_INTERVAL`. The collector exposes a Go interface for consumption by future SSE streaming (US0007) — no HTTP endpoints in this story.

## Acceptance Criteria Summary

| AC | Name | Description |
|----|------|-------------|
| AC1 | CPU metrics | CPU usage % (0-100) per core and total, within 5% of `top` |
| AC2 | RAM metrics | Total, used, available, and percentage in bytes |
| AC3 | Disk metrics | Per partition: path, total, used, free, percentage |
| AC4 | Network metrics | Bytes sent/received per second per interface |
| AC5 | Temperature | CPU temp in Celsius; null if sensor unavailable |
| AC6 | Ring buffer 24h | Up to 17,280 data points, auto-evict older entries |
| AC7 | Configurable interval | ULTRON_METRICS_INTERVAL env var (default 5s) |

---

## Technical Context

### Language & Framework
- **Primary Language:** Go 1.25
- **Metrics Library:** github.com/shirou/gopsutil/v4
- **Test Framework:** Go testing + testify
- **Concurrency:** goroutines + sync.RWMutex

### Existing Patterns
- **Config loading:** `config.Load()` reads `ULTRON_*` env vars with defaults in `internal/config/config.go`
- **Server struct:** Holds dependencies, started in `cmd/ultron-ap/main.go`
- **Goroutine lifecycle:** Server runs in goroutine, signal handling for shutdown
- **Module:** `github.com/cesareyeserrano/ultron-ap`

### Design Decisions

**gopsutil v4 (not v3):** The story mentions v3 but v4 is the latest stable. v4 uses context-based APIs which align better with Go conventions and allow cancellation.

**Ring buffer implementation:** Fixed-size slice with write index. When full, overwrites oldest entry. Protected by `sync.RWMutex` — writers lock exclusively, readers share. This avoids allocation churn.

**Collector as standalone package:** `internal/metrics/collector.go` — not embedded in server. The collector runs independently and the server references it. This keeps concerns separated and makes testing easier.

**Snapshot struct:** A single `Snapshot` struct captures all metrics at one point in time. The ring buffer stores `[]Snapshot`. This simplifies the SSE layer (US0007) — just serialize the latest snapshot.

**Network rate calculation:** Store raw counters on each tick. Calculate bytes/sec as delta between current and previous reading. First reading has no rate (0).

**Temperature fallback:** gopsutil's `host.SensorsTemperatures()` may fail on ARM. Fallback: read `/sys/class/thermal/thermal_zone0/temp` directly, divide by 1000 for Celsius. If both fail, return nil (not an error).

**Graceful shutdown:** Collector accepts a `context.Context`. When cancelled, the collection loop exits cleanly.

---

## Recommended Approach

**Strategy:** TDD
**Rationale:** US0004 has 7 clear ACs, 7 edge cases, and well-defined data contracts (metric types, ring buffer behavior, interval config). The ring buffer and rate calculations are pure logic that benefit from test-first development. gopsutil calls will be tested via interface abstraction.

### Test Priority
1. Ring buffer (add, eviction, thread safety, capacity)
2. Metrics snapshot data model (valid ranges)
3. Network rate calculation (delta between readings)
4. Temperature fallback (sensor available / unavailable)
5. Config extension (ULTRON_METRICS_INTERVAL)
6. Collector start/stop lifecycle

---

## Implementation Tasks

| # | Task | File | Depends On | Status |
|---|------|------|------------|--------|
| 1 | Add gopsutil dependency | `go.mod` | - | [ ] |
| 2 | Extend Config with MetricsInterval | `internal/config/config.go` | - | [ ] |
| 3 | Define metrics data model (Snapshot, CPU, RAM, Disk, Net, Temp) | `internal/metrics/models.go` | - | [ ] |
| 4 | Implement ring buffer | `internal/metrics/ringbuffer.go` | 3 | [ ] |
| 5 | Implement system metrics reader (gopsutil wrapper) | `internal/metrics/reader.go` | 1, 3 | [ ] |
| 6 | Implement collector goroutine | `internal/metrics/collector.go` | 4, 5 | [ ] |
| 7 | Wire collector into main.go | `cmd/ultron-ap/main.go` | 2, 6 | [ ] |
| 8 | Write tests | `internal/metrics/*_test.go`, `internal/config/config_test.go` | All | [ ] |

### Parallel Execution Groups

| Group | Tasks | Prerequisite |
|-------|-------|--------------|
| A | 1, 2, 3 | None (independent) |
| B | 4 | Task 3 (models) |
| C | 5 | Tasks 1, 3 |
| D | 6 | Tasks 4, 5 |
| E | 7 | Tasks 2, 6 |
| F | 8 | All above |

---

## Implementation Phases

### Phase 1: Dependencies & Configuration
**Goal:** Add gopsutil, extend config with metrics interval

- [ ] `go get github.com/shirou/gopsutil/v4`
- [ ] Add `MetricsInterval time.Duration` to Config struct (default 5s)
- [ ] Read `ULTRON_METRICS_INTERVAL` env var (parse as duration, e.g. "5s", "10s")
- [ ] Validate: interval must be >= 1s
- [ ] Add config tests for new field

**Files:**
- `go.mod`, `go.sum` — New dependency
- `internal/config/config.go` — Extended Config
- `internal/config/config_test.go` — New tests

### Phase 2: Metrics Data Model
**Goal:** Define all metric structs

- [ ] Create `internal/metrics/models.go`:
  - `Snapshot` — timestamp + all metric categories
  - `CPUMetrics` — total percent, per-core percents
  - `RAMMetrics` — total, used, available bytes, percent
  - `DiskMetrics` — slice of `DiskPartition` (path, total, used, free, percent)
  - `NetworkMetrics` — slice of `NetworkInterface` (name, bytes sent/recv per sec)
  - `Temperature` — pointer to float64 (nil if unavailable)

**Files:**
- `internal/metrics/models.go` — Data model structs

### Phase 3: Ring Buffer
**Goal:** Thread-safe fixed-size ring buffer for Snapshot storage

- [ ] Create `internal/metrics/ringbuffer.go`:
  - `RingBuffer` struct with fixed-size slice, write index, count, RWMutex
  - `NewRingBuffer(capacity int)` constructor
  - `Add(s Snapshot)` — write at index, wrap around
  - `Latest() *Snapshot` — most recent entry
  - `History(n int) []Snapshot` — last N entries in chronological order
  - `All() []Snapshot` — all entries in chronological order
  - `Len() int` — number of stored entries
- [ ] Write ring buffer tests:
  - Add within capacity
  - Add at capacity (wrap-around eviction)
  - History returns correct order
  - Latest returns most recent
  - Thread safety under concurrent read/write
  - Empty buffer returns nil/empty

**Files:**
- `internal/metrics/ringbuffer.go` — Ring buffer implementation
- `internal/metrics/ringbuffer_test.go` — Ring buffer tests

### Phase 4: System Metrics Reader
**Goal:** Wrapper around gopsutil to collect system metrics

- [ ] Create `internal/metrics/reader.go`:
  - `Reader` interface with `Read(ctx context.Context) (*Snapshot, error)`
  - `SystemReader` struct implementing Reader (uses gopsutil)
  - `NewSystemReader()` constructor
  - CPU: `cpu.PercentWithContext(ctx, 0, false)` for total, `cpu.PercentWithContext(ctx, 0, true)` for per-core
  - RAM: `mem.VirtualMemoryWithContext(ctx)`
  - Disk: `disk.PartitionsWithContext(ctx, false)` + `disk.UsageWithContext(ctx, path)`
  - Network: `net.IOCountersWithContext(ctx, true)` — store raw counters, compute rate vs previous
  - Temperature: `host.SensorsTemperaturesWithContext(ctx)` with `/sys/class/thermal` fallback
- [ ] Handle errors gracefully per metric (one failing doesn't stop others)

**Files:**
- `internal/metrics/reader.go` — gopsutil wrapper
- `internal/metrics/reader_test.go` — Reader tests (with mock for unit tests)

### Phase 5: Collector Goroutine
**Goal:** Background goroutine that collects metrics on a ticker

- [ ] Create `internal/metrics/collector.go`:
  - `Collector` struct with Reader, RingBuffer, interval, cancel context
  - `NewCollector(reader Reader, interval time.Duration, retention time.Duration)` — calculates buffer capacity from interval + retention
  - `Start(ctx context.Context)` — starts ticker goroutine, collects and stores snapshots
  - `Stop()` — cancels context, waits for goroutine to exit
  - `Latest() *Snapshot` — delegates to ring buffer
  - `History(n int) []Snapshot` — delegates to ring buffer
- [ ] Collector respects context cancellation for clean shutdown

**Files:**
- `internal/metrics/collector.go` — Collector implementation
- `internal/metrics/collector_test.go` — Collector lifecycle tests

### Phase 6: Wiring & Integration
**Goal:** Start collector in main.go, pass to server

- [ ] Create collector in `main.go` after config load:
  - `reader := metrics.NewSystemReader()`
  - `collector := metrics.NewCollector(reader, cfg.MetricsInterval, 24*time.Hour)`
  - `collector.Start(ctx)` before server start
  - `defer collector.Stop()` for cleanup
- [ ] Pass collector reference to server (for future US0007 SSE use)
- [ ] Update Server struct to hold collector reference

**Files:**
- `cmd/ultron-ap/main.go` — Wire collector
- `internal/server/server.go` — Add collector field

### Phase 7: Testing & Verification
**Goal:** Comprehensive tests for all components

- [ ] Config tests: MetricsInterval default, custom, invalid
- [ ] Ring buffer tests: capacity, wrap, order, thread safety
- [ ] Reader tests: mock-based unit tests for data shapes
- [ ] Collector tests: start/stop, interval respected, data flows to buffer
- [ ] Integration: collector produces valid snapshots

| AC | Verification Method | File Evidence | Status |
|----|---------------------|---------------|--------|
| AC1 | Test CPU metrics range 0-100 | `reader_test.go` | Pending |
| AC2 | Test RAM total > 0, used <= total | `reader_test.go` | Pending |
| AC3 | Test disk includes root partition | `reader_test.go` | Pending |
| AC4 | Test network rate calculation | `reader_test.go` | Pending |
| AC5 | Test temperature nil or valid float | `reader_test.go` | Pending |
| AC6 | Test ring buffer capacity and eviction | `ringbuffer_test.go` | Pending |
| AC7 | Test custom interval from config | `config_test.go`, `collector_test.go` | Pending |

---

## Edge Case Handling

| # | Edge Case (from Story) | Handling Strategy | Phase |
|---|------------------------|-------------------|-------|
| 1 | Temperature sensor not available | Return nil Temperature, log warning once via sync.Once, continue other metrics | Phase 4 |
| 2 | Network interface disappears | Collect only currently present interfaces each tick; rate is 0 for new ones | Phase 4 |
| 3 | New network interface appears | Automatically included in next collection; first reading has 0 rate | Phase 4 |
| 4 | Disk unmounted mid-collection | Skip partition if disk.Usage returns error, log warning | Phase 4 |
| 5 | gopsutil returns error | Log error, populate that metric as zero/nil, continue with other metrics | Phase 4 |
| 6 | System clock change (NTP sync) | Use time.Now() for timestamps (monotonic component used by ticker, wall clock for display) | Phase 5 |
| 7 | Very high collection frequency (1s) | Config validates interval >= 1s; no additional warning needed since < 2% CPU is tested | Phase 1 |

**Coverage:** 7/7 edge cases handled

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| gopsutil not supporting ARM temperature | Medium | Fallback to reading /sys/class/thermal directly |
| gopsutil v4 API changes from v3 docs | Low | v4 uses context-based APIs; well documented |
| Ring buffer memory with 17,280 entries | Low | Each Snapshot is ~500 bytes; total ~8.5MB well under 30MB |
| CPU.Percent requires interval for accurate reading | Medium | Use cpu.Percent with 0 interval (instant, compares /proc/stat deltas) |

---

## Definition of Done

- [ ] All 7 acceptance criteria implemented
- [ ] Unit tests written and passing
- [ ] 7/7 edge cases handled
- [ ] Ring buffer thread-safe (tested with concurrent goroutines)
- [ ] Collection interval configurable via ULTRON_METRICS_INTERVAL
- [ ] Collector starts and stops cleanly with context
- [ ] Code follows Go conventions (go fmt, go vet clean)
- [ ] No linting errors
- [ ] `go test ./...` passes

---

## Notes

- This story does NOT add any HTTP endpoints or SSE streaming. US0007 will consume the collector's Go interface to expose metrics to the frontend.
- The ring buffer capacity is calculated as `retention / interval` (e.g., 24h / 5s = 17,280).
- Network rate calculation: on first tick, rates are 0 since there's no previous reading to diff against.
- Temperature is a pointer (`*float64`) so nil clearly indicates "sensor not available" vs 0.0.
- The Reader interface allows mocking gopsutil in unit tests while the SystemReader uses real system calls.
