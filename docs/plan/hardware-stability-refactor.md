# Plan: hardware-stability-refactor

STATUS: READY

## Operative Note (2026-02-26)
- This plan includes historical phases and decisions.
- Final enforced posture is monitor-only: no Pironman control in Ultron core/runtime/helper APIs.
- External Pironman service remains managed outside Ultron via `:34001`.

## 1. Intent (from approved spec)
- Retrieval mode: section-level

### Context snapshot
- Summary (provided by user): Refactor del modulo hardware de Ultron para eliminar bloqueos/busy/timeouts, usando integracion estable con Pironman y flujo UX controlado. Restricciones no negociables: consumo de recursos casi imperceptible en Raspberry Pi 5 (CPU/RAM/IO) y seguridad fuerte (least-privilege, boundaries, auditabilidad).
- Requirement source: provided explicitly by user via --idea.
- No inferred requirements were added by Aitri.

### Actors snapshot
- Admin operator (authenticated Ultron user managing hardware settings).
- Ultron web process (unprivileged app process).
- Ultron privileged helper (root-owned local boundary for hardware operations).

### Functional rules snapshot
- Hardware settings updates must be explicitly user-triggered (no auto-apply on field change), and only one apply operation can execute at a time.
- The hardware apply pipeline must expose deterministic operation state (`idle|applying|failed|applied`) so UI and logs never leave ambiguous "busy" behavior.
- Integration with Pironman must prioritize stable local control path and avoid long blocking calls in web request lifecycle.
- Privileged execution must stay outside web process, with strict local boundary, validated command parameters, and auditable outcomes.
- Every hardware apply request must produce traceable telemetry (request id, duration, result, error cause) for incident diagnosis.

### Acceptance criteria snapshot
- Given an authenticated admin on hardware page, when fields are edited, then no apply request is sent until explicit apply action is triggered.
- Given one apply operation in progress, when another apply is requested, then system handles it deterministically without stuck-busy state.
- Given helper or Pironman timeout/error, when apply fails, then UI receives actionable failure status and system returns to operable idle state.
- Given normal operation on Pi5, when repeated hardware applies are performed, then resource usage remains lightweight and response behavior stays stable.
- Given security posture validation, when hardware apply executes, then privileged operations are bounded to helper path with auditable logs and no direct web-process privilege escalation.

### Security snapshot
- Keep privileged operations only in dedicated helper boundary; web process must not execute `sudo` or direct privileged commands.
- Enforce parameter allowlists and input validation before any privileged action.
- Maintain CSRF protection on hardware apply endpoint and authenticated session checks.

### Out-of-scope snapshot
- UI redesign unrelated to hardware operation reliability.
- Broad migration of non-hardware modules.
- New user-facing product features outside hardware stability/security scope.

### Retrieval metadata
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria
- Summary: Refactor de estabilidad del modulo hardware con prioridad en control determinista de operaciones, bajo consumo en Pi5 y seguridad por boundary privilegiado.
- Success looks like: sin `busy` atascado, sin auto-apply, sin privilegios directos en web process, y evidencia trazable por cada apply.

## 2. Discovery Review (Discovery Persona)
### Problem framing
- Problem stated in approved spec context
- Core rule to preserve: Hardware settings updates must be explicitly user-triggered (no auto-apply on field change), and only one apply operation can execute at a time.

### Constraints and dependencies
- Constraints:
- Mantener Ultron liviano en Pi5: evitar loops de polling agresivo, evitar spawning redundante de procesos, y minimizar IO.
- Mantener arquitectura segura: `NoNewPrivileges=true` en web process, helper privilegiado único con allowlist/validación.
- Dependencias:
- Ultron helper (`/run/ultron-helper.sock`), Pironman runtime local, endpoints web hardware con CSRF.

### Success metrics
- 0 incidentes de `pironman apply busy` atascado en pruebas repetidas.
- 0 uso de `sudo` directo en código del web process.
- Flujo UX determinista: editar -> apply -> estado final (success/fail) sin bloqueo de UI.
- Huella de recursos estable durante operaciones repetidas de hardware en Pi5.

### Key assumptions
- Pironman puede presentar latencia variable, por lo que el contrato debe desacoplar request HTTP de operación hardware prolongada.
- El operador acepta un flujo explícito de apply en lugar de auto-apply por campo.

### Discovery rigor profile
- Discovery interview mode: quick
- Planning policy: Plan a constrained first slice and keep assumptions explicit.
- Follow-up gate: Before broad implementation, re-run discovery in standard/deep mode if assumptions remain unresolved.

## 3. Scope
### In scope
- Refactor del pipeline hardware apply (backend + helper + UX de formulario).
- Control de concurrencia determinista y estado de operación.
- Observabilidad de hardware apply (request id, duración, resultado, error).
- Endurecimiento de boundary privilegiado y validación de parámetros.

