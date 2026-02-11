# PL0005: Docker Container Monitor - Implementation Plan

> **Status:** Draft
> **Story:** [US0005: Docker Container Monitor](../stories/US0005-docker-monitor.md)
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Created:** 2026-02-11
> **Language:** Go

## Overview

Implement a Docker container monitor that connects to the Docker Engine via unix socket, lists all containers (running, stopped, exited) with their state and metrics (CPU%, memory), retrieves container details (ports, volumes, env var names), and refreshes every 10 seconds. Uses the Docker SDK for Go. Handles gracefully when Docker is not installed, the socket is inaccessible, or the daemon is not running.

## Acceptance Criteria Summary

| AC | Name | Description |
|----|------|-------------|
| AC1 | List all containers | All containers listed with name, image, state, status, created time |
| AC2 | Per-container metrics | CPU% and memory (bytes + %) for running containers |
| AC3 | Health indicator | Green=running, grey=stopped, red=exited-with-error |
| AC4 | Container details | Ports, volumes, env var names (no values) |
| AC5 | Auto-refresh 10s | Container list and metrics refresh every 10 seconds |

---

## Technical Context

### Language & Framework
- **Primary Language:** Go 1.25
- **Docker SDK:** github.com/docker/docker/client
- **Test Framework:** Go testing + testify
- **Concurrency:** goroutine + sync.RWMutex (same pattern as metrics collector)

### Existing Patterns
- **Collector pattern:** `internal/metrics/collector.go` — goroutine with ticker, context cancellation, Reader interface
- **Config:** `internal/config/config.go` — env var loading with validation
- **Server struct:** Holds collector reference, passed in `New()`

### Design Decisions

**Docker SDK vs CLI:** Use the Docker SDK (`github.com/docker/docker/client`) for type safety and efficiency. Avoids shelling out to `docker` CLI.

**Monitor as separate package:** `internal/docker/monitor.go` — follows the same pattern as `internal/metrics/`. A `Monitor` struct with `Start(ctx)` / `Stop()` / `Containers()` / `ContainerDetail(id)` methods.

**Container stats approach:** Docker's `ContainerStats` streams data. We read exactly one JSON object per container per cycle, then close the stream. This avoids holding open connections.

**State mapping:** Map Docker container state strings to a health enum: `running` → green, `created`/`paused` → yellow, `exited` (code 0) → grey, `exited` (code != 0) / `dead` → red.

**Env var security:** Show env var names only (e.g., `DATABASE_URL`, `API_KEY`) without their values, to avoid leaking secrets in the UI.

**Graceful degradation:** If Docker socket is unavailable at startup, the monitor logs a warning and returns empty container lists. It retries connecting on each cycle, so if Docker becomes available later, it picks it up automatically.

---

## Recommended Approach

**Strategy:** TDD
**Rationale:** US0005 has clear data contracts (container list, stats, details), 7 edge cases including error handling, and well-defined APIs (Docker SDK). The Monitor interface can be tested with a mock Docker client. Edge cases (Docker not installed, socket inaccessible) are critical to test.

### Test Priority
1. Container listing (names, states, images)
2. Container stats (CPU%, memory)
3. Container details (ports, volumes, env names)
4. Error handling (Docker unavailable, socket permission)
5. Health indicator mapping (state → color)
6. Auto-refresh lifecycle (start/stop)

---

## Implementation Tasks

| # | Task | File | Depends On | Status |
|---|------|------|------------|--------|
| 1 | Add Docker SDK dependency | `go.mod` | - | [ ] |
| 2 | Define Docker data models | `internal/docker/models.go` | - | [ ] |
| 3 | Create Docker client wrapper | `internal/docker/client.go` | 1 | [ ] |
| 4 | Implement container listing | `internal/docker/monitor.go` | 2, 3 | [ ] |
| 5 | Implement container stats | `internal/docker/monitor.go` | 3 | [ ] |
| 6 | Implement container details | `internal/docker/monitor.go` | 3 | [ ] |
| 7 | Implement monitor goroutine (10s refresh) | `internal/docker/monitor.go` | 4, 5 | [ ] |
| 8 | Wire monitor into main.go and server | `cmd/ultron-ap/main.go`, `internal/server/server.go` | 7 | [ ] |
| 9 | Write tests | `internal/docker/*_test.go` | All | [ ] |

### Parallel Execution Groups

| Group | Tasks | Prerequisite |
|-------|-------|--------------|
| A | 1, 2 | None |
| B | 3 | Task 1 |
| C | 4, 5, 6 | Tasks 2, 3 |
| D | 7 | Tasks 4, 5 |
| E | 8 | Task 7 |
| F | 9 | All above |

---

## Implementation Phases

### Phase 1: Dependencies & Data Models
**Goal:** Add Docker SDK, define container data structures

- [ ] `go get github.com/docker/docker`
- [ ] Create `internal/docker/models.go`:
  - `ContainerInfo` — ID, Name, Image, State, Status, Health, CreatedAt, CPU%, MemUsage, MemLimit, MemPercent
  - `ContainerDetail` — Ports, Volumes, EnvVarNames
  - `HealthStatus` type (Running, Stopped, Error, Paused)
  - `PortMapping` — HostPort, ContainerPort, Protocol
  - `VolumeMount` — Source, Destination, Mode

**Files:**
- `go.mod`, `go.sum` — Docker SDK dependency
- `internal/docker/models.go` — Data models

### Phase 2: Docker Client Wrapper
**Goal:** Abstraction over Docker SDK for testability

