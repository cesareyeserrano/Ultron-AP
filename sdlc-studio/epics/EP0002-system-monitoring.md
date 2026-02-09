# EP0002: System Monitoring

> **Status:** Draft
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09
> **Target Release:** Phase 1

## Summary

Implementar el dashboard de monitoreo en tiempo real que muestra metricas del sistema (CPU, RAM, Disco, Red, Temperatura), estado de contenedores Docker, y estado de servicios Systemd. Este es el corazon del producto — la razon principal de existir de Ultron-AP.

## Inherited Constraints

> See PRD and TRD for full constraint details. Key constraints for this epic:

| Source | Type | Constraint | Impact |
|--------|------|------------|--------|
| PRD | Performance | Recoleccion < 2% CPU sostenido | Intervalos y eficiencia de recoleccion criticos |
| PRD | Performance | Respuesta < 200ms en LAN | SSE y templates deben ser eficientes |
| PRD | Scalability | Hasta 50 Docker + 100 Systemd | Debe manejar volumen sin degradar rendimiento |
| PRD | Architecture | SSE para real-time | No WebSockets; Server-Sent Events |

---

## Business Context

### Problem Statement
El admin necesita una vista unificada y en tiempo real de la salud de su Raspberry Pi y todos los servicios que corren en ella, sin tener que conectarse por SSH ni revisar multiples herramientas.

**PRD Reference:** [Problem Statement](../prd.md#2-problem-statement)

### Value Proposition
Un solo dashboard que responde la pregunta "esta todo bien?" en menos de 5 segundos. Metricas del sistema + Docker + Systemd en una sola pantalla con actualizacion automatica.

### Success Metrics

| Metric | Current | Target | Measurement |
|--------|---------|--------|-------------|
| Tiempo para evaluar salud del sistema | Minutos (SSH + comandos) | < 5 segundos (un vistazo) | Observacion |
| CPU overhead de recoleccion | N/A | < 2% sostenido | gopsutil self-monitoring |
| Latencia de actualizacion SSE | N/A | < 1s end-to-end | Medicion en browser |
| Contenedores visibles | 0 | 100% de los existentes | Comparar con `docker ps -a` |

---

## Scope

### In Scope
- Recoleccion periodica de metricas del sistema (CPU, RAM, Disco, Red, Temperatura)
- Ring buffer in-memory para historico de 24h
- Graficas historicas de metricas (ultimos 60 min en dashboard)
- Listado de contenedores Docker con estado y metricas
- Detalle expandible por contenedor (ports, volumes, env)
- Listado de servicios Systemd con estado
- Filtro de servicios con errores
- SSE endpoint para streaming de metricas en tiempo real
- Indicadores visuales de salud con colores (verde/amarillo/rojo)
- Uptime del sistema

### Out of Scope
- Alertas basadas en umbrales (EP0003)
- Start/Stop/Restart de servicios (EP0004)
- Historico mayor a 24h (open question)
- Logs de contenedores/servicios en tiempo real (futuro)

### Affected Personas
- **Admin:** Ve toda la informacion de monitoreo en un dashboard

---

## Acceptance Criteria (Epic Level)

- [ ] Dashboard principal muestra CPU%, RAM%, Disco%, Red, y Temperatura en tiempo real
- [ ] Graficas historicas de CPU y RAM muestran los ultimos 60 minutos
- [ ] Temperatura del CPU tiene indicador de color (verde < 60C, amarillo 60-75C, rojo > 75C)
- [ ] Seccion Docker lista todos los contenedores con estado y metricas basicas
- [ ] Click en contenedor expande detalles (ports, volumes, environment)
- [ ] Seccion Systemd lista servicios activos/habilitados con estado
- [ ] Filtro funcional para mostrar solo servicios con errores
- [ ] Todas las metricas se actualizan via SSE sin recargar la pagina
- [ ] La recoleccion de metricas consume < 2% CPU sostenido
- [ ] Funciona correctamente con 50+ contenedores Docker

---

## Dependencies

### Blocked By

| Dependency | Type | Status | Owner |
|------------|------|--------|-------|
| EP0001: Foundation & Auth | Epic | Draft | TBD |

### Blocking

| Item | Type | Impact |
|------|------|--------|
| EP0003: Alerting & Notifications | Epic | Necesita metricas para evaluar umbrales |
| EP0004: Service Controls | Epic | Necesita listados de Docker/Systemd |

---

## Risks & Assumptions

### Assumptions
- Docker socket (/var/run/docker.sock) accesible para el proceso Go
- gopsutil funciona correctamente en ARM Linux para temperatura del CPU
- El admin ejecuta el binario con permisos para leer servicios Systemd

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| gopsutil no reporta temperatura en ARM | Medium | Medium | Fallback a leer /sys/class/thermal directamente |
| Docker SDK consume mucha memoria con 50+ containers | Low | High | Lazy loading, pagination, cache con TTL |
| SSE con muchos clientes simultanios | Low | Low | Disenado para 1 usuario; broadcast pattern simple |
| Systemd D-Bus no disponible | Low | Medium | Fallback a exec `systemctl` como alternativa |

---

## Technical Considerations

### Architecture Impact
- Introduce el patron de recoleccion periodica (collectors)
- Establece el patron SSE para streaming de datos al frontend
- Ring buffer in-memory define patron de almacenamiento temporal de metricas
- HTMX partial updates para UI reactiva sin SPA

### Integration Points
- Docker Engine via unix socket + Docker SDK for Go
- Systemd via D-Bus o subprocess `systemctl`
- gopsutil para metricas de sistema
- SSE endpoint consumido por HTMX en el frontend

---

## Sizing

**Story Points:** 34
**Estimated Story Count:** 6-8

**Complexity Factors:**
- Integracion con Docker SDK (API compleja)
- Integracion con Systemd (D-Bus o subprocess)
- SSE streaming con HTMX
- Ring buffer eficiente en memoria
- Graficas historicas en frontend (ligeras, sin D3.js pesado)
- Responsive layout para seccion de metricas

---

## Story Breakdown

- [ ] [US0004: System Metrics Collector](../stories/US0004-system-metrics-collector.md) (5 pts)
- [ ] [US0005: Docker Container Monitor](../stories/US0005-docker-monitor.md) (5 pts)
- [ ] [US0006: Systemd Service Monitor](../stories/US0006-systemd-monitor.md) (3 pts)
- [ ] [US0007: Dashboard View with SSE](../stories/US0007-dashboard-view.md) (8 pts)

---

## Test Plan

> Test specs will be generated with `/sdlc-studio test-spec --epic EP0002`

---

## Open Questions

- [ ] Usar Chart.js (ligero) o uPlot (ultra-ligero) para graficas? - Owner: Dev
- [ ] Retencion de metricas: 24h suficiente o necesitamos mas? (hereda de PRD) - Owner: Product
- [ ] Fallback si Docker no esta instalado: mostrar seccion vacia o esconderla? - Owner: Dev

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Epic creado desde PRD v1.0.0 |