### Out of scope
- Rediseño visual integral del panel.
- Cambios funcionales fuera del módulo hardware.
- Migraciones tecnológicas amplias no requeridas por estabilidad/seguridad.

## 4. Product Review (Product Persona)
### Business value
- Address user pain by enforcing: Hardware settings updates must be explicitly user-triggered (no auto-apply on field change), and only one apply operation can execute at a time.
- Secondary value from supporting rule: The hardware apply pipeline must expose deterministic operation state (`idle|applying|failed|applied`) so UI and logs never leave ambiguous "busy" behavior.

### Success metric
- Primary KPI: operación hardware estable bajo ráfaga de cambios sin atascos ni degradación de UX.
- Gate de salida: pruebas funcionales + seguridad + rendimiento en Pi5 aprobadas.

### Assumptions to validate
- Validar si Pironman API local está disponible de forma consistente para reducir dependencia de CLI.
- Validar latencia media/p95 de apply con carga normal de operación.
- Confirmar que no existe regresión de consumo en módulos no hardware.

## 5. Architecture (Architect Persona)
### Components
- Hardware form UI (explicit apply action).
- Hardware apply handler (request contract + CSRF + sync semantics).
- Privileged helper client (IPC over Unix socket).
- Privileged helper server (serialized hardware execution + timeout/cancel cleanup).
- Pironman adapter (preferred stable path + fallback policy).
- Audit/telemetry layer for hardware operations.

### Data flow
- Operator edita parámetros localmente en UI.
- Operator dispara `Apply` explícito (1 request).
- Handler valida sesión/CSRF y crea `request_id`.
- Handler envía comando al helper por socket local.
- Helper valida payload, ejecuta apply controlado y retorna estado.
- Handler responde con resultado y estado consistente; logs/auditoría quedan registrados.

### Key decisions
- Keep FR to implementation traceability explicit by preserving story and TC identifiers.
- Use Node.js CLI modules aligned with detected stack (Node.js CLI).
- Favor deterministic error paths over silent fallback behavior.

### Risks & mitigations
- Spec-to-code drift risk: enforce FR/US/TC traces in generated artifacts.
- Integration fragility risk: isolate external calls behind adapters with clear contracts.
- Scope drift risk: block changes not linked to approved FR/AC entries.

### Observability (logs/metrics/tracing)
- Log estructurado por apply: `request_id`, `duration_ms`, `result`, `error`.
- Métrica de tasa de éxito/fallo y latencia p50/p95.
- Señales explícitas de timeout/cancel y estado final del lock de ejecución.

### Domain quality profile
- Domain: Raspberry Pi Admin Panel (HTMX + Go server-side rendering).
- Constraint: minimizar coste runtime; evitar complejidad frontend innecesaria.
- Forbidden defaults: auto-apply por cambios de campo y flujos no deterministas.

## 6. Security (Security Persona)
### Threats
- Review spec for domain-specific threat model.
- Derived from spec security section: - Keep privileged operations only in dedicated helper boundary; web process must not execute `sudo` or direct privileged commands.

### Required controls
- - Keep privileged operations only in dedicated helper boundary; web process must not execute `sudo` or direct privileged commands.
- Enforce parameter allowlists and input validation before any privileged action.
- Maintain CSRF protection on hardware apply endpoint and authenticated session checks.

### Validation rules
- Security controls must be verified before delivery gate.

## 7. UX/UI Review (UX/UI Persona, if user-facing)
### Primary user flow
- Editar configuración -> presionar Apply -> ver estado `applying` -> resultado `applied` o `failed` con mensaje accionable.

### Key states (empty/loading/error/success)
- `idle`: formulario editable.
- `applying`: botón bloqueado y feedback visual.
- `applied`: confirmación + formulario operativo.
- `failed`: error visible + formulario recuperable sin recarga completa.

### Accessibility baseline
- Submit por teclado, foco preservado tras respuesta, mensajes de estado legibles por lectores.

### Asset and placeholder strategy
- Reusar componentes y estilos actuales del panel; no introducir assets pesados.

## 8. Backlog
> Create as many epics/stories as needed. Do not impose artificial limits.

## 8.1 Official Phased Execution Plan (Aitri-aligned)
- Phase 1 (Completed): Core decoupling from visible hardware module flow.
  - Delivered: hardware removed from main navigation and default operator flow.
  - Verification: core regression tests pass.
- Phase 2 (Completed): hard removal of hardware module from core runtime.
  - Target: remove hardware handlers/routes/templates and core dependency chain.
  - Verification: no core runtime reference to hardware module; contracts/tests updated to enforce decoupling and green.
