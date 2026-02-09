# US0008: Alert Engine & Rule Evaluation

> **Status:** Draft
> **Epic:** [EP0003: Alerting & Notifications](../epics/EP0003-alerting-and-notifications.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to define alert rules with thresholds that are evaluated automatically
**So that** I'm notified when system metrics or service states cross dangerous levels

## Context

### Persona Reference
**Admin** - Needs proactive alerts, not constant dashboard watching
[Full persona details](../personas.md#admin)

### Background
The alert engine evaluates configured rules against current metrics on every collection cycle. When a threshold is crossed, an alert record is created in SQLite. Includes cooldown logic to prevent alert spam.

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Performance | Alert eval must not impact collection | Evaluation must be fast and non-blocking |
| PRD | Architecture | SQLite for persistence | Alert history stored in DB |

---

## Acceptance Criteria

### AC1: Metric-based alerts
- **Given** an alert rule exists: CPU > 90% severity=critical
- **When** the CPU metric exceeds 90%
- **Then** an alert record is created with type=cpu, severity=critical, value=current%, threshold=90

### AC2: Docker state alerts
- **Given** alert monitoring is enabled for Docker
- **When** a container changes state to exited or unhealthy
- **Then** an alert record is created with type=docker, severity=warning, target=container_name

### AC3: Systemd state alerts
- **Given** alert monitoring is enabled for Systemd
- **When** a service changes state to failed
- **Then** an alert record is created with type=systemd, severity=critical, target=service_name

### AC4: Cooldown prevents spam
- **Given** a CPU alert was triggered 5 minutes ago and cooldown is 15 minutes
- **When** CPU is still above threshold
- **Then** no new alert is created until cooldown expires

### AC5: Alert history persisted
- **Given** alerts have been triggered over time
- **When** I query the alert history
- **Then** all alerts are stored in SQLite with: id, type, severity, message, value, threshold, created_at

### AC6: Severity levels
- **Given** an alert is created
- **When** I view it
- **Then** severity is one of: critical (red), warning (yellow), info (blue)

---

## Scope

### In Scope
- Alert rule evaluation engine (goroutine)
- Metric threshold evaluation (CPU, RAM, Disk, Temperature)
- Service state change detection (Docker, Systemd)
- Cooldown timer per alert rule
- Alert record persistence in SQLite
- Default alert rules (CPU > 90%, RAM > 85%, Disk > 90%, Temp > 75C)

### Out of Scope
- Alert configuration UI (US0009)
- Notification channels (US0010, US0011)
- Alert dashboard panel (US0012)

---

## Technical Notes

- Run evaluation in same goroutine as metrics collection (after each cycle)
- Cooldown: in-memory map of rule_id -> last_triggered_time
- State change detection: compare current Docker/Systemd state with previous cycle
- AlertConfig table stores rules; Alert table stores history

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Multiple rules trigger simultaneously | All alerts created independently |
| Rule disabled | Skipped during evaluation |
| Metric temporarily spikes then drops | Alert created on first cross, cooldown prevents duplicates |
| DB full (disk space) | Log error, alerts still evaluated but not persisted |
| Alert rule with invalid metric type | Skip rule, log warning |
| Cooldown set to 0 | Alert on every cycle (valid but not recommended) |
| Server restart clears cooldown timers | First alert after restart may re-trigger (acceptable) |

---

## Test Scenarios

- [ ] CPU threshold alert triggers correctly
- [ ] RAM threshold alert triggers correctly
- [ ] Disk threshold alert triggers correctly
- [ ] Temperature threshold alert triggers correctly
- [ ] Docker container state change triggers alert
- [ ] Systemd service failure triggers alert
- [ ] Cooldown prevents duplicate alerts
- [ ] Cooldown expires and allows new alert
- [ ] Disabled rule is skipped
- [ ] Alert record saved to SQLite correctly
- [ ] Multiple simultaneous alerts handled

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0004](US0004-system-metrics-collector.md) | Data | Metrics data | Draft |
| [US0005](US0005-docker-monitor.md) | Data | Docker state data | Draft |
| [US0006](US0006-systemd-monitor.md) | Data | Systemd state data | Draft |

---

## Estimation

**Story Points:** 5
**Complexity:** Medium

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0003 |
