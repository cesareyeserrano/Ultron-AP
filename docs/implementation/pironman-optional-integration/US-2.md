# Implementation Brief: US-2

Feature: pironman-optional-integration
Story: As a worker, I want to implement US-2.
Trace: FR-2, AC-1

## 1. Feature Context
- Integracion opcional de Pironman en Ultron, desacoplada del core, con control de hardware estable y seguro estilo Home Assistant.
- Primary actor: Admin operator de Ultron, maintainer de plataforma, y security owner.
- Expected outcome: El operador puede ver estado de integracion (available/unavailable/degraded), leer configuracion Pironman y aplicar cambios de forma determinista sin bloqueos; si falla, Ultron sigue operativo y muestra error accionable.

## 2. Acceptance Criteria
- Given edited values, when Apply is not pressed, then no apply request is sent.
- Given active apply, when repeated apply clicks occur, then system remains deterministic and recoverable.

## 3. Test Cases to Satisfy
- TC-3: Apply endpoint CSRF/session guard (Trace FR: FR-2, FR-4)

## 4. Scaffold References
- Interface: internal/contracts/fr-2-hardware-settings-must-be-applie.go
- Test stub: tests/pironman-optional-integration/generated/tc-2_feature-flag-enabled-route_test.go

## 5. Dependency Notes
- Order rationale: Implement after US-1, US-3
- Plan sequence hint: Follow IMPLEMENTATION_ORDER.md from this command.
- Plan dependency hint: Use scaffold interfaces as non-breaking contracts.

## 6. Quality Constraints
- Domain profile: Not specified
- Stack constraint: Not specified
- Forbidden defaults: Not specified
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

