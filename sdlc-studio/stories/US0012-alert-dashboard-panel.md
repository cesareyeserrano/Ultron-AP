# US0012: Alert Dashboard Panel

> **Status:** Draft
> **Epic:** [EP0003: Alerting & Notifications](../epics/EP0003-alerting-and-notifications.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to see active and historical alerts on the dashboard
**So that** I can review what happened and when

## Context

### Persona Reference
**Admin** - Wants a central place to review all alerts
[Full persona details](../personas.md#admin)

### Background
Dedicated alerts page and alert indicators on the main dashboard. Shows real-time alerts via SSE and allows browsing historical alerts with filters.

---

## Acceptance Criteria

### AC1: Alert indicator on dashboard
- **Given** there are unacknowledged critical alerts
- **When** I view the main dashboard
- **Then** a red badge with count appears on the Alerts nav item
- **And** a subtle alert banner appears at the top of the dashboard

### AC2: Alerts page
- **Given** I navigate to the Alerts page
- **When** the page loads
- **Then** I see recent alerts sorted by newest first
- **And** each alert shows: severity icon, type, message, value, timestamp

### AC3: Filter by severity
- **Given** I am on the Alerts page
- **When** I select filter "Critical"
- **Then** only critical alerts are shown

### AC4: Real-time alert notification
- **Given** I am on any page
- **When** a new alert triggers
- **Then** a toast notification appears briefly (5 seconds)
- **And** the alert badge count updates via SSE

### AC5: Acknowledge alert
- **Given** an unacknowledged alert exists
- **When** I click the acknowledge button
- **Then** the alert is marked as acknowledged with current timestamp
- **And** it no longer counts toward the badge

---

## Scope

### In Scope
- Alerts page with list and filters
- Alert badge on sidebar navigation
- Toast notifications for new alerts
- Acknowledge functionality
- SSE integration for real-time updates

### Out of Scope
- Alert rule configuration (US0009)
- Notification channel management (US0009)

---

## Technical Notes

- SSE event type "alert" for new alerts
- HTMX `hx-sse` for badge updates and toast
- Alert list: HTMX `hx-get` with query params for filters
- Acknowledge: HTMX `hx-post` inline

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| No alerts exist | Show "No alerts" message with checkmark icon |
| Hundreds of alerts | Paginated (20 per page) |
| SSE disconnects | Toast stops, badge freezes, HTMX auto-reconnects |
| Acknowledge fails (DB error) | Show error toast, alert remains unacknowledged |
| Alert arrives while on Alerts page | Added to top of list automatically |

---

## Test Scenarios

- [ ] Alert badge shows correct count
- [ ] Alerts page lists recent alerts
- [ ] Severity filter works
- [ ] New alert shows toast notification
- [ ] Acknowledge marks alert correctly
- [ ] Badge count decreases after acknowledge
- [ ] SSE delivers real-time alerts
- [ ] Empty state shown when no alerts
- [ ] Pagination works for many alerts

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0003](US0003-dark-mode-layout.md) | UI | Layout shell | Draft |
| [US0008](US0008-alert-engine.md) | Data | Alert records in SQLite | Draft |
| [US0007](US0007-dashboard-view.md) | UI | SSE infrastructure | Draft |

---

## Estimation

**Story Points:** 5
**Complexity:** Medium

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0003 |
