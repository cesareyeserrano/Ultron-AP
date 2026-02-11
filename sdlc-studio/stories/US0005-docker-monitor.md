# US0005: Docker Container Monitor

> **Status:** Planned
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to see the status and metrics of all Docker containers
**So that** I know which services are running, stopped, or unhealthy

## Context

### Persona Reference
**Admin** - Runs multiple Docker containers on the Raspberry Pi
[Full persona details](../personas.md#admin)

### Background
Connects to Docker Engine via unix socket to list containers, their status, and per-container resource usage. Updates periodically (every 10s).

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Scalability | Up to 50 containers | Must handle volume without degradation |
| PRD | Architecture | Docker socket access | Binary needs /var/run/docker.sock access |

---

## Acceptance Criteria

### AC1: List all containers
- **Given** Docker is running with containers
- **When** the Docker monitor collects data
- **Then** all containers are listed (running, stopped, exited)
- **And** each entry includes: name, image, state, status text, created time

### AC2: Per-container metrics
- **Given** a container is running
- **When** the Docker monitor collects data
- **Then** CPU percentage and memory usage (bytes + percentage) are available for that container

### AC3: Container health indicator
- **Given** containers in various states
- **When** the data is displayed
- **Then** running containers show green indicator
- **And** stopped containers show grey indicator
- **And** exited-with-error containers show red indicator

### AC4: Container details
- **Given** a container exists
- **When** I request its details
- **Then** I get: ports mapping, volume mounts, environment variables (names only, no values for security)

### AC5: Auto-refresh every 10 seconds
- **Given** the Docker monitor is running
- **When** 10 seconds pass
- **Then** the container list and metrics are refreshed

---

## Scope

### In Scope
- Docker SDK client initialization
- Container listing with filters
- Per-container stats (CPU%, memory)
- Container detail retrieval (ports, volumes, env names)
- Periodic refresh goroutine

### Out of Scope
- Start/Stop/Restart controls (US0015, EP0004)
- Container logs
- Image management
- Docker Compose awareness

---

## Technical Notes

- Use `github.com/docker/docker/client` SDK
- Connect via unix socket: `/var/run/docker.sock`
- Use `ContainerList` for listing, `ContainerStats` for metrics
- Stats are streamed by Docker; read one snapshot per cycle and close
- Cache container details (ports, volumes) - they don't change often

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Docker not installed | Show "Docker not available" message, don't crash |
| Docker socket not accessible | Log permission error, show warning in UI |
| Docker daemon not running | Show "Docker daemon not responding", retry next cycle |
| Container created/removed between cycles | Update list on next cycle |
| Container with no name (only ID) | Show truncated container ID |
| 50+ containers | All listed, paginated if needed, no performance degradation |
| Container stats unavailable (stopped) | Show "--" for CPU/memory |

---

## Test Scenarios

- [ ] Lists all containers (running + stopped)
- [ ] Shows correct state for each container
- [ ] CPU and memory metrics for running containers
- [ ] Details include ports and volumes
- [ ] Env var names shown without values
- [ ] Handles Docker not installed gracefully
- [ ] Handles Docker socket permission error
- [ ] Updates every 10 seconds
- [ ] Handles 50+ containers without lag

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
