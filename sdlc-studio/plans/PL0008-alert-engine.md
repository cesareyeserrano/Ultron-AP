# PL0008: Alert Engine & Rule Evaluation - Implementation Plan

> **Status:** Complete
> **Story:** [US0008: Alert Engine & Rule Evaluation](../stories/US0008-alert-engine.md)
> **Epic:** [EP0003: Alerting & Notifications](../epics/EP0003-alerting-and-notifications.md)
> **Created:** 2026-02-11
> **Language:** Go

## Overview

Build the alert engine that evaluates rules against current metrics, Docker states, and systemd states on each cycle. Includes cooldown logic, severity levels, and SQLite persistence via existing schema.

## Approach: TDD

The alert engine is pure logic (no UI), ideal for TDD.

## Implementation Phases

### Phase 1: DB Alert Methods
- Add `database/alerts.go` with CRUD for AlertConfig and Alert tables
- Methods: CreateAlertConfig, ListAlertConfigs, CreateAlert, ListAlerts, SeedDefaultRules

### Phase 2: Alert Models
- Add `internal/alerts/models.go` with AlertRule, Alert structs
- Severity enum (critical, warning, info)
- Operator evaluation function

### Phase 3: Alert Engine
- Add `internal/alerts/engine.go` with Engine struct
- Evaluate metric rules against latest Snapshot
- Detect Docker/Systemd state changes vs previous cycle
- Cooldown map: ruleKey -> lastTriggered
- Run as goroutine with same interval as metrics

### Phase 4: Wiring
- Update `cmd/ultron-ap/main.go` to create + start engine
- Update `server.New()` to accept alert engine (for future dashboard)
- Seed default rules on first run

### Phase 5: Tests
- DB alert methods tests
- Engine evaluation tests (threshold cross, cooldown, state change)
- Edge cases (disabled rules, invalid metrics, multiple simultaneous)

## Files to Create/Modify

| File | Action |
|------|--------|
| `internal/database/alerts.go` | Create — DB methods |
| `internal/database/alerts_test.go` | Create — DB tests |
| `internal/alerts/models.go` | Create — models |
| `internal/alerts/engine.go` | Create — engine |
| `internal/alerts/engine_test.go` | Create — engine tests |
| `cmd/ultron-ap/main.go` | Modify — wire engine |
| `internal/server/server.go` | Modify — accept engine |

## Estimated Effort: 5 points
