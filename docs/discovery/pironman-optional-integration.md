# Discovery: pironman-optional-integration

STATUS: DRAFT

## 1. Problem Statement
Derived from approved spec retrieval snapshot:
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria

### Context snapshot
- Integracion opcional de Pironman en Ultron, desacoplada del core, con control de hardware estable y seguro estilo Home Assistant.
- Primary actor: Admin operator de Ultron, maintainer de plataforma, y security owner.
- Expected outcome: El operador puede ver estado de integracion (available/unavailable/degraded), leer configuracion Pironman y aplicar cambios de forma determinista sin bloqueos; si falla, Ultron sigue operativo y muestra error accionable.

### Actors snapshot
- Admin operator de Ultron, maintainer de plataforma, y security owner.

### Functional rules snapshot
- The optional Pironman module must run with explicit feature flag gating and default disabled mode.
- Hardware settings must be applied only by explicit user action and only one apply operation can run at a time.
- Integration must expose capability states `available|unavailable|degraded` and must not block Ultron core when Pironman fails.
- Privileged operations must remain in helper boundary with strict parameter validation and auditable logs.

### Security snapshot
- Mantener privilegios fuera del web process: helper root como unico boundary, validacion estricta de parametros, CSRF/session obligatorios y logs auditables.

### Out-of-scope snapshot
- No rediseño visual global, no gestion de lifecycle de servicios Pironman/Influx desde core, no cambios en modulos no hardware.

Refined problem framing:
- What problem are we solving? Current direct integration caused busy/timeout states and degraded UX; high severity because it blocks operator actions.
- Why now? 0 stuck busy states; p95 apply under 2s when API healthy; idle CPU budget maintained (ultron-ap <=2%, helper <=1%).; Baseline: No hardware module in core; optional diagnostics endpoint exists; Pironman/influx often inactive by default.

## 2. Discovery Interview Summary (Discovery Persona)
- Primary users:
- Admin operators on Raspberry Pi 5, platform maintainer, and security owner.

- Jobs to be done:
- Read Pironman status/config and apply hardware settings safely without freezing Ultron core.

- Current pain:
- Current direct integration caused busy/timeout states and degraded UX; high severity because it blocks operator actions.

- Constraints (business/technical/compliance):
- Keep Ultron lightweight on Pi5, no service lifecycle control from core, strict helper privilege boundary, CSRF/session enforcement, no regressions in core modules.

- Dependencies:
- ultron-helper over unix socket, local Pironman API (127.0.0.1:34001), existing Ultron auth/session/database.

- Success metrics:
- 0 stuck busy states; p95 apply under 2s when API healthy; idle CPU budget maintained (ultron-ap <=2%, helper <=1%).; Baseline: No hardware module in core; optional diagnostics endpoint exists; Pironman/influx often inactive by default.

- Assumptions:
- Pironman API endpoints remain stable and reachable when service is active; helper queue/cancel model remains deterministic under burst input.; Why now: Need stable, secure hardware control without regressing core after full stabilization and decoupling.

- Interview mode:
- deep

## 3. Scope
### In scope
- Feature-flagged optional module, hardware read/apply endpoints, capability state mapping, operation status model, on-demand diagnostics, audit telemetry.

### Out of scope
- Global UI redesign, automatic background polling for hardware, host service lifecycle operations, changes to non-hardware modules.; No-go zone: Never reintroduce privileged direct execution from web process, and never force-start Pironman services from Ultron core.

## 4. Actors & User Journeys
Actors:
- Admin operators on Raspberry Pi 5, platform maintainer, and security owner.

Primary journey:
- Open optional hardware module, inspect state, modify values, press Apply once, receive deterministic success/failure with audit trace.

## 5. Architecture (Architect Persona)
- Components:
-
- Data flow:
-
- Key decisions:
-
- Risks:
-

## 6. Security (Security Persona)
- Threats:
-
- Controls required:
-
- Validation rules:
-

## 7. Backlog Outline
Epic:
-

User stories:
1.
2.
3.

## 8. Test Strategy
- Smoke tests:
-
- Functional tests:
-
- Security tests:
-
- Edge cases:
-

## 9. Discovery Confidence
- Confidence:
-

- Reason:
-

- Evidence gaps:
-

- Handoff decision:
-
