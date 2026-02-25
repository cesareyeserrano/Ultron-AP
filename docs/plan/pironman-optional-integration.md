# Plan: pironman-optional-integration

STATUS: DRAFT

## 1. Intent (from approved spec)
- Retrieval mode: section-level

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

### Acceptance criteria snapshot
- Given un admin autenticado en el modulo opcional de hardware, when edita valores y presiona Apply, then se ejecuta una sola operacion, el estado transiciona a applied o failed, y la UI nunca queda en busy permanente.
- Given el feature flag de Pironman esta deshabilitado, when el usuario navega por Ultron core, then no se muestra modulo hardware ni se ejecutan operaciones Pironman.
- Given Pironman API falla o excede timeout, when se solicita read/apply, then la integracion reporta `degraded` o `unavailable` y Ultron core permanece operativo.
- Given una solicitud invalida de parametros hardware, when llega al helper, then se rechaza y queda registro auditable del rechazo.

### Security snapshot
- Mantener privilegios fuera del web process: helper root como unico boundary, validacion estricta de parametros, CSRF/session obligatorios y logs auditables.

### Out-of-scope snapshot
- No rediseño visual global, no gestion de lifecycle de servicios Pironman/Influx desde core, no cambios en modulos no hardware.

### Retrieval metadata
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria
- Summary:
-
- Success looks like:
-

## 2. Discovery Review (Discovery Persona)
### Problem framing
- Current direct integration caused busy/timeout states and degraded UX; high severity because it blocks operator actions.
- Core rule to preserve: The optional Pironman module must run with explicit feature flag gating and default disabled mode.

### Constraints and dependencies
- Constraints: Keep Ultron lightweight on Pi5, no service lifecycle control from core, strict helper privilege boundary, CSRF/session enforcement, no regressions in core modules.
- Dependencies: ultron-helper over unix socket, local Pironman API (127.0.0.1:34001), existing Ultron auth/session/database.

### Success metrics
- 0 stuck busy states; p95 apply under 2s when API healthy; idle CPU budget maintained (ultron-ap <=2%, helper <=1%).; Baseline: No hardware module in core; optional diagnostics endpoint exists; Pironman/influx often inactive by default.

### Key assumptions
- Pironman API endpoints remain stable and reachable when service is active; helper queue/cancel model remains deterministic under burst input.; Why now: Need stable, secure hardware control without regressing core after full stabilization and decoupling.

### Discovery rigor profile
- Discovery interview mode: deep
- Planning policy: Plan for full decomposition (explicit risks, constraints, and dependency handling).
- Follow-up gate: No extra discovery depth required before implementation unless scope changes.

## 3. Scope
### In scope
-

### Out of scope
-

## 4. Product Review (Product Persona)
### Business value
- Address user pain by enforcing: The optional Pironman module must run with explicit feature flag gating and default disabled mode.
- Secondary value from supporting rule: Hardware settings must be applied only by explicit user action and only one apply operation can run at a time.

### Success metric
- Primary KPI: 0 stuck busy states; p95 apply under 2s when API healthy; idle CPU budget maintained (ultron-ap <=2%, helper <=1%).; Baseline: No hardware module in core; optional diagnostics endpoint exists; Pironman/influx often inactive by default.
- Ship only if metric has baseline and target.

### Assumptions to validate
- Pironman API endpoints remain stable and reachable when service is active; helper queue/cancel model remains deterministic under burst input.; Why now: Need stable, secure hardware control without regressing core after full stabilization and decoupling.
- Validate dependency and constraint impact before implementation start.
- Discovery rigor policy: No extra discovery depth required before implementation unless scope changes.

## 5. Architecture (Architect Persona)
### Components
- Go HTTP handler or CLI entrypoint
- Go package: pironman-optional-integration-service
- Go package: account-repository

### Data flow
- Request enters Go handler and is validated at boundary.
- Service package applies FR rules and coordinates adapters.
- Storage/integration package persists data and returns typed results.

### Key decisions
- Keep FR to implementation traceability explicit by preserving story and TC identifiers.
- Use Go service aligned with detected stack (Go).
- Favor deterministic error paths over silent fallback behavior.

### Risks & mitigations
- Spec-to-code drift risk: enforce FR/US/TC traces in generated artifacts.
- Integration fragility risk: isolate external calls behind adapters with clear contracts.
- Scope drift risk: block changes not linked to approved FR/AC entries.

### Observability (logs/metrics/tracing)
- Context-aware logs with request identifiers.
- Metrics for route latency and failure classes.
- Tracing spans for external calls.

### Domain quality profile
- Domain: Game/Interactive (game)
- Stack constraint: Use a rendering/game engine (for example Phaser or Three.js). Avoid raw primitive-only canvas logic as architecture baseline.
- Forbidden defaults: Rectangle-only or geometry-only output without asset pipeline.

## 6. Security (Security Persona)
### Threats
- Review spec for domain-specific threat model.
- Derived from spec security section: - Mantener privilegios fuera del web process: helper root como unico boundary, validacion estricta de parametros, CSRF/session obligatorios y logs auditables.

### Required controls
- - Mantener privilegios fuera del web process: helper root como unico boundary, validacion estricta de parametros, CSRF/session obligatorios y logs auditables.

### Validation rules
- Security controls must be verified before delivery gate.

## 7. UX/UI Review (UX/UI Persona, if user-facing)
### Primary user flow
- Flow must include complete state coverage and fallback paths.

### Key states (empty/loading/error/success)
- Define deterministic behavior for empty/loading/error/success states.

### Accessibility baseline
- Keyboard and screen-reader baseline for user-facing interactions.

### Asset and placeholder strategy
- Use external asset loading (sprites/GLTF/audio) with public-domain packs or placeholders and document fallback behavior.
- Avoid default primitive-only output when domain requires visual fidelity.

## 8. Backlog
> Create as many epics/stories as needed. Do not impose artificial limits.

### Epics
- Epic 1:
  - Outcome:
  - Notes:
- Epic 2:
  - Outcome:
  - Notes:

### User Stories
For each story include clear Acceptance Criteria (Given/When/Then).

#### Story:
- As a <actor>, I want <capability>, so that <benefit>.
- Acceptance Criteria:
  - Given ..., when ..., then ...
  - Given ..., when ..., then ...

(repeat as needed)

## 9. Test Cases (QA Persona)
> Create as many test cases as needed. Include negative and edge cases.

### Functional
1.
2.

### Negative / Abuse
1.
2.

### Security
1.
2.

### Edge cases
1.
2.

## 10. Implementation Notes (Developer Persona)
- Suggested sequence:
-
- Dependencies:
-
- Rollout / fallback:
-
