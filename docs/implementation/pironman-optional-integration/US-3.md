# Implementation Brief: US-3

Feature: pironman-optional-integration
Story: As a worker, I want to implement US-3.
Trace: FR-3, AC-3

## 1. Feature Context
- Integracion opcional de Pironman en Ultron, desacoplada del core, con control de hardware estable y seguro estilo Home Assistant.
- Primary actor: Admin operator de Ultron, maintainer de plataforma, y security owner.
- Expected outcome: El operador puede ver estado de integracion (available/unavailable/degraded), leer configuracion Pironman y aplicar cambios de forma determinista sin bloqueos; si falla, Ultron sigue operativo y muestra error accionable.

## 2. Acceptance Criteria
- Given Pironman timeout, when read/apply runs, then status becomes `degraded` and core remains usable.
- Given helper/socket unavailable, when read/apply runs, then status becomes `unavailable`.

## 3. Test Cases to Satisfy
- TC-5: Timeout capability mapping (Trace FR: FR-3)
- TC-6: Socket unavailable mapping (Trace FR: FR-3)

## 4. Scaffold References
- Interface: internal/contracts/fr-3-integration-must-expose-capabili.go

## 5. Dependency Notes
- Order rationale: Implement after US-1
- Plan sequence hint: Follow IMPLEMENTATION_ORDER.md from this command.
- Plan dependency hint: Use scaffold interfaces as non-breaking contracts.

## 6. Quality Constraints
- Domain profile: Not specified
- Stack constraint: Not specified
- Forbidden defaults: Not specified
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

