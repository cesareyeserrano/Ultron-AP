# US0010: Telegram Notifications

> **Status:** Draft
> **Epic:** [EP0003: Alerting & Notifications](../epics/EP0003-alerting-and-notifications.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to receive critical alerts on Telegram
**So that** I'm immediately aware of problems even when I'm not looking at the dashboard

## Context

### Persona Reference
**Admin** - Wants push notifications for urgent issues
[Full persona details](../personas.md#admin)

### Background
When an alert triggers (from US0008) and Telegram is configured (via US0009), send a formatted message to the configured Telegram chat via Bot API.

---

## Acceptance Criteria

### AC1: Send alert to Telegram
- **Given** Telegram is configured with valid token and chat_id
- **When** a critical or warning alert triggers
- **Then** a Telegram message is sent within 10 seconds
- **And** the message includes: severity emoji, metric/service name, current value, threshold

### AC2: Message format
- **Given** a CPU critical alert (value=95%, threshold=90%)
- **When** the Telegram message is sent
- **Then** the message reads:
  ```
  🔴 CRITICAL: CPU Usage
  Value: 95% (threshold: 90%)
  Host: ultron
  Time: 2026-02-09 14:30:00
  ```

### AC3: Respects mute setting
- **Given** notifications are muted for 1 hour
- **When** an alert triggers
- **Then** no Telegram message is sent

### AC4: Async sending
- **Given** Telegram API is slow (2s response)
- **When** an alert triggers
- **Then** the alert engine is not blocked
- **And** the Telegram message is sent in a background goroutine

---

## Scope

### In Scope
- Telegram Bot API HTTP client (sendMessage endpoint)
- Message formatting with emoji severity indicators
- Background goroutine for async sending
- Respect mute setting
- Retry once on failure (5s delay)

### Out of Scope
- Telegram bot setup (user does this via @BotFather)
- Rich media (photos, inline keyboards)
- Two-way interaction (bot commands)

---

## Technical Notes

- Telegram API: `POST https://api.telegram.org/bot{token}/sendMessage`
- Body: `{"chat_id": "xxx", "text": "...", "parse_mode": "Markdown"}`
- Use standard `net/http` client with 10s timeout
- Send in goroutine with channel-based queue (buffered channel, size 100)

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Invalid token | Log error "Telegram auth failed: 401", alert still saved to DB |
| Invalid chat_id | Log error "Telegram chat not found: 400", alert still saved |
| Telegram API down | Log error, retry once after 5s, then give up |
| Rate limited (429) | Respect Retry-After header, queue message |
| Message too long (>4096 chars) | Truncate message body |
| Network timeout | Log error after 10s timeout |
| Channel buffer full (100 pending) | Drop oldest, log warning |

---

## Test Scenarios

- [ ] Alert triggers Telegram message
- [ ] Message format includes severity, metric, value, threshold
- [ ] Muted notifications are not sent
- [ ] Sending is async (non-blocking)
- [ ] Invalid token handled gracefully
- [ ] Network timeout handled
- [ ] Rate limit respected
- [ ] Retry on first failure

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0008](US0008-alert-engine.md) | Data | Alert events | Draft |
| [US0009](US0009-alert-configuration-ui.md) | Config | Telegram credentials | Draft |

---

## Estimation

**Story Points:** 3
**Complexity:** Low

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0003 |
