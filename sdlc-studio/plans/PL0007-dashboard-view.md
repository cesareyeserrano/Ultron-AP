# PL0007: Dashboard View with SSE - Implementation Plan

> **Status:** Complete
> **Story:** [US0007: Dashboard View with SSE](../stories/US0007-dashboard-view.md)
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Created:** 2026-02-11
> **Language:** Go + HTML/HTMX

## Overview

Implement a real-time dashboard that displays system metrics (CPU, RAM, disk, network, temperature), Docker containers, and systemd services. Uses Server-Sent Events (SSE) with HTMX SSE extension for live updates. Server renders HTML partials that HTMX swaps into the DOM. Historical CPU/RAM charts use server-rendered SVG polylines (no JS chart library). Container details expand on demand via HTMX hx-get.

## Acceptance Criteria Summary

| AC | Name | Description |
|----|------|-------------|
| AC1 | System metrics cards | CPU%, RAM%, Disk%, Network, Temperature with units and color |
| AC2 | Historical charts | SVG line charts for CPU/RAM last 60 minutes, updated via SSE |
| AC3 | SSE real-time updates | Dashboard updates automatically via SSE without page reload |
| AC4 | Docker section | Container list with expand for details (ports, volumes, env names) |
| AC5 | Systemd section | Service list with failed filter |
| AC6 | Uptime display | Human-readable uptime in header |

## Recommended Approach

**Strategy:** Test-After
**Rationale:** UI-heavy story with templates, SSE streaming, and visual components. Test the SSE handler, dashboard data assembly, and rendering logic after implementation.

## Implementation Phases

### Phase 1: HTMX SSE Extension
- Download htmx-ext-sse.js
- Add to web/static/js/

### Phase 2: SSE Broker & Endpoint
- SSE broker with fan-out to multiple clients
- /api/sse/dashboard endpoint
- Sends named events: metrics, docker, systemd

### Phase 3: Dashboard Data & Handlers
- DashboardData struct combining all monitor data
- Template helper functions (formatBytes, tempColor, etc.)
- Docker detail endpoint /api/docker/{id}

### Phase 4: Dashboard Templates
- Update dashboard.html with real metric cards + SSE
- Docker section with expandable rows
- Systemd section with failed filter
- SVG chart partials for CPU/RAM history

### Phase 5: CSS Rebuild
- Rebuild Tailwind CSS with new template classes

### Phase 6: Testing
- SSE endpoint tests
- Dashboard handler tests
- Docker detail endpoint tests
- Template helper tests

## Edge Case Handling

| # | Edge Case | Handling Strategy | Phase |
|---|-----------|-------------------|-------|
| 1 | SSE connection drops | HTMX SSE ext auto-reconnects | Phase 2 |
| 2 | No Docker installed | Docker section shows "Docker not available" | Phase 4 |
| 3 | No Systemd services | Services section shows "No services found" | Phase 4 |
| 4 | Temperature unavailable | Temperature card shows "--" | Phase 4 |
| 5 | Browser doesn't support SSE | Fallback: hx-trigger="every 5s" on metric cards | Phase 4 |
| 6 | Multiple tabs open | Each gets own SSE connection via broker | Phase 2 |
| 7 | Slow connection | Initial render is static HTML, SSE enhances | Phase 3 |

**Coverage:** 7/7 edge cases handled
