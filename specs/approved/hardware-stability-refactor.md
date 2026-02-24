# AF-SPEC: hardware-stability-refactor

STATUS: APPROVED
## 1. Context
Summary (provided by user): Refactor del modulo hardware de Ultron para eliminar bloqueos/busy/timeouts, usando integracion estable con Pironman y flujo UX controlado. Restricciones no negociables: consumo de recursos casi imperceptible en Raspberry Pi 5 (CPU/RAM/IO) y seguridad fuerte (least-privilege, boundaries, auditabilidad).
Requirement source: provided explicitly by user via --idea.
No inferred requirements were added by Aitri.

---

(Complete all requirement sections with explicit user-provided requirements before approve.)

## 2. Actors
- Admin operator (authenticated Ultron user managing hardware settings).
- Ultron web process (unprivileged app process).
- Ultron privileged helper (root-owned local boundary for hardware operations).
- Pironman runtime/components on Raspberry Pi 5.

## 3. Functional Rules (traceable)
Use stable IDs so stories/tests can reference them.

- FR-1: Hardware settings updates must be explicitly user-triggered (no auto-apply on field change), and only one apply operation can execute at a time.
- FR-2: The hardware apply pipeline must expose deterministic operation state (`idle|applying|failed|applied`) so UI and logs never leave ambiguous "busy" behavior.
- FR-3: Integration with Pironman must prioritize stable local control path and avoid long blocking calls in web request lifecycle.
- FR-4: Privileged execution must stay outside web process, with strict local boundary, validated command parameters, and auditable outcomes.
- FR-5: Every hardware apply request must produce traceable telemetry (request id, duration, result, error cause) for incident diagnosis.

## 4. Edge Cases
- User modifies many controls quickly before pressing apply.
- User sends repeated apply clicks while one apply is still running.
- Pironman runtime is available but responds slowly (> normal latency window).
- Pironman runtime/API is temporarily unavailable while hardware page is open.
- Hardware partial change succeeds but one field fails validation or application.

## 5. Failure Conditions
- Helper/service unreachable from web process.
- Apply request exceeds configured timeout and must be cancelled/cleaned.
- Invalid parameter payload rejected by allowlist/validation rules.
- Pironman execution path returns error and no state confirmation is possible.

## 6. Non-Functional Requirements
<!-- For visual features, replace this section with "## 6. UI Structure" to enable UI traceability:
## 6. UI Structure
Screen: <ScreenName>
Flow: <ScreenName> → <TargetScreen>
### References
- UI-REF-1: <path/to/mockup.png> → AC-1, AC-2
-->
- NFR-1 (Pi5 efficiency): Refactor must keep background resource impact near-imperceptible on Raspberry Pi 5 (no continuous polling loops, no high-frequency retries, no unnecessary process spawning).
- NFR-2 (latency): UI apply feedback should return deterministic status quickly and must not freeze interaction lifecycle.
- NFR-3 (stability): No deadlock/stuck-busy condition is allowed after timeout, cancellation, or backend errors.
- NFR-4 (security): Maintain least-privilege model (`NoNewPrivileges=true` on web process) and local privileged boundary enforcement.

## 7. Security Considerations
- Keep privileged operations only in dedicated helper boundary; web process must not execute `sudo` or direct privileged commands.
- Enforce parameter allowlists and input validation before any privileged action.
- Maintain CSRF protection on hardware apply endpoint and authenticated session checks.
- Record auditable logs for all hardware operations including denied/failed attempts.

## 8. Out of Scope
- UI redesign unrelated to hardware operation reliability.
- Broad migration of non-hardware modules.
- New user-facing product features outside hardware stability/security scope.

## 9. Acceptance Criteria (Given/When/Then)
- AC-1: Given an authenticated admin on hardware page, when fields are edited, then no apply request is sent until explicit apply action is triggered.
- AC-2: Given one apply operation in progress, when another apply is requested, then system handles it deterministically without stuck-busy state.
- AC-3: Given helper or Pironman timeout/error, when apply fails, then UI receives actionable failure status and system returns to operable idle state.
- AC-4: Given normal operation on Pi5, when repeated hardware applies are performed, then resource usage remains lightweight and response behavior stays stable.
- AC-5: Given security posture validation, when hardware apply executes, then privileged operations are bounded to helper path with auditable logs and no direct web-process privilege escalation.
## 10. Requirement Source Statement
- Requirements must be provided explicitly by the user.
- Aitri does not invent requirements.
