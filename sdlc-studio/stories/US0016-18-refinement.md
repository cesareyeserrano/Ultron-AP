# US0016: Database Backup Export

> **Status:** Done
> **Epic:** [EP0001: Foundation & Auth](../epics/EP0001-foundation-and-auth.md)
> **Created:** 2026-02-21

## User Story
**As an** Admin
**I want** to export a copy of the Ultron-AP database
**So that** I can backup my settings, alerts, and logs before a system wipe

## Acceptance Criteria
- [x] Add a "Download Backup" button in the Settings page
- [x] Clicking the button triggers a download of the `ultron.db` file
- [x] Ensure the file is not corrupted during download (use WAL safe copy if possible)
- [x] Require Admin authentication to access the export endpoint

---

# US0017: Container Log Viewer

> **Status:** Done
> **Epic:** [EP0004: Service Controls](../epics/EP0004-service-controls.md)
> **Created:** 2026-02-21

## User Story
**As an** Admin
**I want** to view the last 100 lines of logs for a Docker container
**So that** I can troubleshoot failures without SSH access

## Acceptance Criteria
- [x] Add a "View Logs" link in the Docker container detail view
- [x] Display logs in a scrollable, monospace modal or section
- [x] Limit output to the most recent 100 lines to preserve memory
- [x] Handle ANSI color codes if possible (or strip them for plain text)

---

# US0018: HTTPS Reverse Proxy Documentation

> **Status:** Done
> **Epic:** [EP0001: Foundation & Auth](../epics/EP0001-foundation-and-auth.md)
> **Created:** 2026-02-21

## User Story
**As an** Admin
**I want** a guide on how to deploy Ultron-AP with HTTPS
**So that** I can access the panel securely over the internet

## Acceptance Criteria
- [x] Create a `DEPLOY.md` file (or add to README)
- [x] Include a Caddyfile example for reverse proxying Ultron-AP
- [x] Document Tailscale HTTPS integration for private VPN access
- [x] Add security best practices for public exposure (WAF, Geo-blocking)
