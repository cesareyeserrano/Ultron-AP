# EP0004: Service Controls

> **Status:** Draft
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09
> **Target Release:** Phase 2

## Summary

Agregar controles de Start, Stop y Restart para contenedores Docker y servicios Systemd directamente desde el dashboard. Convierte a Ultron-AP de un panel de solo lectura a una herramienta de administracion activa, eliminando la necesidad de SSH para operaciones basicas de servicio.

## Inherited Constraints

> See PRD and TRD for full constraint details. Key constraints for this epic:

| Source | Type | Constraint | Impact |
|--------|------|------------|--------|
| PRD | Security | Confirmacion explicita antes de Stop/Restart | Modal de confirmacion obligatorio |
| PRD | Security | Audit trail de todas las acciones | Log en SQLite con usuario, accion, resultado |
| PRD | Architecture | Docker SDK + Systemd D-Bus/exec | Mismas integraciones que EP0002, pero con escritura |

---

## Business Context

### Problem Statement
El admin puede ver que un servicio esta fallando (gracias a EP0002), pero actualmente debe conectarse por SSH para reiniciarlo. Esto agrega friccion, especialmente desde un dispositivo movil o cuando se esta fuera de casa.

**PRD Reference:** [FT-008: Service Controls](../prd.md#ft-008-service-controls)

### Value Proposition
Accion inmediata desde el dashboard. Ver un problema y resolverlo en la misma pantalla — sin SSH, sin terminal, sin recordar comandos. Todo con un click (y confirmacion).

### Success Metrics

| Metric | Current | Target | Measurement |
|--------|---------|--------|-------------|
| Tiempo para reiniciar un servicio | 30-60s (SSH + comando) | < 5s (click + confirm) | Observacion |
| Acciones requeridas para restart | 3+ (SSH, cd, docker restart) | 2 (click + confirm) | Conteo |
| Audit trail completeness | 0% | 100% de acciones loggeadas | Query SQLite |

---

## Scope

### In Scope
- Botones de Start, Stop, Restart por cada contenedor Docker
- Botones de Start, Stop, Restart por cada servicio Systemd
- Modal de confirmacion antes de Stop y Restart
- Ejecucion asincrona de acciones con feedback visual
- Log de acciones (audit trail) en SQLite
- Feedback de resultado: success (verde) o error (rojo con mensaje)

### Out of Scope
- Crear/eliminar contenedores Docker
- Deploy/update de imagenes Docker
- Editar configuracion de servicios
- Terminal/shell remoto
- Logs en tiempo real de servicios (potencial futuro)
- Bulk actions (operar multiples servicios a la vez)

### Affected Personas
- **Admin:** Controla servicios desde el dashboard

---

## Acceptance Criteria (Epic Level)

- [ ] Cada contenedor Docker tiene botones Start, Stop, Restart visibles
- [ ] Cada servicio Systemd tiene botones Start, Stop, Restart visibles
- [ ] Stop y Restart muestran modal de confirmacion antes de ejecutar
- [ ] Start no requiere confirmacion (accion segura)
- [ ] La accion se ejecuta de forma asincrona y muestra spinner mientras se procesa
- [ ] El resultado (success/error) se muestra como toast/notificacion inline
- [ ] Todas las acciones se registran en SQLite con: usuario, accion, target, resultado, timestamp
- [ ] El admin puede ver el historial de acciones ejecutadas
- [ ] Los botones se deshabilitan apropiadamente segun el estado (ej. no Start si ya esta running)

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
- El proceso Go tiene permisos para ejecutar `docker start/stop/restart`
- El proceso Go tiene permisos para ejecutar `systemctl start/stop/restart`
- El admin entiende las implicaciones de detener servicios

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Admin detiene un servicio critico accidentalmente | Medium | High | Modal de confirmacion; highlight en rojo para acciones destructivas |
| Permisos insuficientes para controlar Systemd | Medium | Medium | Documentar requisitos de permisos; verificar al inicio |
| Docker stop timeout causa hang en UI | Low | Medium | Timeout en el SDK call; feedback asincrono |
| Servicio no se recupera despues de restart | Low | High | Mostrar estado post-accion; alerta si no vuelve a healthy |

---

## Technical Considerations

### Architecture Impact
- Agrega endpoints REST con side effects (POST para acciones)
- Proteccion CSRF critica para estos endpoints
- Tabla ActionLog en SQLite
- Patron async action -> feedback via SSE/HTMX swap

### Integration Points
- Docker SDK: container.Start(), container.Stop(), container.Restart()
- Systemd: `systemctl start/stop/restart {service}`
- HTMX: hx-post para acciones, hx-swap para feedback

---

## Sizing

**Story Points:** 13
**Estimated Story Count:** 3-4

**Complexity Factors:**
- Reutiliza integraciones Docker/Systemd de EP0002
- Modal de confirmacion con HTMX
- Audit trail (CRUD simple en SQLite)
- Manejo de permisos del sistema operativo

---

## Story Breakdown

- [ ] [US0013: Docker Container Controls](../stories/US0013-docker-controls.md) (5 pts)
- [ ] [US0014: Systemd Service Controls](../stories/US0014-systemd-controls.md) (3 pts)
- [ ] [US0015: Action History & Audit Trail](../stories/US0015-action-history.md) (3 pts)

---

## Test Plan

> Test specs will be generated with `/sdlc-studio test-spec --epic EP0004`

---

## Open Questions

- [ ] Deberia haber un rol "read-only" que no pueda ejecutar controles? - Owner: Product
- [ ] Timeout para docker stop: usar el default (10s) o hacerlo configurable? - Owner: Dev
- [ ] Mostrar historial de acciones en pagina separada o en sidebar? - Owner: Design

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Epic creado desde PRD v1.0.0 |
