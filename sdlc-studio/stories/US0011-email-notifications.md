# US0011: Email Notifications

> **Status:** Draft
> **Epic:** [EP0003: Alerting & Notifications](../epics/EP0003-alerting-and-notifications.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to receive critical alerts and daily summaries by email
**So that** I have a persistent record of all incidents

## Context

### Persona Reference
**Admin** - Wants email as a backup notification channel and for record-keeping
[Full persona details](../personas.md#admin)

### Background
Sends email notifications for critical alerts via configured SMTP server. Also includes an optional daily digest email summarizing all alerts from the past 24 hours.

---

## Acceptance Criteria

### AC1: Send critical alert email
- **Given** SMTP is configured with valid credentials
- **When** a critical alert triggers
- **Then** an email is sent to the configured recipient
- **And** the subject is "[Ultron-AP] CRITICAL: {metric/service name}"
- **And** the body includes: severity, description, current value, threshold, timestamp

### AC2: Daily digest
- **Given** SMTP is configured and digest is enabled
- **When** the configured digest time arrives (e.g., 08:00)
- **Then** an email is sent with a summary of all alerts from the past 24 hours
- **And** grouped by severity (critical first, then warning, then info)

### AC3: Respects mute setting
- **Given** notifications are muted
- **When** a critical alert triggers
- **Then** no email is sent (digest still runs if scheduled)

### AC4: Async sending
- **Given** SMTP server is slow to respond
- **When** an alert triggers
- **Then** the email is sent in a background goroutine
- **And** the alert engine is not blocked

---

## Scope

### In Scope
- SMTP client (net/smtp or gomail)
- Critical alert email with HTML body
- Daily digest scheduler (simple ticker)
- Background goroutine for async sending
- Respect mute setting (for real-time alerts, not digest)

### Out of Scope
- HTML email templates with rich formatting
- Attachment support
- Per-alert-type email preferences

---

## Technical Notes

- Use Go standard `net/smtp` with STARTTLS
- Digest scheduler: goroutine with time.Ticker, check current time every minute
- Queue: same pattern as Telegram (buffered channel)
- HTML body with inline CSS (email client compatibility)

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| SMTP auth failure | Log error "SMTP authentication failed", alert saved to DB |
| SMTP connection timeout | Log error after 10s, retry once |
| Invalid recipient email | Log error "Invalid recipient address" |
| No alerts in 24h | Digest email not sent (skip empty digest) |
| Email body too large | Truncate to 50 alerts max in digest |
| SMTP server unavailable | Log error, alert still saved, no retry after 1 failure |

---

## Test Scenarios

- [ ] Critical alert sends email
- [ ] Email subject includes alert type
- [ ] Email body includes value and threshold
- [ ] Daily digest includes all alerts from 24h
- [ ] Empty digest is not sent
- [ ] Muted alerts don't send email
- [ ] SMTP auth failure handled
- [ ] Sending is async

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0008](US0008-alert-engine.md) | Data | Alert events | Draft |
| [US0009](US0009-alert-configuration-ui.md) | Config | SMTP credentials | Draft |

---

## Estimation

**Story Points:** 3
**Complexity:** Low

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0003 |
