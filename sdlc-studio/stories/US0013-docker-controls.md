# US0013: Docker Container Controls

> **Status:** Draft
> **Epic:** [EP0004: Service Controls](../epics/EP0004-service-controls.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to start, stop, and restart Docker containers from the dashboard
**So that** I can manage failing services without SSH

## Context

### Persona Reference
**Admin** - Wants one-click service management
[Full persona details](../personas.md#admin)

### Background
Adds control buttons to the Docker container list (US0005). Uses Docker SDK to execute operations. All actions require confirmation and are logged for audit trail.

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Security | Confirmation required for Stop/Restart | Modal before destructive actions |
| PRD | Security | Audit trail | Every action logged to SQLite |

---

## Acceptance Criteria

### AC1: Start container
- **Given** a stopped container "my-service"
- **When** I click the Start button
- **Then** the container starts (no confirmation needed for Start)
- **And** the UI shows success feedback "Container my-service started"
- **And** the action is logged: user=admin, action=start, target=my-service, result=success

### AC2: Stop container with confirmation
- **Given** a running container "my-service"
- **When** I click the Stop button
- **Then** a confirmation modal appears: "Stop container my-service?"
- **And** when I confirm, the container stops
- **And** success feedback shown and action logged

### AC3: Restart container with confirmation
- **Given** a running container "my-service"
- **When** I click the Restart button
- **Then** a confirmation modal appears: "Restart container my-service?"
- **And** when I confirm, the container restarts
- **And** success feedback shown and action logged

### AC4: Button states
- **Given** a container in state "running"
- **When** I view its controls
- **Then** Start button is disabled, Stop and Restart are enabled
- **And** for a stopped container, Start is enabled, Stop and Restart are disabled

### AC5: Error handling
- **Given** a container operation fails
- **When** the error occurs
- **Then** the UI shows error feedback: "Failed to stop my-service: {error message}"
- **And** the failure is logged: result=error, message={error}

---

## Scope

### In Scope
- Start, Stop, Restart buttons per container
- Confirmation modal for Stop/Restart
- Docker SDK calls for container operations
- Success/error feedback (toast or inline)
- Audit log (ActionLog table in SQLite)

### Out of Scope
- Container creation/deletion
- Image management
- Bulk operations
- Container logs

---

## Technical Notes

- Docker SDK: `client.ContainerStart()`, `client.ContainerStop()`, `client.ContainerRestart()`
- HTMX: `hx-post="/api/docker/{id}/start"` with `hx-confirm` for stop/restart
- Async execution in goroutine; result returned via SSE or HTMX swap
- Stop timeout: use Docker default (10s)

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Container removed between click and execution | Error: "Container not found" |
| Docker daemon not responding | Error: "Docker daemon unreachable" |
| Stop timeout exceeded | Container force-killed after 10s, logged as warning |
| User cancels confirmation modal | No action taken |
| Concurrent operations on same container | Second operation waits/errors, no corruption |
| Permission denied | Error: "Permission denied: insufficient privileges" |

---

## Test Scenarios

- [ ] Start button starts a stopped container
- [ ] Stop button shows confirmation modal
- [ ] Confirmed stop actually stops container
- [ ] Restart button shows confirmation modal
- [ ] Confirmed restart restarts container
- [ ] Button states match container state
- [ ] Success feedback displayed
- [ ] Error feedback displayed on failure
- [ ] Action logged to SQLite
- [ ] Cancelled confirmation takes no action

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0005](US0005-docker-monitor.md) | UI | Docker container list | Draft |
| [US0002](US0002-authentication.md) | Auth | Auth middleware | Draft |

---

## Estimation

**Story Points:** 5
**Complexity:** Medium

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0004 |