- [ ] Create `internal/docker/client.go`:
  - `DockerClient` interface with methods: `ListContainers(ctx)`, `GetContainerStats(ctx, id)`, `InspectContainer(ctx, id)`, `Ping(ctx)`
  - `NewDockerClient()` — creates real client from env or defaults to unix socket
  - Handle connection errors gracefully (return nil client + warning)

**Files:**
- `internal/docker/client.go` — Docker client abstraction

### Phase 3: Monitor Implementation
**Goal:** Container listing, stats, details, health mapping

- [ ] Create `internal/docker/monitor.go`:
  - `Monitor` struct with DockerClient, cached containers, RWMutex
  - `refresh(ctx)` — lists containers, fetches stats for running ones
  - `Containers() []ContainerInfo` — returns cached list (thread-safe read)
  - `ContainerDetail(id string) (*ContainerDetail, error)` — on-demand inspect
  - `Available() bool` — whether Docker is reachable
  - Health mapping: `running` → Running, `exited` (code 0) → Stopped, `exited` (code != 0) → Error, `created`/`paused` → Paused

**Files:**
- `internal/docker/monitor.go` — Monitor implementation

### Phase 4: Monitor Goroutine
**Goal:** Background refresh every 10 seconds with context cancellation

- [ ] Add `Start(ctx context.Context)` and `Stop()` methods to Monitor
- [ ] Use `time.NewTicker(10 * time.Second)` for periodic refresh
- [ ] Collect once immediately on start (same pattern as metrics collector)
- [ ] Log warnings on Docker errors, don't crash

**Files:**
- `internal/docker/monitor.go` — Goroutine lifecycle

### Phase 5: Wiring & Integration
**Goal:** Connect Docker monitor to main.go and server

- [ ] Create monitor in `main.go`, start before server
- [ ] Pass monitor to `server.New()`
- [ ] Update Server struct to hold `*docker.Monitor`
- [ ] Update test files that call `server.New()`

**Files:**
- `cmd/ultron-ap/main.go` — Wire Docker monitor
- `internal/server/server.go` — Add monitor field

### Phase 6: Testing & Verification
**Goal:** Comprehensive tests

- [ ] Model tests: health status mapping
- [ ] Monitor tests with mock client: list, stats, details
- [ ] Error handling tests: Docker unavailable, socket permission error
- [ ] Goroutine lifecycle tests: start, refresh, stop
- [ ] Integration tests: monitor with real Docker (if available, skip otherwise)

| AC | Verification Method | File Evidence | Status |
|----|---------------------|---------------|--------|
| AC1 | Test container listing with mock | `monitor_test.go` | Pending |
| AC2 | Test stats retrieval for running containers | `monitor_test.go` | Pending |
| AC3 | Test health status mapping | `monitor_test.go` | Pending |
| AC4 | Test detail retrieval (ports, volumes, env names) | `monitor_test.go` | Pending |
| AC5 | Test ticker-based refresh | `monitor_test.go` | Pending |

---

## Edge Case Handling

| # | Edge Case (from Story) | Handling Strategy | Phase |
|---|------------------------|-------------------|-------|
| 1 | Docker not installed | `NewDockerClient()` returns nil, Monitor.Available() returns false, empty container list | Phase 2 |
| 2 | Docker socket not accessible | Log permission error, Available() returns false, retry each cycle | Phase 2 |
| 3 | Docker daemon not running | Ping fails, log warning, Available() returns false, retry next cycle | Phase 3 |
| 4 | Container created/removed between cycles | Full re-list on each refresh; new containers appear, removed ones disappear | Phase 3 |
| 5 | Container with no name (only ID) | Use truncated ID (first 12 chars) as display name | Phase 3 |
| 6 | 50+ containers | List all without pagination at data layer; UI handles display (future) | Phase 3 |
| 7 | Container stats unavailable (stopped) | Skip stats for non-running containers, show 0 for CPU/memory | Phase 3 |

**Coverage:** 7/7 edge cases handled

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Docker SDK adds significant binary size | Medium | SDK is well-tree-shaken by Go compiler; monitor ~2-3MB extra |
| Stats collection slow with 50 containers | Medium | Collect stats concurrently (goroutine per container, bounded) |
| Docker SDK version compatibility | Low | Use client.WithAPIVersionNegotiation() for compatibility |
| Docker not available on dev machine (macOS) | Low | Tests use mock client; integration tests skip if Docker unavailable |

---

## Definition of Done

- [ ] All 5 acceptance criteria implemented
- [ ] Unit tests written and passing (with mock Docker client)
- [ ] 7/7 edge cases handled
- [ ] Monitor starts and stops cleanly with context
- [ ] Graceful handling when Docker is unavailable
- [ ] Code follows Go conventions (go fmt, go vet clean)
- [ ] No linting errors
- [ ] `go test ./...` passes

---

## Notes

- This story does NOT add UI components. US0007 (Dashboard View with SSE) will consume the Monitor's data and render it in the Docker section of the dashboard.
- Container stats collection uses `ContainerStats` with `stream: false` to get a single snapshot per container per cycle.
- Env var names are shown without values (security). The UI will display e.g., "DATABASE_URL, API_KEY, NODE_ENV" without the actual values.
- The monitor refreshes every 10 seconds (hardcoded for now). This could be made configurable in the future if needed.
- On macOS (development), Docker Desktop exposes the socket differently. The SDK's `client.NewClientWithOpts(client.FromEnv)` handles this automatically.
