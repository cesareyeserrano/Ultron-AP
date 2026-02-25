# AF-SPEC: pironman-optional-integration

STATUS: APPROVED
## 1. Context
Integracion opcional de Pironman en Ultron, desacoplada del core, con control de hardware estable y seguro estilo Home Assistant.

Primary actor: Admin operator de Ultron, maintainer de plataforma, y security owner.
Expected outcome: El operador puede ver estado de integracion (available/unavailable/degraded), leer configuracion Pironman y aplicar cambios de forma determinista sin bloqueos; si falla, Ultron sigue operativo y muestra error accionable.
In scope: Modulo opcional por feature flag, API adapter Pironman, cola single-flight para apply, timeouts/cancelacion, telemetria por request, UI de hardware aislada, y diagnosticos de CPU/RAM por proceso.
Out of scope: No rediseño visual global, no gestion de lifecycle de servicios Pironman/Influx desde core, no cambios en modulos no hardware.
Technology: Go + HTMX + SQLite + ultron-helper via unix socket + Pironman API local.
Requirement source: Provided explicitly by user in guided draft.

## 2. Actors
- Admin operator de Ultron, maintainer de plataforma, y security owner.

## 3. Functional Rules (traceable)
- FR-1: The optional Pironman module must run with explicit feature flag gating and default disabled mode.
- FR-2: Hardware settings must be applied only by explicit user action and only one apply operation can run at a time.
- FR-3: Integration must expose capability states `available|unavailable|degraded` and must not block Ultron core when Pironman fails.
- FR-4: Privileged operations must remain in helper boundary with strict parameter validation and auditable logs.

## 4. Edge Cases
- Timeout o caida de API Pironman durante apply, o clicks repetidos de apply en rafaga.

## 5. Failure Conditions
- Pironman local API timeout during read/apply.
- Repeated apply clicks while a previous apply is still in progress.
- Helper unavailable or socket I/O timeout.

## 6. Non-Functional Requirements
- Raspberry Pi 5 lightweight target in idle:
- `ultron-ap` <= 2% CPU.
- `ultron-helper` <= 1% CPU.
- No continuous polling loops for hardware integration; diagnostics must be on-demand.
- Any apply operation must have bounded timeout and deterministic exit state.

## 7. Security Considerations
- Mantener privilegios fuera del web process: helper root como unico boundary, validacion estricta de parametros, CSRF/session obligatorios y logs auditables.

## 8. Out of Scope
- No rediseño visual global, no gestion de lifecycle de servicios Pironman/Influx desde core, no cambios en modulos no hardware.

## 9. Acceptance Criteria
- AC-1: Given un admin autenticado en el modulo opcional de hardware, when edita valores y presiona Apply, then se ejecuta una sola operacion, el estado transiciona a applied o failed, y la UI nunca queda en busy permanente.
- AC-2: Given el feature flag de Pironman esta deshabilitado, when el usuario navega por Ultron core, then no se muestra modulo hardware ni se ejecutan operaciones Pironman.
- AC-3: Given Pironman API falla o excede timeout, when se solicita read/apply, then la integracion reporta `degraded` o `unavailable` y Ultron core permanece operativo.
- AC-4: Given una solicitud invalida de parametros hardware, when llega al helper, then se rechaza y queda registro auditable del rechazo.

## 10. Requirement Source Statement
- All requirements in this draft were provided explicitly by the user.
- Aitri structured the content and did not invent requirements.

## 11. Resource Strategy
- Sin polling continuo; diagnosticos solo on-demand; timeouts cortos; sin loops extra en idle; presupuesto objetivo en Pi5: ultron-ap <=2% CPU y ultron-helper <=1% CPU en reposo.

## 12. Asset Strategy
- No external visual/game assets required.
- Reuse existing Ultron templates and styles; no UI redesign required for this feature.
