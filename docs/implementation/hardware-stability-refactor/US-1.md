# Implementation Brief: US-1

Feature: hardware-stability-refactor
Story: As a Admin operator (authenticated Ultron user managing hardware settings).
Trace: FR-1, AC-1

## 1. Feature Context
- Summary (provided by user): Refactor del modulo hardware de Ultron para eliminar bloqueos/busy/timeouts, usando integracion estable con Pironman y flujo UX controlado. Restricciones no negociables: consumo de recursos casi imperceptible en Raspberry Pi 5 (CPU/RAM/IO) y seguridad fuerte (least-privilege, boundaries, auditabilidad).
- Requirement source: provided explicitly by user via --idea.
- No inferred requirements were added by Aitri.

## 2. Acceptance Criteria
- Given an authenticated admin on hardware page, when fields are edited, then no apply request is sent until explicit apply action is triggered.

## 3. Test Cases to Satisfy
- TC-1: Validate us-1 primary behavior. (Trace FR: FR-1)
- TC-6: Handle edge behavior - User modifies many controls quickly before pressing apply. (Trace FR: FR-1)
- TC-7: Handle edge behavior - User sends repeated apply clicks while one apply is still running. (Trace FR: FR-1)
- TC-8: Enforce security control - Keep privileged operations only in dedicated helper boundary; web process must not execute `sudo` or direct privileged commands. (Trace FR: FR-1)
- TC-9: Enforce security control - Enforce parameter allowlists and input validation before any privileged action. (Trace FR: FR-1)

## 4. Scaffold References
- Interface: src/contracts/fr-1-hardware-settings-updates-must-b.js
- Test stub: tests/hardware-stability-refactor/generated/tc-1-validate-us-1-primary-behavior.test.mjs

## 5. Dependency Notes
- Order rationale: No previous story dependency
- Plan sequence hint: - S1: Frontend explicit-apply contract + sync semantics. - S2: Helper single-flight + timeout/cancel cleanup hardening. - S3: Operation-state endpoint + UI state machine. - S4: Telemetry/audit enrichment + Pi5 regression checks.
- Plan dependency hint: - Helper service, Pironman runtime, HTMX form contracts, action logs.

## 6. Quality Constraints
- Domain profile: Raspberry Pi Admin Panel (HTMX + Go server-side rendering).
- Stack constraint: Not specified
- Forbidden defaults: auto-apply por cambios de campo y flujos no deterministas.
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

