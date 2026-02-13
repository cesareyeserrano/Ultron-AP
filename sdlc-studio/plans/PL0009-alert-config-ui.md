# PL0009: Alert Configuration UI - Implementation Plan

> **Status:** Complete
> **Story:** [US0009: Alert Configuration UI](../stories/US0009-alert-configuration-ui.md)
> **Epic:** [EP0003: Alerting & Notifications](../epics/EP0003-alerting-and-notifications.md)
> **Created:** 2026-02-11
> **Language:** Go + HTML/HTMX

## Overview

Build the settings page for alert rules CRUD and notification channel configuration (Telegram, Email). HTMX inline forms, CSRF protection, sensitive field masking.

## Approach: Test-After (UI-heavy)

## Implementation Phases

### Phase 1: DB Methods for Alert CRUD + Notification Config
- Add UpdateAlertConfig, DeleteAlertConfig to database/alerts.go
- Add NotificationConfig table and CRUD to database/notifications.go

### Phase 2: Settings Page Handlers
- handleSettings — renders settings page with alert rules + notification config
- handleAlertRuleCreate (POST /api/alerts/rules)
- handleAlertRuleUpdate (PUT /api/alerts/rules/{id})
- handleAlertRuleDelete (DELETE /api/alerts/rules/{id})
- handleAlertRuleToggle (POST /api/alerts/rules/{id}/toggle)
- handleNotificationConfigSave (POST /api/notifications/{channel})

### Phase 3: Settings Template
- settings.html with tabs: Alert Rules, Notifications
- Alert rules table with inline add/edit forms
- Telegram/Email config forms with masked fields

### Phase 4: Routes + Wiring
- Register new API routes in server.go

### Phase 5: Tests

## Files to Create/Modify

| File | Action |
|------|--------|
| `internal/database/alerts.go` | Modify — add Update/Delete |
| `internal/database/notifications.go` | Create — NotificationConfig CRUD |
| `internal/server/handlers_settings.go` | Create — settings handlers |
| `web/templates/settings.html` | Create — settings page |
| `web/templates/partials/alert-rule-row.html` | Create — rule row partial |
| `internal/server/server.go` | Modify — add routes |
