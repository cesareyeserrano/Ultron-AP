# Implementation Brief: US-4

Feature: hardware-stability-refactor
Story: As a Ultron privileged helper (root-owned local boundary for hardware operations).
Trace: FR-4, AC-5

## 1. Feature Context
- Summary (provided by user): Refactor del modulo hardware de Ultron para eliminar bloqueos/busy/timeouts, usando integracion estable con Pironman y flujo UX controlado. Restricciones no negociables: consumo de recursos casi imperceptible en Raspberry Pi 5 (CPU/RAM/IO) y seguridad fuerte (least-privilege, boundaries, auditabilidad).
- Requirement source: provided explicitly by user via --idea.
- No inferred requirements were added by Aitri.

## 2. Acceptance Criteria
- Given security posture validation, when hardware apply executes, then privileged operations are bounded to helper path with auditable logs and no direct web-process privilege escalation.

## 3. Test Cases to Satisfy
- TC-4: Validate us-4 primary behavior. (Trace FR: FR-4)

## 4. Scaffold References
- Interface: src/contracts/fr-4-privileged-execution-must-stay-o.js
- Test stub: tests/hardware-stability-refactor/generated/tc-4-validate-us-4-primary-behavior.test.mjs

## 5. Dependency Notes
- Order rationale: Implement after US-1, US-2, US-3
- Plan sequence hint: - S1: Frontend explicit-apply contract + sync semantics. - S2: Helper single-flight + timeout/cancel cleanup hardening. - S3: Operation-state endpoint + UI state machine. - S4: Telemetry/audit enrichment + Pi5 regression checks.
- Plan dependency hint: - Helper service, Pironman runtime, HTMX form contracts, action logs.

## 6. Quality Constraints
- Domain profile: Raspberry Pi Admin Panel (HTMX + Go server-side rendering).
- Stack constraint: Not specified
- Forbidden defaults: auto-apply por cambios de campo y flujos no deterministas.
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

