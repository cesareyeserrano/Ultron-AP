# US0014: Systemd Service Controls

> **Status:** Draft
> **Epic:** [EP0004: Service Controls](../epics/EP0004-service-controls.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to start, stop, and restart Systemd services from the dashboard
**So that** I can manage system services without SSH

## Context

### Persona Reference
**Admin** - Wants unified control over all services
[Full persona details](../personas.md#admin)

### Background
Adds control buttons to the Systemd service list (US0006). Executes systemctl commands. Same confirmation and audit trail pattern as Docker controls.

---

## Acceptance Criteria

### AC1: Start service
- **Given** an inactive service "my-service.service"
- **When** I click the Start button
- **Then** `systemctl start my-service.service` is executed
- **And** success feedback shown and action logged

### AC2: Stop service with confirmation
- **Given** an active service
- **When** I click Stop and confirm
- **Then** `systemctl stop my-service.service` is executed
- **And** success feedback shown and action logged

### AC3: Restart service with confirmation
- **Given** an active or failed service
- **When** I click Restart and confirm
- **Then** `systemctl restart my-service.service` is executed
- **And** success feedback shown and action logged

### AC4: Button states
- **Given** a service in state "active"
- **Then** Start is disabled, Stop and Restart are enabled
- **And** for "inactive": Start is enabled, Stop and Restart are disabled
- **And** for "failed": Start and Restart are enabled, Stop is disabled

### AC5: Permission handling
- **Given** the process lacks privileges for a service
- **When** I attempt to control it
- **Then** error message: "Permission denied: cannot control {service}"

---

## Scope

### In Scope
- Start, Stop, Restart buttons per service
- Confirmation modal for Stop/Restart
- systemctl command execution
- Success/error feedback
- Audit log (ActionLog table)

### Out of Scope
- Enable/Disable service (persist across reboots)
- Service configuration editing
- Journal/log viewing

---

## Technical Notes

- Execute via `exec.CommandContext("systemctl", action, serviceName)`
- Capture stdout/stderr for error reporting
- 30s timeout on command execution
- May require sudo or polkit rules for non-root execution

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Service not found | Error: "Service not found: {name}" |
| Permission denied (no sudo) | Error: "Permission denied" with setup instructions |
| Service fails to start | Error with stderr output from systemctl |
| systemctl hangs | Timeout after 30s, report timeout error |
| Service name with special characters | Sanitize input, reject if invalid |

---

## Test Scenarios

- [ ] Start button starts inactive service
- [ ] Stop button shows confirmation and stops
- [ ] Restart button shows confirmation and restarts
- [ ] Button states match service state
- [ ] Permission error handled gracefully
- [ ] Action logged to SQLite
- [ ] Timeout handled
- [ ] Service name sanitized

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0006](US0006-systemd-monitor.md) | UI | Systemd service list | Draft |
| [US0002](US0002-authentication.md) | Auth | Auth middleware | Draft |

---

## Estimation

**Story Points:** 3
**Complexity:** Low

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0004 |
