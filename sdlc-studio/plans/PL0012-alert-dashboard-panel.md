# PL0012: Alert Dashboard Panel - Implementation Plan

> **Status:** Complete
> **Story:** [US0012: Alert Dashboard Panel](../stories/US0012-alert-dashboard-panel.md)
> **Epic:** [EP0003: Alerting & Notifications](../epics/EP0003-alerting-and-notifications.md)
> **Created:** 2026-02-11
> **Language:** Go

## Overview

Alerts page with severity filters, acknowledge functionality, sidebar badge with SSE real-time count, and empty state handling.

## Implementation

- `internal/database/alerts.go` — Added AcknowledgeAlert, UnacknowledgedAlertCount, ListAlertsBySeverity
- `internal/server/handlers_alerts.go` — Alerts page handler, acknowledge endpoint, severity filter, alert list partial render
- `web/templates/alerts.html` — Alerts page with severity filter tabs and unack badge
- `web/templates/partials/alerts-list.html` — Alert cards with severity icons, value, timestamp, ack button via HTMX
- `internal/server/sse.go` — Added alert-count SSE event for real-time badge
- `web/templates/partials/sidebar.html` — Alert badge element updated via SSE
- `web/templates/base.html` — EventSource listener for badge updates on all pages
- `internal/server/server.go` — Wired alerts page route and acknowledge API endpoint
