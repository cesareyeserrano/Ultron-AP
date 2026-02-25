# AF-SPEC: pironman-optional-integration

STATUS: DRAFT

## 1. Context
Integracion opcional de Pironman en Ultron, desacoplada del core, con control de hardware estable y seguro estilo Home Assistant.

Primary actor: Admin operator de Ultron, maintainer de plataforma, y security owner.
Expected outcome: El operador puede ver estado de integracion (available/unavailable/degraded), leer configuracion Pironman y aplicar cambios de forma determinista sin bloqueos; si falla, Ultron sigue operativo y muestra error accionable.
In scope: Modulo opcional por feature flag, API adapter Pironman, cola single-flight para apply, timeouts/cancelacion, telemetria por request, UI de hardware aislada, y diagnosticos de CPU/RAM por proceso.
Out of scope: No rediseño visual global, no gestion de lifecycle de servicios Pironman/Influx desde core, no cambios en modulos no hardware.
Technology: Go + HTMX + SQLite + ultron-helper via unix socket + Pironman API local.
Requirement source: Provided explicitly by user in guided draft.

## 2. Actors
- Admin operator de Ultron, maintainer de plataforma, y security owner.

## 3. Functional Rules (traceable)
- FR-1: El sistema debe aplicar cambios de hardware solo por accion explicita del usuario y ejecutar maximo una operacion apply activa a la vez.
- FR-2: La integracion debe exponer estados available/unavailable/degraded y nunca bloquear el core de Ultron cuando Pironman falle.

## 4. Edge Cases
- Timeout o caida de API Pironman durante apply, o clicks repetidos de apply en rafaga.

## 5. Failure Conditions
- TBD (refine during review)

## 6. Non-Functional Requirements
- TBD (refine during review)

## 7. Security Considerations
- Mantener privilegios fuera del web process: helper root como unico boundary, validacion estricta de parametros, CSRF/session obligatorios y logs auditables.

## 8. Out of Scope
- No rediseño visual global, no gestion de lifecycle de servicios Pironman/Influx desde core, no cambios en modulos no hardware.

## 9. Acceptance Criteria
- AC-1: Given un admin autenticado en el modulo opcional de hardware, when edita valores y presiona Apply, then se ejecuta una sola operacion, el estado transiciona a applied o failed, y la UI nunca queda en busy permanente.

## 10. Requirement Source Statement
- All requirements in this draft were provided explicitly by the user.
- Aitri structured the content and did not invent requirements.

## 11. Resource Strategy
- Sin polling continuo; diagnosticos solo on-demand; timeouts cortos; sin loops extra en idle; presupuesto objetivo en Pi5: ultron-ap <=2% CPU y ultron-helper <=1% CPU en reposo.
