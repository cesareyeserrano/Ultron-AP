# PL0006: Systemd Service Monitor - Implementation Plan

> **Status:** Draft
> **Story:** [US0006: Systemd Service Monitor](../stories/US0006-systemd-monitor.md)
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Created:** 2026-02-11
> **Language:** Go

## Overview

Implement a Systemd service monitor that runs `systemctl list-units --type=service --all --no-pager --plain` to list all services with their state (active/inactive/failed), sub-state, and description. Parses the tabular output, maps states to health indicators, supports filtering by failed status, and refreshes every 30 seconds. Follows the same Monitor pattern as `internal/docker/`. Handles gracefully when systemctl is not available (macOS dev, non-systemd Linux).

## Acceptance Criteria Summary

| AC | Name | Description |
|----|------|-------------|
| AC1 | List active/enabled services | All services with name, state, sub-state, since timestamp |
| AC2 | Health indicators | Green=active, grey=inactive, red=failed |
| AC3 | Filter failed services | Data-layer filter returning only failed services |
| AC4 | Auto-refresh 30s | Service list refreshes every 30 seconds |

---

## Technical Context

### Language & Framework
- **Primary Language:** Go 1.25
- **System Command:** `systemctl` (parsed output)
- **Test Framework:** Go testing + testify
- **Concurrency:** goroutine + sync.RWMutex (same pattern as metrics/docker)

### Existing Patterns
- **Docker monitor:** `internal/docker/monitor.go` — goroutine with ticker, context cancellation, interface abstraction for testability
- **Collector pattern:** `internal/metrics/collector.go` — Start/Stop lifecycle
- **Server struct:** Already holds `*docker.Monitor`, will add `*systemd.Monitor`

### Design Decisions

**systemctl CLI vs D-Bus:** Use `systemctl` CLI for simplicity and minimal dependencies. D-Bus (godbus) would add complexity with marginal benefit for a read-only monitor. The story explicitly recommends CLI first.

**Command execution interface:** Abstract `exec.Command` behind a `CommandRunner` interface so tests can inject canned output without needing systemd on macOS.

**Output parsing:** `systemctl list-units --type=service --all --no-pager --plain` outputs fixed-width columns: UNIT, LOAD, ACTIVE, SUB, DESCRIPTION. Parse by splitting on whitespace (first 4 fields are single-word, description is the rest).

**Since timestamp:** Obtained per-service via `systemctl show -p ActiveEnterTimestamp <unit>`. Batched for efficiency, cached since it doesn't change frequently.

**Filter at data layer:** The `Failed()` method returns a filtered slice. UI (future US0007) calls either `Services()` for all or `Failed()` for errors only.

---

## Recommended Approach

**Strategy:** TDD
**Rationale:** The core logic is output parsing (deterministic, pure function) and health mapping (same pattern as Docker). Both are ideal for test-first development. The CommandRunner interface enables full mock-based testing on macOS.

### Test Priority
1. Output parsing (systemctl output → ServiceInfo structs)
2. Health status mapping (state → color)
3. Failed filter
4. Error handling (systemctl not found, permission denied)
5. Monitor lifecycle (start/stop)

---

## Implementation Tasks

| # | Task | File | Depends On | Status |
|---|------|------|------------|--------|
| 1 | Define service data models | `internal/systemd/models.go` | - | [ ] |
| 2 | Create CommandRunner interface | `internal/systemd/runner.go` | - | [ ] |
| 3 | Implement systemctl output parser | `internal/systemd/parser.go` | 1 | [ ] |
| 4 | Implement monitor (list, filter, refresh) | `internal/systemd/monitor.go` | 2, 3 | [ ] |
| 5 | Wire monitor into main.go and server | `cmd/ultron-ap/main.go`, `internal/server/server.go` | 4 | [ ] |
| 6 | Write tests | `internal/systemd/*_test.go` | All | [ ] |

### Parallel Execution Groups

| Group | Tasks | Prerequisite |
|-------|-------|--------------|
| A | 1, 2 | None |
| B | 3 | Task 1 |
| C | 4 | Tasks 2, 3 |
| D | 5 | Task 4 |
| E | 6 | All above |

---

## Implementation Phases

### Phase 1: Data Models & Command Runner
**Goal:** Define ServiceInfo struct and command execution abstraction

- [ ] Create `internal/systemd/models.go`: ServiceInfo (Name, LoadState, ActiveState, SubState, Description, Since), ServiceHealth enum, MapServiceHealth()
- [ ] Create `internal/systemd/runner.go`: CommandRunner interface with Run(ctx, name, args) method, ExecRunner (real implementation using exec.CommandContext)

