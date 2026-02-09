# US0004: System Metrics Collector

> **Status:** Draft
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** the system to collect CPU, RAM, Disk, Network, and Temperature metrics periodically
**So that** real-time and historical data is available for the dashboard

## Context

### Persona Reference
**Admin** - Needs to monitor Raspberry Pi health at a glance
[Full persona details](../personas.md#admin)

### Background
Backend collector that runs in a goroutine, gathering system metrics at configurable intervals (default 5s). Stores data in an in-memory ring buffer for 24h history and exposes it for SSE streaming and HTTP endpoints.

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Performance | < 2% CPU sustained | Collection must be efficient |
| PRD | Performance | < 30MB RAM total | Ring buffer must be memory-efficient |

---

## Acceptance Criteria

### AC1: CPU metrics collected
- **Given** the collector is running
- **When** a collection cycle completes
- **Then** CPU usage percentage (0-100) per core and total is stored
- **And** the value matches `top` output within 5% margin

### AC2: RAM metrics collected
- **Given** the collector is running
- **When** a collection cycle completes
- **Then** RAM total, used, available, and percentage are stored in bytes

### AC3: Disk metrics collected
- **Given** the collector is running
- **When** a collection cycle completes
- **Then** for each mounted partition: path, total, used, free, percentage are stored

### AC4: Network metrics collected
- **Given** the collector is running
- **When** a collection cycle completes
- **Then** bytes sent/received per second are calculated per network interface

### AC5: Temperature collected
- **Given** the collector is running on a Raspberry Pi
- **When** a collection cycle completes
- **Then** CPU temperature in Celsius is stored
- **And** if temperature sensor is unavailable, value is null (not an error)

### AC6: Ring buffer stores 24h history
- **Given** the collector runs for 24 hours at 5s intervals
- **When** I query history
- **Then** I get up to 17,280 data points (24h * 60min * 12/min)
- **And** data older than 24h is automatically evicted

### AC7: Collection interval is configurable
- **Given** ULTRON_METRICS_INTERVAL=10
- **When** the collector starts
- **Then** metrics are collected every 10 seconds

---

## Scope

### In Scope
- Metrics collector goroutine
- gopsutil integration for CPU, RAM, Disk, Network
- Temperature reading (gopsutil or /sys/class/thermal fallback)
- In-memory ring buffer with configurable retention
- System uptime tracking
- Thread-safe access to metrics data

### Out of Scope
- Dashboard display (US0007)
- SSE streaming (US0007)
- Alert evaluation (US0011)
- Persistent storage of metrics (in-memory only)

---

## Technical Notes

- Use `github.com/shirou/gopsutil/v3` for cross-platform metrics
- Ring buffer: fixed-size slice with head/tail pointers, mutex-protected
- Temperature fallback: read `/sys/class/thermal/thermal_zone0/temp` on ARM Linux
- Expose via internal Go interface, not HTTP (US0007 adds HTTP/SSE layer)

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Temperature sensor not available | Return null, log warning once, continue collecting other metrics |
| Network interface disappears | Remove from active interfaces, no error |
| New network interface appears | Add to collection automatically |
| Disk unmounted mid-collection | Skip that partition, log warning |
| gopsutil returns error | Log error, return last known value for that metric |
| System clock change (NTP sync) | Ring buffer uses monotonic timestamps |
| Very high collection frequency (1s) | Warn if CPU overhead exceeds 2% |

---

## Test Scenarios

- [ ] CPU percentage is collected and within valid range (0-100)
- [ ] RAM values are collected (total > 0, used <= total)
- [ ] Disk metrics include at least root partition
- [ ] Network bytes per second calculated correctly
- [ ] Temperature returns value or null (no error)
- [ ] Ring buffer evicts data older than retention period
- [ ] Ring buffer is thread-safe under concurrent access
- [ ] Custom interval is respected
- [ ] Uptime is reported correctly

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0001](US0001-project-scaffolding.md) | Infrastructure | Server, config | Draft |

---

## Estimation

**Story Points:** 5
**Complexity:** Medium

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0002 |
