# US0006: Systemd Service Monitor

> **Status:** Planned
> **Epic:** [EP0002: System Monitoring](../epics/EP0002-system-monitoring.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to see the status of all Systemd services
**So that** I can monitor background processes on my Raspberry Pi

## Context

### Persona Reference
**Admin** - Runs Systemd services alongside Docker
[Full persona details](../personas.md#admin)

### Background
Queries Systemd for active/enabled service units, showing their state and since-when data. Updates every 30 seconds.

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Scalability | Up to 100 services | Must handle large service lists |
| PRD | Architecture | D-Bus or systemctl | Needs local system access |

---

## Acceptance Criteria

### AC1: List active/enabled services
- **Given** Systemd is running with services
- **When** the Systemd monitor collects data
- **Then** all active and enabled services are listed
- **And** each entry includes: name, state (active/inactive/failed), sub-state, since timestamp

### AC2: Health indicators
- **Given** services in various states
- **When** the data is displayed
- **Then** active services show green indicator
- **And** inactive services show grey indicator
- **And** failed services show red indicator

### AC3: Filter failed services
- **Given** the service list contains failed services
- **When** I apply the "errors only" filter
- **Then** only failed services are shown

### AC4: Auto-refresh every 30 seconds
- **Given** the Systemd monitor is running
- **When** 30 seconds pass
- **Then** the service list is refreshed

---

## Scope

### In Scope
- Systemd service listing (active + enabled units)
- State detection (active, inactive, failed)
- Since-when timestamp
- Error filter
- Periodic refresh

### Out of Scope
- Start/Stop/Restart controls (US0016, EP0004)
- Service logs
- Service configuration editing
- Timer units, socket units (only .service)

---

## Technical Notes

- Primary: exec `systemctl list-units --type=service --all --no-pager --plain` and parse output
- Alternative: D-Bus integration via `godbus` (more complex, consider for future)
- Parse output columns: UNIT, LOAD, ACTIVE, SUB, DESCRIPTION
- Filter to only `.service` units

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| systemctl not available | Show "Systemd not available" message |
| Permission denied | Log warning, show subset of visible services |
| Service with very long name | Truncate with ellipsis, show full name on hover/expand |
| 100+ services listed | All shown, scrollable list, no performance issue |
| Service state changes between cycles | Updated on next refresh cycle |

---

## Test Scenarios

- [ ] Lists all active and enabled services
- [ ] Shows correct state for each service
- [ ] Failed services show red indicator
- [ ] Error filter works correctly
- [ ] Handles systemctl not available
- [ ] Handles permission errors gracefully
- [ ] Refreshes every 30 seconds
- [ ] Handles 100+ services

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0001](US0001-project-scaffolding.md) | Infrastructure | Server, config | Draft |

---

## Estimation

**Story Points:** 3
**Complexity:** Low

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0002 |