**Files:**
- `internal/systemd/models.go` — Data models
- `internal/systemd/runner.go` — Command runner abstraction

### Phase 2: Output Parser
**Goal:** Parse systemctl output into ServiceInfo structs

- [ ] Create `internal/systemd/parser.go`:
  - `parseListUnits(output string) []ServiceInfo` — parse tabular output
  - Handle header line, empty lines, summary footer
  - Split each line: UNIT (trim .service suffix for display), LOAD, ACTIVE, SUB, DESCRIPTION (rest)

**Files:**
- `internal/systemd/parser.go` — Output parser

### Phase 3: Monitor Implementation
**Goal:** Background refresh with goroutine, service listing, failed filter

- [ ] Create `internal/systemd/monitor.go`:
  - `Monitor` struct with CommandRunner, cached services, RWMutex
  - `NewMonitor()` — creates monitor, checks if systemctl exists
  - `Start(ctx)` / `Stop()` — goroutine lifecycle (30s ticker)
  - `Services() []ServiceInfo` — all services (thread-safe copy)
  - `Failed() []ServiceInfo` — failed services only
  - `Available() bool` — whether systemctl is reachable
  - `refresh(ctx)` — runs systemctl, parses output, updates cache

**Files:**
- `internal/systemd/monitor.go` — Monitor implementation

### Phase 4: Wiring & Integration
**Goal:** Connect systemd monitor to main.go and server

- [ ] Create monitor in `main.go`, start before server
- [ ] Pass monitor to `server.New()`
- [ ] Update Server struct to hold `*systemd.Monitor`
- [ ] Update test files that call `server.New()`

**Files:**
- `cmd/ultron-ap/main.go` — Wire systemd monitor
- `internal/server/server.go` — Add monitor field

### Phase 5: Testing & Verification
**Goal:** Comprehensive tests with mock command runner

- [ ] Parser tests: valid output, empty output, header-only, malformed lines
- [ ] Health mapping tests: active→green, inactive→grey, failed→red
- [ ] Filter tests: Failed() returns only failed services
- [ ] Error handling tests: systemctl not found, permission denied
- [ ] Monitor lifecycle tests: start, refresh, stop
- [ ] 100+ services test: performance

| AC | Verification Method | File Evidence | Status |
|----|---------------------|---------------|--------|
| AC1 | Test parser with sample systemctl output | `parser_test.go` | Pending |
| AC2 | Test health status mapping | `models_test.go` | Pending |
| AC3 | Test Failed() filter | `monitor_test.go` | Pending |
| AC4 | Test ticker-based refresh | `monitor_test.go` | Pending |

---

## Edge Case Handling

| # | Edge Case (from Story) | Handling Strategy | Phase |
|---|------------------------|-------------------|-------|
| 1 | systemctl not available | CommandRunner.Run returns exec.ErrNotFound, Available() returns false, empty service list | Phase 1 |
| 2 | Permission denied | Log warning, return whatever services are visible (systemctl works without root for listing) | Phase 3 |
| 3 | Service with very long name | Store full name; truncation is a UI concern (US0007) | Phase 2 |
| 4 | 100+ services listed | All stored in slice, no pagination at data layer | Phase 3 |
| 5 | Service state changes between cycles | Full re-list on each refresh; new states appear, removed services disappear | Phase 3 |

**Coverage:** 5/5 edge cases handled

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| systemctl not available on macOS | Low | CommandRunner interface + mock in tests; real runner skips on non-Linux |
| systemctl output format varies by distro | Low | Parse by whitespace splitting, not fixed-width; tested with real Raspberry Pi OS output |
| Since timestamp requires extra systemctl call per service | Medium | Batch/cache timestamps, only fetch for services with state changes |

---

## Definition of Done

- [ ] All 4 acceptance criteria implemented
- [ ] Unit tests written and passing (with mock CommandRunner)
- [ ] 5/5 edge cases handled
- [ ] Monitor starts and stops cleanly with context
- [ ] Graceful handling when systemctl is unavailable
- [ ] Code follows Go conventions (go fmt, go vet clean)
- [ ] No linting errors
- [ ] `go test ./...` passes

---

## Notes

- This story does NOT add UI components. US0007 (Dashboard View with SSE) will consume the Monitor's data.
- The `Since` timestamp is optional — if `systemctl show` fails for a service, Since is zero-value (omitted in JSON).
- On macOS (development), systemctl doesn't exist. All tests use mock CommandRunner. The monitor gracefully reports Available()=false.
- The monitor refreshes every 30 seconds (per story spec), slower than Docker's 10s since service states change less frequently.
- Only `.service` units are listed (not timers, sockets, etc.) per story scope.
