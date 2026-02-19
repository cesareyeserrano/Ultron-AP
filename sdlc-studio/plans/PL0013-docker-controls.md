# PL0013: Docker Container Controls - Implementation Plan

> **Status:** Complete
> **Story:** [US0013: Docker Container Controls](../stories/US0013-docker-controls.md)
> **Epic:** [EP0004: Service Controls](../epics/EP0004-service-controls.md)
> **Created:** 2026-02-19
> **Language:** Go

## Overview

Add Start, Stop, Restart control buttons to the Docker containers page. Operations use Docker SDK, require CSRF + auth, show confirmation modal for destructive actions, return HTMX partial responses, and log every action to the existing ActionLog table.

## Acceptance Criteria Summary

| AC | Name | Description |
|----|------|-------------|
| AC1 | Start container | Start a stopped container with success feedback and audit log |
| AC2 | Stop with confirmation | Modal before stop; confirmed stop + feedback + log |
| AC3 | Restart with confirmation | Modal before restart; confirmed restart + feedback + log |
| AC4 | Button states | Start disabled when running, Stop/Restart disabled when stopped |
| AC5 | Error handling | UI error feedback + failure logged |

---

## Technical Context

### Language & Framework
- **Primary Language:** Go 1.22
- **Framework:** net/http + HTMX 1.x
- **Test Framework:** testing (stdlib) + httptest

### Existing Patterns
- CSRF: `s.validateCSRF(w, r)` — used in all POST handlers
- Auth: `s.requireAuth(...)` middleware — wraps all protected routes
- HTMX partial: handler returns HTML fragment; template via `s.renderPartial()`
- ActionLog table: already in schema (`id, user_id, action, target, result, details, created_at`)
- Docker client: `DockerClient` interface in `internal/docker/client.go`
- Monitor: `s.docker *docker.Monitor` available on Server

---

## Implementation Tasks

| # | Task | File | Status |
|---|------|------|--------|
| 1 | Extend DockerClient interface with control methods | `internal/docker/client.go` | [ ] |
| 2 | Add Start/Stop/Restart methods to Monitor | `internal/docker/controls.go` | [ ] |
| 3 | Add LogAction DB method | `internal/database/actions.go` | [ ] |
| 4 | Replace /docker placeholder with full page handler + control API handlers | `internal/server/handlers_docker.go` | [ ] |
| 5 | Docker page template with control buttons + modal | `web/templates/docker.html` | [ ] |
| 6 | Container list partial (HTMX swap target) | `web/templates/partials/docker-list.html` | [ ] |
| 7 | Wire new API routes | `internal/server/server.go` | [ ] |
| 8 | Tests for control handlers | `internal/server/handlers_docker_test.go` | [ ] |

---

## Implementation Phases

### Phase 1: Docker SDK + DB Layer
**Goal:** Add control operations to DockerClient interface and Monitor; add LogAction to DB

- [ ] Add `ContainerStart`, `ContainerStop`, `ContainerRestart` to `DockerClient` interface
- [ ] Create `internal/docker/controls.go` with `Monitor.Start()`, `Stop()`, `Restart()` methods
- [ ] Create `internal/database/actions.go` with `LogAction(userID *int64, action, target, result, details string) error`

### Phase 2: HTTP Handlers + Templates
**Goal:** Full docker page with controls and HTMX integration

- [ ] Replace `handlePlaceholderPage("Docker", "docker")` with `handleDockerPage` in `handlers_docker.go`
- [ ] Add `POST /api/docker/{id}/start`, `/stop`, `/restart` handlers with CSRF + auth + audit log
- [ ] Create `web/templates/docker.html` — container list page with control buttons, state-based disable
- [ ] Create `web/templates/partials/docker-list.html` — container rows with Start/Stop/Restart, HTMX confirm for stop/restart
- [ ] Update `internal/server/server.go` routes

### Phase 3: Testing & Validation
**Goal:** Cover all ACs with handler tests using mock DockerClient

| AC | Verification Method | Status |
|----|---------------------|--------|
| AC1 | `TestDockerStart_StartsContainer`, `TestDockerStart_RequiresAuth` | Pending |
| AC2 | `TestDockerStop_StopsContainer`, `TestDockerStop_RequiresCSRF` | Pending |
| AC3 | `TestDockerRestart_RestartsContainer` | Pending |
| AC4 | Template rendering (button disabled attrs) | Pending |
| AC5 | `TestDockerStart_ErrorLogged`, `TestDockerStop_NotFound` | Pending |

---

## Edge Case Handling

| # | Edge Case | Handling Strategy | Phase |
|---|-----------|-------------------|-------|
| 1 | Container removed between click and execution | Docker SDK returns error → 404 with "Container not found" message | Phase 2 |
| 2 | Docker daemon not responding | Monitor.Available() check → 503 "Docker daemon unreachable" | Phase 2 |
| 3 | Stop timeout exceeded | Docker default 10s timeout; SDK handles force-kill; log as warning | Phase 2 |
| 4 | User cancels confirmation modal | hx-confirm client-side cancel → no request sent | Phase 2 |
| 5 | Concurrent operations on same container | Docker SDK serialises; second op returns error → logged | Phase 2 |
| 6 | Permission denied | SDK error propagated → "Permission denied" in feedback + log | Phase 2 |

**Coverage:** 6/6 edge cases handled

---

## Definition of Done

- [ ] All 5 ACs implemented
- [ ] Tests written and passing
- [ ] All edge cases handled
- [ ] CSRF protection on all POST endpoints
- [ ] Auth required on all endpoints
- [ ] Every action logged to ActionLog table
- [ ] No linting errors
