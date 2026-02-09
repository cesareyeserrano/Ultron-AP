# US0009: Alert Configuration UI

> **Status:** Draft
> **Epic:** [EP0003: Alerting & Notifications](../epics/EP0003-alerting-and-notifications.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to configure alert rules and notification channels from the web panel
**So that** I can customize thresholds and choose how I get notified without editing config files

## Context

### Persona Reference
**Admin** - Prefers efficient interfaces, but wants web-based config over file editing
[Full persona details](../personas.md#admin)

### Background
Settings page for managing alert rules (create, edit, enable/disable, delete) and configuring notification channels (Telegram, Email). Includes test buttons to verify connectivity.

---

## Acceptance Criteria

### AC1: Alert rules management
- **Given** I navigate to Settings > Alerts
- **When** I view the alert rules page
- **Then** I see all configured rules with: metric, operator, threshold, severity, cooldown, enabled status

### AC2: Create alert rule
- **Given** I click "Add Rule"
- **When** I fill in: metric=CPU, operator=>, threshold=90, severity=critical, cooldown=15min
- **Then** the rule is saved to SQLite and immediately active

### AC3: Edit/disable alert rule
- **Given** an alert rule exists
- **When** I toggle its enabled switch
- **Then** the rule is disabled/enabled without deletion

### AC4: Telegram configuration
- **Given** I navigate to Settings > Notifications > Telegram
- **When** I enter Bot Token and Chat ID and click Save
- **Then** the credentials are saved (token masked in UI after save)
- **And** a "Test" button sends a test message to verify connectivity

### AC5: Email/SMTP configuration
- **Given** I navigate to Settings > Notifications > Email
- **When** I enter SMTP host, port, user, password, from, to and click Save
- **Then** the credentials are saved (password masked)
- **And** a "Test" button sends a test email

### AC6: Mute notifications
- **Given** notifications are active
- **When** I click "Mute" and select a duration (1h, 4h, 24h)
- **Then** no notifications are sent until the mute expires
- **And** a countdown shows remaining mute time

---

## Scope

### In Scope
- Alert rules CRUD (create, read, update, delete)
- Telegram config form (token, chat_id, test)
- Email/SMTP config form (host, port, user, pass, from, to, test)
- Mute/unmute notifications with timer
- All forms use HTMX for inline updates (no full page reload)

### Out of Scope
- The actual sending of notifications (US0010, US0011)
- Alert history viewing (US0012)

---

## Technical Notes

- HTMX forms with `hx-post` and `hx-swap="outerHTML"` for inline feedback
- CSRF tokens on all forms
- Sensitive fields (passwords, tokens): write-only in UI, masked after save
- Store notification config in a separate `NotificationConfig` table or extend AlertConfig

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Invalid threshold (negative, non-numeric) | Validation error: "Threshold must be a positive number" |
| Duplicate rule (same metric + operator + threshold) | Warning: "Similar rule exists", allow creation |
| Telegram test fails (invalid token) | Show error: "Telegram API error: 401 Unauthorized" |
| SMTP test fails (connection timeout) | Show error: "SMTP connection failed: timeout after 10s" |
| Mute expires while page is open | UI updates automatically (HTMX polling or SSE) |

---

## Test Scenarios

- [ ] Alert rules page lists all rules
- [ ] Can create new alert rule
- [ ] Can edit existing rule
- [ ] Can enable/disable rule
- [ ] Can delete rule with confirmation
- [ ] Telegram config saves correctly
- [ ] Telegram test button sends message
- [ ] Email config saves correctly
- [ ] Email test button sends email
- [ ] Mute activates and shows countdown
- [ ] Mute expires correctly
- [ ] Sensitive fields masked after save

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0003](US0003-dark-mode-layout.md) | UI | Layout shell | Draft |
| [US0008](US0008-alert-engine.md) | Data | AlertConfig schema | Draft |

---

## Estimation

**Story Points:** 5
**Complexity:** Medium

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0003 |
