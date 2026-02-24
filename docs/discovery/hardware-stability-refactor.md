# Discovery: hardware-stability-refactor

STATUS: DRAFT

## 1. Problem Statement
Derived from approved spec retrieval snapshot:
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria

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

### Security snapshot
- Keep privileged operations only in dedicated helper boundary; web process must not execute `sudo` or direct privileged commands.
- Enforce parameter allowlists and input validation before any privileged action.
- Maintain CSRF protection on hardware apply endpoint and authenticated session checks.

### Out-of-scope snapshot
- UI redesign unrelated to hardware operation reliability.
- Broad migration of non-hardware modules.
- New user-facing product features outside hardware stability/security scope.

Refined problem framing:
- What problem are we solving? Problem stated in approved spec context
- Why now? Acceptance criteria defined in approved spec

## 2. Discovery Interview Summary (Discovery Persona)
- Primary users:
- Users defined in approved spec

- Jobs to be done:
- Deliver capability described in approved spec

- Current pain:
- Problem stated in approved spec context

- Constraints (business/technical/compliance):
- Constraints identified in approved spec

- Dependencies:
- Dependencies identified in approved spec

- Success metrics:
- Acceptance criteria defined in approved spec

- Assumptions:
- Assumptions embedded in approved spec scope

- Interview mode:
- quick

## 3. Scope
### In scope
- Approved spec functional scope

### Out of scope
- Anything not explicitly stated in approved spec

## 4. Actors & User Journeys
Actors:
- Users defined in approved spec

Primary journey:
- Primary journey derived from approved spec context

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
