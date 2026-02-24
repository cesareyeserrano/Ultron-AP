# Implementation Brief: US-2

Feature: hardware-stability-refactor
Story: As a Admin operator (authenticated Ultron user managing hardware settings).
Trace: FR-2, AC-2

## 1. Feature Context
- Summary (provided by user): Refactor del modulo hardware de Ultron para eliminar bloqueos/busy/timeouts, usando integracion estable con Pironman y flujo UX controlado. Restricciones no negociables: consumo de recursos casi imperceptible en Raspberry Pi 5 (CPU/RAM/IO) y seguridad fuerte (least-privilege, boundaries, auditabilidad).
- Requirement source: provided explicitly by user via --idea.
- No inferred requirements were added by Aitri.

## 2. Acceptance Criteria
- Given one apply operation in progress, when another apply is requested, then system handles it deterministically without stuck-busy state.

## 3. Test Cases to Satisfy
- TC-2: Validate us-2 primary behavior. (Trace FR: FR-2)

## 4. Scaffold References
- Interface: src/contracts/fr-2-the-hardware-apply-pipeline-must.js
- Test stub: tests/hardware-stability-refactor/generated/tc-2-validate-us-2-primary-behavior.test.mjs

## 5. Dependency Notes
- Order rationale: Implement after US-1
- Plan sequence hint: - S1: Frontend explicit-apply contract + sync semantics. - S2: Helper single-flight + timeout/cancel cleanup hardening. - S3: Operation-state endpoint + UI state machine. - S4: Telemetry/audit enrichment + Pi5 regression checks.
- Plan dependency hint: - Helper service, Pironman runtime, HTMX form contracts, action logs.

## 6. Quality Constraints
- Domain profile: Raspberry Pi Admin Panel (HTMX + Go server-side rendering).
- Stack constraint: Not specified
- Forbidden defaults: auto-apply por cambios de campo y flujos no deterministas.
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

