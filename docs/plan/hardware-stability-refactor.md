# Plan: hardware-stability-refactor

STATUS: DRAFT

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
- Summary:
-
- Success looks like:
-

## 2. Discovery Review (Discovery Persona)
### Problem framing
- Problem stated in approved spec context
- Core rule to preserve: Hardware settings updates must be explicitly user-triggered (no auto-apply on field change), and only one apply operation can execute at a time.

### Constraints and dependencies
- Constraints: Constraints identified in approved spec
- Dependencies: Dependencies identified in approved spec

### Success metrics
- Acceptance criteria defined in approved spec

### Key assumptions
- Assumptions embedded in approved spec scope

### Discovery rigor profile
- Discovery interview mode: quick
- Planning policy: Plan a constrained first slice and keep assumptions explicit.
- Follow-up gate: Before broad implementation, re-run discovery in standard/deep mode if assumptions remain unresolved.

## 3. Scope
### In scope
-

### Out of scope
-

## 4. Product Review (Product Persona)
### Business value
- Address user pain by enforcing: Hardware settings updates must be explicitly user-triggered (no auto-apply on field change), and only one apply operation can execute at a time.
- Secondary value from supporting rule: The hardware apply pipeline must expose deterministic operation state (`idle|applying|failed|applied`) so UI and logs never leave ambiguous "busy" behavior.

### Success metric
- Primary KPI: Acceptance criteria defined in approved spec
- Ship only if metric has baseline and target.

### Assumptions to validate
- Assumptions embedded in approved spec scope
- Validate dependency and constraint impact before implementation start.
- Discovery rigor policy: Before broad implementation, re-run discovery in standard/deep mode if assumptions remain unresolved.

## 5. Architecture (Architect Persona)
### Components
- CLI command parser
- Command handler service
- Module: hardware-stability-refactor-service
- Module: auth-service
- Module: account-repository

### Data flow
- Operator executes command with validated inputs.
- Service layer enforces FR logic and delegates to adapters.
- Result is persisted/emitted with deterministic status and error text.

### Key decisions
- Keep FR to implementation traceability explicit by preserving story and TC identifiers.
- Use Node.js CLI modules aligned with detected stack (Node.js CLI).
- Favor deterministic error paths over silent fallback behavior.

### Risks & mitigations
- Spec-to-code drift risk: enforce FR/US/TC traces in generated artifacts.
- Integration fragility risk: isolate external calls behind adapters with clear contracts.
- Scope drift risk: block changes not linked to approved FR/AC entries.

### Observability (logs/metrics/tracing)
- Structured command logs with feature and story IDs.
- Metrics for command success/failure and runtime duration.
- Trace markers for dependency boundaries.

### Domain quality profile
- Domain: Web/SaaS (web)
- Stack constraint: Use a component-based UI stack (for example React + Tailwind/shadcn or equivalent). Avoid raw static HTML/CSS-only scaffolds.
- Forbidden defaults: Raw HTML tables, default browser typography, and layout-only placeholders as final UI baseline.

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
- Flow must be explicit and testable.

### Key states (empty/loading/error/success)
- Define deterministic behavior for empty/loading/error/success states.

### Accessibility baseline
- Keyboard and screen-reader baseline for user-facing interactions.

### Asset and placeholder strategy
- Use credible placeholder/image/icon sources (for example placehold.co, Lucide/Heroicons) and define an explicit fallback strategy.
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
