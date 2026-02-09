# EP0003: Alerting & Notifications

> **Status:** Draft
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09
> **Target Release:** Phase 1

## Summary

Implementar el sistema de alertas con umbrales configurables y canales de notificacion (dashboard, Telegram, email). El admin define reglas ("si CPU > 90%, alerta critical"), el sistema evalua las metricas continuamente, y notifica por los canales configurados cuando se cruza un umbral.

## Inherited Constraints

> See PRD and TRD for full constraint details. Key constraints for this epic:

| Source | Type | Constraint | Impact |
|--------|------|------------|--------|
| PRD | Performance | Evaluacion de alertas no debe impactar recoleccion | Evaluacion ligera, no bloqueante |
| PRD | Security | Credenciales SMTP y Telegram token seguros | No exponer en logs ni en UI |
| PRD | Architecture | SQLite para persistencia de alertas | Historial y configuracion en DB |

---

## Business Context

### Problem Statement
Monitorear un dashboard 24/7 no es viable. El admin necesita ser notificado proactivamente cuando algo va mal, incluso cuando no esta mirando el panel. Las alertas con umbrales configurables y notificaciones push (Telegram) convierten el monitoreo pasivo en activo.

**PRD Reference:** [FT-004: Alert System](../prd.md#ft-004-alert-system)

### Value Proposition
Tranquilidad. El admin no necesita vigilar el dashboard constantemente — Ultron-AP le avisa automaticamente cuando algo requiere atencion, por el canal que prefiera.

### Success Metrics

| Metric | Current | Target | Measurement |
|--------|---------|--------|-------------|
| Tiempo de deteccion de problema | Manual (desconocido) | < 30 segundos | Timestamp metrica vs timestamp alerta |
| Falsos positivos | N/A | < 5% | Alertas vs incidentes reales |
| Tiempo de notificacion (Telegram) | N/A | < 10s desde deteccion | Timestamp alerta vs mensaje Telegram |
| Cooldown efectivo | N/A | 0 alertas duplicadas en ventana | Test automatizado |

---

## Scope

### In Scope
- Motor de alertas que evalua metricas contra umbrales configurados
- Umbrales configurables: CPU%, RAM%, Disco%, Temperatura
- Alertas de estado: Docker container unhealthy/exited, Systemd service failed
- Severidades: critical, warning, info
- Cooldown configurable entre alertas del mismo tipo (default 15 min)
- Panel de alertas en dashboard con filtros por severidad
- Historial de alertas persistido en SQLite
- Notificaciones via Telegram Bot (configuracion, envio, test, silenciar)
- Notificaciones via Email/SMTP (configuracion, envio, test)
- Digest diario por email (resumen de alertas del dia)
- Pagina de configuracion de alertas en el panel

### Out of Scope
- Notificaciones via Slack, Discord u otros canales
- Escalation automatico (si no se atiende en X tiempo, escalar)
- Alertas predictivas basadas en tendencias
- Webhook generico (potencial futuro)

### Affected Personas
- **Admin:** Configura alertas, recibe notificaciones, revisa historial

---

## Acceptance Criteria (Epic Level)

- [ ] El admin puede crear reglas de alerta con metrica, operador, umbral y severidad
- [ ] Las alertas se evaluan automaticamente con cada ciclo de metricas
- [ ] Cuando se cruza un umbral, se crea un registro de alerta en SQLite
- [ ] La alerta aparece en el panel de alertas del dashboard en tiempo real (SSE)
- [ ] El cooldown previene alertas duplicadas dentro de la ventana configurada
- [ ] Las alertas critical y warning se envian a Telegram si esta configurado
- [ ] Las alertas critical se envian por email si SMTP esta configurado
- [ ] El admin puede configurar Telegram (token + chat ID) desde el panel
- [ ] El admin puede configurar SMTP desde el panel
- [ ] Existe boton de test para Telegram y para Email
- [ ] El admin puede silenciar notificaciones temporalmente (1h, 4h, 24h)
- [ ] El historial de alertas tiene filtros por severidad y tipo

---

## Dependencies

### Blocked By

| Dependency | Type | Status | Owner |
|------------|------|--------|-------|
| EP0001: Foundation & Auth | Epic | Draft | TBD |
| EP0002: System Monitoring | Epic | Draft | TBD |

### Blocking

| Item | Type | Impact |
|------|------|--------|
| Ninguno | - | - |

---

## Risks & Assumptions

### Assumptions
- El admin tiene acceso a crear un Telegram Bot (via @BotFather)
- El admin tiene acceso a un servidor SMTP (Gmail, Outlook, propio)
- Las metricas de EP0002 estan disponibles para evaluacion de umbrales

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Telegram API rate limits | Low | Medium | Batch alerts, respect rate limits (30 msgs/sec) |
| SMTP timeout bloquea el thread principal | Medium | High | Enviar emails en goroutine separada con timeout |
| Demasiadas alertas saturan la DB | Low | Medium | Purga automatica de alertas > 30 dias |
| Credenciales SMTP/Telegram expuestas | Low | High | Cifrar en DB; no mostrar en UI; no loggear |

---

## Technical Considerations

### Architecture Impact
- Introduce el patron de evaluacion de reglas (rule engine simple)
- Agrega goroutines para envio asincrono de notificaciones
- Nuevas tablas SQLite: Alert, AlertConfig
- Pagina de configuracion en la UI

### Integration Points
- Telegram Bot API (HTTPS REST)
- SMTP Server (net/smtp o gomail)
- Ring buffer de metricas de EP0002 (consumidor)
- SSE para alertas en tiempo real en dashboard

---

## Sizing

**Story Points:** 26
**Estimated Story Count:** 5-7

**Complexity Factors:**
- Motor de reglas con cooldown y deduplicacion
- Integracion Telegram API
- Integracion SMTP con envio asincrono
- UI de configuracion de alertas
- Digest diario (scheduler/cron interno)
- Silenciar notificaciones con timer

---

## Story Breakdown

- [ ] [US0008: Alert Engine & Rule Evaluation](../stories/US0008-alert-engine.md) (5 pts)
- [ ] [US0009: Alert Configuration UI](../stories/US0009-alert-configuration-ui.md) (5 pts)
- [ ] [US0010: Telegram Notifications](../stories/US0010-telegram-notifications.md) (3 pts)
- [ ] [US0011: Email Notifications](../stories/US0011-email-notifications.md) (3 pts)
- [ ] [US0012: Alert Dashboard Panel](../stories/US0012-alert-dashboard-panel.md) (5 pts)

---

## Test Plan

> Test specs will be generated with `/sdlc-studio test-spec --epic EP0003`

---

## Open Questions

- [ ] El digest diario a que hora se envia? Configurable o fijo (ej. 08:00)? - Owner: Product
- [ ] Acknowledging de alertas: solo visual o tambien detiene notificaciones? - Owner: Product
- [ ] Limite de historial de alertas: purgar despues de 30 dias? Configurable? - Owner: Dev

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Epic creado desde PRD v1.0.0 |
