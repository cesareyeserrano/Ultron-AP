# EP0001: Foundation & Authentication

> **Status:** Draft
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09
> **Target Release:** Phase 1

## Summary

Establecer la base del proyecto Ultron-AP: estructura del servidor Go, sistema de autenticacion, layout UI dark mode, y la infraestructura de base de datos SQLite. Este epic es prerequisito para todos los demas — nada puede funcionar sin el servidor, la autenticacion y el shell visual.

## Inherited Constraints

> See PRD and TRD for full constraint details. Key constraints for this epic:

| Source | Type | Constraint | Impact |
|--------|------|------------|--------|
| PRD | Performance | < 30MB RAM total | El servidor base + auth debe usar < 10MB |
| PRD | Performance | Binario ARM < 20MB | Compilacion cruzada con dependencias minimas |
| PRD | Security | bcrypt, CSRF, secure cookies | Auth debe cumplir todos los requisitos de seguridad |
| PRD | Architecture | Monolito unico binario Go | Todo embebido: templates, CSS, assets |

---

## Business Context

### Problem Statement
Sin un servidor funcional con autenticacion y UI base, ningun otro feature puede desarrollarse ni desplegarse.

**PRD Reference:** [Project Overview](../prd.md#1-project-overview)

### Value Proposition
Proporciona la base segura y profesional sobre la que se construye todo el panel. Un login robusto protege el acceso, y el layout dark mode establece la identidad visual del producto.

### Success Metrics

| Metric | Current | Target | Measurement |
|--------|---------|--------|-------------|
| Tiempo de arranque del servidor | N/A | < 2s | Medicion manual |
| Consumo de RAM base (idle) | N/A | < 10MB | `ps aux` o gopsutil |
| Tiempo de respuesta pagina login | N/A | < 100ms | Benchmark local |
| Intentos de brute force bloqueados | N/A | 100% despues de 5 intentos | Test automatizado |

---

## Scope

### In Scope
- Estructura del proyecto Go (cmd/, internal/, web/)
- Servidor HTTP con router y middleware
- Sistema de autenticacion completo (login/logout/sessions)
- Base de datos SQLite (schema inicial, migraciones)
- Layout UI base con Tailwind CSS dark mode
- Sidebar de navegacion colapsable
- Pagina de login
- Health check endpoint (/health)
- Configuracion via env vars y/o YAML
- Systemd unit file para auto-arranque

### Out of Scope
- Metricas del sistema (EP0002)
- Alertas y notificaciones (EP0003)
- Controles de servicios (EP0004)
- HTTPS nativo (open question en PRD)

### Affected Personas
- **Admin:** Puede hacer login y ver el shell vacio del dashboard

---

## Acceptance Criteria (Epic Level)

- [ ] El binario Go compila para ARM (linux/arm64) y arranca en < 2s
- [ ] El servidor escucha en el puerto configurado (default 8080)
- [ ] /health responde 200 OK sin autenticacion
- [ ] /login muestra formulario de login con UI dark mode profesional
- [ ] Login exitoso redirige al dashboard (vacio por ahora)
- [ ] Login fallido muestra error y no revela si el usuario existe o no
- [ ] Despues de 5 intentos fallidos, el login se bloquea por 15 minutos
- [ ] Todas las rutas excepto /login y /health requieren autenticacion
- [ ] El logout destruye la sesion y redirige a /login
- [ ] La sesion expira despues del TTL configurado (default 24h)
- [ ] SQLite se inicializa automaticamente en el primer arranque
- [ ] El layout tiene sidebar colapsable y es responsive (mobile/tablet/desktop)
- [ ] Consumo de RAM idle < 10MB

---

## Dependencies

### Blocked By

| Dependency | Type | Status | Owner |
|------------|------|--------|-------|
| Ninguna | - | - | - |

### Blocking

| Item | Type | Impact |
|------|------|--------|
| EP0002: System Monitoring | Epic | Requiere servidor y auth funcionando |
| EP0003: Alerting & Notifications | Epic | Requiere servidor, auth, y DB |
| EP0004: Service Controls | Epic | Requiere servidor, auth, y UI |

---

## Risks & Assumptions

### Assumptions
- Go 1.21+ disponible para desarrollo
- El target es linux/arm64 (Raspberry Pi 4/5 con OS de 64 bits)
- modernc.org/sqlite (pure Go) funciona correctamente en ARM sin CGo
- Tailwind CSS se purga en build-time para minimizar tamano de CSS

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| SQLite pure Go lento en ARM | Low | Medium | Benchmark temprano; fallback a CGo si necesario |
| Tailwind CSS embebido agranda el binario | Low | Low | Purge agresivo; solo clases usadas |
| bcrypt lento en ARM para brute force protection | Medium | Low | Cost factor 10 es suficiente; no afecta UX normal |

---

## Technical Considerations

### Architecture Impact
Define la estructura base del proyecto que todos los epics usaran. Decisiones de layout de directorios, patrones de middleware, y manejo de sesiones afectan todo lo demas.

### Integration Points
- SQLite: Esquema inicial con tablas User, Session
- Filesystem: Lectura de config YAML, escritura de DB
- Systemd: Unit file para arranque automatico

---

## Sizing

**Story Points:** 21
**Estimated Story Count:** 5-7

**Complexity Factors:**
- Setup de proyecto Go con estructura limpia
- Compilacion cruzada ARM con SQLite pure Go
- Sistema de sesiones seguro con proteccion brute force
- Layout responsive con Tailwind CSS dark mode
- Embeber assets estaticos en binario Go

---

## Story Breakdown

- [ ] [US0001: Project Scaffolding & Go Server](../stories/US0001-project-scaffolding.md) (5 pts)
- [ ] [US0002: User Authentication](../stories/US0002-authentication.md) (8 pts)
- [ ] [US0003: Dark Mode UI Layout](../stories/US0003-dark-mode-layout.md) (5 pts)

---

## Test Plan

> Test specs will be generated with `/sdlc-studio test-spec --epic EP0001`

---

## Open Questions

- [ ] Usar `embed.FS` de Go para embeber templates y CSS en el binario? - Owner: Dev
- [ ] Primer setup de admin: env vars vs web wizard vs CLI? (hereda de PRD open question) - Owner: Product

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Epic creado desde PRD v1.0.0 |
