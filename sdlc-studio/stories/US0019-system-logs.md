# US0019: System Log Viewer (On-Demand)

> **Status:** Done
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Created:** 2026-02-21

## User Story
**As an** Admin
**I want** to view system-level logs (journalctl) on demand
**So that** I can troubleshoot OS and service issues without background resource drain

## Acceptance Criteria
- [x] Add a "System Logs" item to the sidebar
- [x] Implement a dropdown to select between: `Ultron-AP`, `Docker Daemon`, and `Kernel (dmesg)`
- [x] Fetch exactly 100 lines only when the "View" button is clicked
- [x] Ensure zero CPU usage when the Logs page is not being viewed
- [x] Use the existing "Terminal Style" UI component for consistency

## Technical Strategy
- Execute `journalctl -u <service> -n 100 --no-pager` via `exec.Command`
- Return as plain text to the frontend
- No persistent streams or background tails to preserve Raspberry Pi CPU/RAM
