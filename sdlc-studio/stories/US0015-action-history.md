# US0015: Action History & Audit Trail

> **Status:** Draft
> **Epic:** [EP0004: Service Controls](../epics/EP0004-service-controls.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to see a history of all service control actions I've executed
**So that** I can review what was done and when for troubleshooting

## Context

### Persona Reference
**Admin** - Wants accountability and traceability for service operations
[Full persona details](../personas.md#admin)

### Background
A dedicated page (or section) showing the audit trail of all Start/Stop/Restart actions executed through the panel. Uses the ActionLog table populated by US0013 and US0014.

---

## Acceptance Criteria

### AC1: Action history page
- **Given** I navigate to the Action History page
- **When** the page loads
- **Then** I see a table of all actions sorted by newest first
- **And** each row shows: timestamp, action, target type (docker/systemd), target name, result, user

### AC2: Filter by type
- **Given** actions exist for both Docker and Systemd
- **When** I filter by "Docker"
- **Then** only Docker actions are shown

### AC3: Result indicators
- **Given** actions with various results
- **When** I view the history
- **Then** successful actions show green checkmark
- **And** failed actions show red X with error message on hover/expand

---

## Scope

### In Scope
- Action history page
- Filter by type (Docker/Systemd)
- Result indicators (success/error)
- Pagination (20 per page)

### Out of Scope
- Export to CSV
- Action replay
- Undo functionality

---

## Technical Notes

- Simple HTMX `hx-get` with query params for filtering
- Read from ActionLog table in SQLite

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| No actions yet | Show "No actions recorded" message |
| Hundreds of actions | Paginated (20 per page) |
| Very long error message | Truncated with expandable detail |
| DB read error | Show error message, offer retry |
| Action with deleted container | Show original name, note "no longer exists" |

---

## Test Scenarios

- [ ] History page renders with actions
- [ ] Actions sorted newest first
- [ ] Filter by Docker works
- [ ] Filter by Systemd works
- [ ] Success actions show green indicator
- [ ] Failed actions show red with message
- [ ] Empty state shown when no actions
- [ ] Pagination works

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0013](US0013-docker-controls.md) | Data | ActionLog records (Docker) | Draft |
| [US0014](US0014-systemd-controls.md) | Data | ActionLog records (Systemd) | Draft |
| [US0003](US0003-dark-mode-layout.md) | UI | Layout shell | Draft |

---

## Estimation

**Story Points:** 3
**Complexity:** Low

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0004 |
