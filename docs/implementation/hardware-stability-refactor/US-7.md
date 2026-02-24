# Implementation Brief: US-7

Feature: hardware-stability-refactor
Story: As a Product owner, I want hardware refactor to preserve lightweight Pi5 behavior, so that Ultron remains low-resource in production.
Trace: FR-3, AC-4

## 1. Feature Context
- Summary (provided by user): Refactor del modulo hardware de Ultron para eliminar bloqueos/busy/timeouts, usando integracion estable con Pironman y flujo UX controlado. Restricciones no negociables: consumo de recursos casi imperceptible en Raspberry Pi 5 (CPU/RAM/IO) y seguridad fuerte (least-privilege, boundaries, auditabilidad).
- Requirement source: provided explicitly by user via --idea.
- No inferred requirements were added by Aitri.

## 2. Acceptance Criteria
- Given repeated hardware applies, when resource metrics are sampled, then no sustained abnormal CPU/RAM/IO increase is introduced by refactor.
- Given idle hardware screen, when no apply is running, then background workload remains minimal.

## 3. Test Cases to Satisfy
- TC-11: Validate Pi5 lightweight resource profile under repeated applies. (Trace FR: FR-3)

## 4. Scaffold References
- Interface: src/contracts/fr-3-integration-with-pironman-must-p.js
- Test stub: tests/hardware-stability-refactor/generated/tc-7-handle-edge-behavior-user-sends-repe.test.mjs

## 5. Dependency Notes
- Order rationale: Implement after US-1, US-2, US-3, US-4, US-5, US-6
- Plan sequence hint: - S1: Frontend explicit-apply contract + sync semantics. - S2: Helper single-flight + timeout/cancel cleanup hardening. - S3: Operation-state endpoint + UI state machine. - S4: Telemetry/audit enrichment + Pi5 regression checks.
- Plan dependency hint: - Helper service, Pironman runtime, HTMX form contracts, action logs.

## 6. Quality Constraints
- Domain profile: Raspberry Pi Admin Panel (HTMX + Go server-side rendering).
- Stack constraint: Not specified
- Forbidden defaults: auto-apply por cambios de campo y flujos no deterministas.
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

