# Epic Registry

**Last Updated:** 2026-02-09
**PRD Reference:** [Product Requirements Document](../prd.md)

## Summary

| Status | Count |
|--------|-------|
| Draft | 4 |
| Ready | 0 |
| Approved | 0 |
| In Progress | 0 |
| Done | 0 |
| **Total** | **4** |

## Epics

| ID | Title | Status | Owner | Features | Phase | Dependencies |
|----|-------|--------|-------|----------|-------|--------------|
| [EP0001](EP0001-foundation-and-auth.md) | Foundation & Authentication | Draft | TBD | FT-007, FT-009 | 1 | None |
| [EP0002](EP0002-system-monitoring.md) | System Monitoring | Draft | TBD | FT-001, FT-002, FT-003 | 1 | EP0001 |
| [EP0003](EP0003-alerting-and-notifications.md) | Alerting & Notifications | Draft | TBD | FT-004, FT-005, FT-006 | 1 | EP0001, EP0002 |
| [EP0004](EP0004-service-controls.md) | Service Controls | Draft | TBD | FT-008 | 2 | EP0001, EP0002 |

## Dependency Graph

```
EP0001 (Foundation & Auth)
  |
  +---> EP0002 (System Monitoring)
  |       |
  |       +---> EP0003 (Alerting & Notifications)
  |       |
  |       +---> EP0004 (Service Controls) [Phase 2]
  |
  +---> EP0003
  +---> EP0004
```

## Feature Mapping

| Feature | Epic | Rationale |
|---------|------|-----------|
| FT-007: Authentication | EP0001 | Prerequisito para todo; sin dependencias propias |
| FT-009: Dark Mode UI | EP0001 | Layout base necesario para todas las vistas |
| FT-001: System Metrics Dashboard | EP0002 | Core monitoring - metricas del sistema |
| FT-002: Docker Container Monitor | EP0002 | Core monitoring - misma vista de dashboard |
| FT-003: Systemd Service Monitor | EP0002 | Core monitoring - misma vista de dashboard |
| FT-004: Alert System | EP0003 | Depende de metricas; comparte pipeline con notificaciones |
| FT-005: Telegram Notifications | EP0003 | Canal de alerta; depende del motor de alertas |
| FT-006: Email Notifications | EP0003 | Canal de alerta; depende del motor de alertas |
| FT-008: Service Controls | EP0004 | Phase 2; depende de listados de Docker/Systemd |

## Notes

- Epics are numbered globally (EP0001, EP0002, etc.)
- Stories are tracked in [Story Registry](../stories/_index.md)
- All features (FT-001 through FT-009) are mapped to exactly one epic
- EP0001 must be completed first as it blocks all other epics
