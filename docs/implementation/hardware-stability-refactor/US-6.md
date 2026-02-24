# Implementation Brief: US-6

Feature: hardware-stability-refactor
Story: As a Platform maintainer, I want timeout/cancel paths to always release execution locks and child processes, so that the next apply request can run without browser restart.
Trace: FR-2, FR-3, AC-2, AC-3

## 1. Feature Context
- Summary (provided by user): Refactor del modulo hardware de Ultron para eliminar bloqueos/busy/timeouts, usando integracion estable con Pironman y flujo UX controlado. Restricciones no negociables: consumo de recursos casi imperceptible en Raspberry Pi 5 (CPU/RAM/IO) y seguridad fuerte (least-privilege, boundaries, auditabilidad).
- Requirement source: provided explicitly by user via --idea.
- No inferred requirements were added by Aitri.

## 2. Acceptance Criteria
- Given an apply timeout, when cancellation is triggered, then lock state is released and helper is ready for next request.
- Given a timed-out apply, when a new apply is submitted, then system processes it normally without stale busy state.

## 3. Test Cases to Satisfy
- TC-10: Validate timeout/cancel cleanup unlock behavior. (Trace FR: FR-2, FR-3)

## 4. Scaffold References
- Interface: src/contracts/fr-2-the-hardware-apply-pipeline-must.js
- Interface: src/contracts/fr-3-integration-with-pironman-must-p.js
- Test stub: tests/hardware-stability-refactor/generated/tc-6-handle-edge-behavior-user-modifies-m.test.mjs

## 5. Dependency Notes
- Order rationale: Implement after US-1, US-2, US-3, US-4, US-5
- Plan sequence hint: - S1: Frontend explicit-apply contract + sync semantics. - S2: Helper single-flight + timeout/cancel cleanup hardening. - S3: Operation-state endpoint + UI state machine. - S4: Telemetry/audit enrichment + Pi5 regression checks.
- Plan dependency hint: - Helper service, Pironman runtime, HTMX form contracts, action logs.

## 6. Quality Constraints
- Domain profile: Raspberry Pi Admin Panel (HTMX + Go server-side rendering).
- Stack constraint: Not specified
- Forbidden defaults: auto-apply por cambios de campo y flujos no deterministas.
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