- Phase 3 (Cancelled by product decision): optional external Pironman integration.
  - Target: plugin/adaptor pattern with explicit capability states and fail-fast behavior.
  - Decision: do not ship Pironman control in Ultron. Keep Pironman external (`:34001`) and monitor only from Ultron.
- Phase 4 (Completed): policy and operations hardening.
  - Target: document non-intrusive host policy and add per-module resource attribution diagnostics.
  - Verification: operator docs updated and on-demand CPU/RAM process snapshot visible from settings diagnostics.

### Epics
- EP-1: Flujo determinista de apply y UX estable.
  - Outcome: cero auto-apply, cero bloqueo por ráfagas, estado claro al usuario.
  - Notes: incluye sync en formulario y control single-flight.
- EP-2: Boundary privilegiado robusto y seguro.
  - Outcome: ejecución hardware segura fuera del web process con validación y auditoría.
  - Notes: incluye timeout/cancel cleanup y trazabilidad.
- EP-3: Optimización de recursos y observabilidad.
  - Outcome: comportamiento estable en Pi5 con consumo casi imperceptible y métricas de control.
  - Notes: incluye pruebas de regresión de latencia/huella.

### User Stories
For each story include clear Acceptance Criteria (Given/When/Then).

#### US-1 Explicit Apply
- As an Admin operator, I want hardware changes to apply only when I press Apply, so that I avoid accidental multi-requests.
- Acceptance Criteria:
  - Given edited fields, when no apply action is triggered, then no hardware apply request is sent.
  - Given edited fields, when Apply is pressed, then exactly one apply request starts.

#### US-2 Deterministic Operation State
- As an Admin operator, I want clear `idle|applying|failed|applied` state, so that I can operate without ambiguity.
- Acceptance Criteria:
  - Given an apply in progress, when another apply is requested, then behavior is deterministic and never leaves stuck-busy state.
  - Given apply completion, when response returns, then UI state transitions to `applied` or `failed`.

#### US-3 Secure Privileged Boundary
- As a Security owner, I want privileged hardware execution isolated in helper boundary, so that web process remains unprivileged.
- Acceptance Criteria:
  - Given hardware apply execution, when command runs, then no direct sudo/privileged exec happens from web process.
  - Given invalid parameters, when helper receives payload, then request is rejected and audited.

#### US-4 Timeout and Cleanup Reliability
- As a Platform operator, I want timeout/cancel to fully release resources and lock state, so that next apply can run normally.
- Acceptance Criteria:
  - Given an apply timeout, when operation is canceled, then all child processes are cleaned and lock is released.
  - Given subsequent apply request, when previous timed out, then new request can execute successfully.

#### US-5 Pi5 Lightweight Runtime
- As a Product owner, I want near-imperceptible resource impact, so that Ultron remains lightweight on Pi5.
- Acceptance Criteria:
  - Given repeated applies, when monitoring CPU/RAM/IO, then no sustained abnormal increase is introduced by refactor.
  - Given idle hardware page, when no apply is active, then background work remains minimal.

#### US-6 Audit and Telemetry
- As a Maintainer, I want traceable per-apply telemetry, so that incidents are diagnosable quickly.
- Acceptance Criteria:
  - Given an apply request, when processing completes, then logs include request id, duration, result, and error cause when applicable.
  - Given failure scenarios, when reviewing logs, then root cause is identifiable without guesswork.

## 9. Test Cases (QA Persona)
> Create as many test cases as needed. Include negative and edge cases.

### Functional
1. Validate explicit apply contract (no auto-request on field edits).
2. Validate deterministic state transitions and no stuck-busy.
3. Validate helper execution success/failure mapping to UI responses.

### Negative / Abuse
1. Repeated rapid apply clicks under active execution.
2. Invalid payload values rejected by helper allowlist.
3. Helper unavailable/socket error handling without UI deadlock.

### Security
1. Verify no direct privileged execution from web process.
2. Verify CSRF/session enforcement on hardware apply endpoint.
3. Verify audit trail on denied/failed privileged operations.

### Edge cases
1. Pironman high-latency behavior with timeout/cancel.
2. Partial failure after prior successful apply.
3. Recovery path after helper restart during active session.

## 10. Implementation Notes (Developer Persona)
- Suggested sequence:
- S1: Frontend explicit-apply contract + sync semantics.
- S2: Helper single-flight + timeout/cancel cleanup hardening.
- S3: Operation-state endpoint + UI state machine.
- S4: Telemetry/audit enrichment + Pi5 regression checks.
- Dependencies:
- Helper service, Pironman runtime, HTMX form contracts, action logs.
- Rollout / fallback:
- Deploy in canary on Pi host, keep rollback binary/service unit available.
