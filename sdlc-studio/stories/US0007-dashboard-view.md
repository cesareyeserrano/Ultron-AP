# US0007: Dashboard View with SSE

> **Status:** Done
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** a real-time dashboard showing system metrics, Docker containers, and Systemd services
**So that** I can assess the health of my Raspberry Pi in one glance

## Context

### Persona Reference
**Admin** - Needs instant health overview without SSH
[Full persona details](../personas.md#admin)

### Background
The main dashboard page. Combines system metrics (US0004), Docker data (US0005), and Systemd data (US0006) into a single view with real-time updates via Server-Sent Events. Includes historical charts for CPU and RAM.

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Performance | < 200ms page response | Templates must render fast |
| PRD | Architecture | SSE for real-time | No WebSockets |

---

## Acceptance Criteria

### AC1: System metrics cards
- **Given** I am on the dashboard
- **When** the page loads
- **Then** I see cards for: CPU%, RAM%, Disk%, Network (in/out), Temperature
- **And** each card shows current value with appropriate unit
- **And** temperature card has color indicator (green < 60C, yellow 60-75C, red > 75C)

### AC2: Historical charts
- **Given** the dashboard has been running for > 5 minutes
- **When** I look at the CPU and RAM sections
- **Then** I see line charts showing the last 60 minutes of data
- **And** charts update in real-time as new data arrives

### AC3: SSE real-time updates
- **Given** I am on the dashboard
- **When** the metrics collector produces new data
- **Then** the dashboard updates automatically via SSE without page reload
- **And** HTMX swaps update only the relevant DOM elements

### AC4: Docker section
- **Given** Docker containers exist
- **When** I view the Docker section
- **Then** I see a list of containers with name, image, state, CPU%, memory
- **And** clicking a container expands to show ports, volumes, env var names

### AC5: Systemd section
- **Given** Systemd services exist
- **When** I view the Services section
- **Then** I see a list of services with name, state, since
- **And** I can filter to show only failed services

### AC6: Uptime display
- **Given** the system is running
- **When** I look at the dashboard header area
- **Then** I see system uptime in human-readable format (e.g., "5d 12h 34m")

---

## Scope

### In Scope
- Dashboard page template
- SSE endpoint (/api/sse/metrics)
- HTMX integration for real-time DOM updates
- Metric cards with current values
- Historical line charts (lightweight JS library)
- Docker container list with expandable details
- Systemd service list with filter
- Uptime display

### Out of Scope
- Alert indicators on dashboard (US0012)
- Service control buttons (EP0004)

---

## Technical Notes

- SSE endpoint: single goroutine broadcasts to all connected clients
- HTMX: `hx-sse="connect:/api/sse/metrics"` with `sse-swap` for targeted updates
- Charts: consider uPlot (lightweight, ~35KB) or Chart.js (more features, ~65KB)
- Container expand/collapse: HTMX `hx-get` to load details on demand
- Template partials: metric-card.html, docker-row.html, systemd-row.html

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| SSE connection drops | HTMX auto-reconnects (built-in) |
| No Docker installed | Docker section shows "Docker not available" |
| No Systemd services | Services section shows "No services found" |
| Temperature sensor unavailable | Temperature card shows "--" |
| Browser doesn't support SSE | Fallback to periodic polling via HTMX hx-trigger="every 5s" |
| Multiple browser tabs open | Each gets its own SSE connection |
| Page opened on slow connection | Initial render is static HTML, SSE enhances |

---

## Test Scenarios

- [ ] Dashboard renders with all metric cards
- [ ] SSE endpoint streams data correctly
- [ ] HTMX updates DOM without full page reload
- [ ] Temperature color indicator works (green/yellow/red)
- [ ] Historical charts display data
- [ ] Docker container list renders
- [ ] Container expand/collapse works
- [ ] Systemd service list renders
- [ ] Error filter shows only failed services
- [ ] Uptime displays correctly

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0003](US0003-dark-mode-layout.md) | UI | Layout shell, sidebar | Draft |
| [US0004](US0004-system-metrics-collector.md) | Data | Metrics collector + ring buffer | Draft |
| [US0005](US0005-docker-monitor.md) | Data | Docker container data | Draft |
| [US0006](US0006-systemd-monitor.md) | Data | Systemd service data | Draft |

---

## Estimation

**Story Points:** 8
**Complexity:** High

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0002 |
