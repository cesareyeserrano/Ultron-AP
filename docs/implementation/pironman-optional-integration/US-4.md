# Implementation Brief: US-4

Feature: pironman-optional-integration
Story: As a worker, I want to implement US-4.
Trace: FR-4, AC-4

## 1. Feature Context
- Integracion opcional de Pironman en Ultron, desacoplada del core, con control de hardware estable y seguro estilo Home Assistant.
- Primary actor: Admin operator de Ultron, maintainer de plataforma, y security owner.
- Expected outcome: El operador puede ver estado de integracion (available/unavailable/degraded), leer configuracion Pironman y aplicar cambios de forma determinista sin bloqueos; si falla, Ultron sigue operativo y muestra error accionable.

## 2. Acceptance Criteria
- Given invalid payload values, when apply is requested, then input is sanitized/rejected and rejection is auditable.
- Given normal apply, when action executes, then privileged operation stays in helper path only.

## 3. Test Cases to Satisfy
- TC-4: Input sanitization (Trace FR: FR-4)

## 4. Scaffold References
- Interface: internal/contracts/fr-4-privileged-operations-must-remai.go
- Test stub: tests/pironman-optional-integration/generated/tc-4_input-sanitization_test.go

## 5. Dependency Notes
- Order rationale: Implement after US-1, US-3, US-2
- Plan sequence hint: Follow IMPLEMENTATION_ORDER.md from this command.
- Plan dependency hint: Use scaffold interfaces as non-breaking contracts.

## 6. Quality Constraints
- Domain profile: Not specified
- Stack constraint: Not specified
- Forbidden defaults: Not specified
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

