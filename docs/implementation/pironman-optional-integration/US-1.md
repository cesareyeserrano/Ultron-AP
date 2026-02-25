# Implementation Brief: US-1

Feature: pironman-optional-integration
Story: As a worker, I want to implement US-1.
Trace: FR-1, AC-2

## 1. Feature Context
- Integracion opcional de Pironman en Ultron, desacoplada del core, con control de hardware estable y seguro estilo Home Assistant.
- Primary actor: Admin operator de Ultron, maintainer de plataforma, y security owner.
- Expected outcome: El operador puede ver estado de integracion (available/unavailable/degraded), leer configuracion Pironman y aplicar cambios de forma determinista sin bloqueos; si falla, Ultron sigue operativo y muestra error accionable.

## 2. Acceptance Criteria
- Given `ULTRON_FEATURE_PIRONMAN=false`, when user navigates core, then no optional Pironman route/action is exposed.
- Given `ULTRON_FEATURE_PIRONMAN=true`, when user opens settings, then optional module entrypoint is available.

## 3. Test Cases to Satisfy
- TC-1: Feature flag disabled path (Trace FR: FR-1)
- TC-2: Feature flag enabled route (Trace FR: FR-1)

## 4. Scaffold References
- Interface: internal/contracts/fr-1-the-optional-pironman-module-mus.go
- Test stub: tests/pironman-optional-integration/generated/tc-1_feature-flag-disabled-path_test.go

## 5. Dependency Notes
- Order rationale: No previous story dependency
- Plan sequence hint: Follow IMPLEMENTATION_ORDER.md from this command.
- Plan dependency hint: Use scaffold interfaces as non-breaking contracts.

## 6. Quality Constraints
- Domain profile: Not specified
- Stack constraint: Not specified
- Forbidden defaults: Not specified
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.

