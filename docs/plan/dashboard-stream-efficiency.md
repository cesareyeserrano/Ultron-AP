# Plan: dashboard-stream-efficiency

STATUS: DRAFT

## 1. Intent (from approved spec)
- Retrieval mode: section-level

### Context snapshot
- Optimize Ultron dashboard streaming for Raspberry Pi: move high-frequency dashboard updates to lightweight SSE JSON channels, keep UX fluid, add per-channel update cadence (metrics fast, charts medium, services slower), preserve security boundaries and low resource footprint.
- ---

### Actors snapshot
- Raspberry Pi Operator: uses Ultron dashboard to monitor system state in real time.
- Platform Maintainer: keeps Ultron resource usage low and verifies production stability.

### Functional rules snapshot
- Dashboard stream path must support lightweight updates for high-frequency cards.
- Dashboard must include temperature history chart.
- Temperature indicator and temperature chart must expose 3 visual states.
- SSE broadcast must avoid unnecessary heavy updates.

### Acceptance criteria snapshot
- Given an authenticated operator on dashboard, when SSE stream runs, then metrics remain frequently updated and charts update at a slower cadence.
- Given temperature samples in collector history, when charts render, then temperature history chart is visible.
- Given temperature value in normal/warning/high range, when dashboard renders indicator and chart, then colors are green/yellow/red respectively.
- Given active SSE clients, when chart cadence is reduced, then non-chart dashboard updates continue without waiting for chart partials.

### Security snapshot
- Keep existing privilege separation: web process remains unprivileged and does not execute host-privileged commands.
- Do not introduce new external endpoints or unauthenticated routes for dashboard stream changes.
- Keep server-side template rendering safe; avoid eval/dynamic script injection.

### Out-of-scope snapshot
- Replacing SSE with WebSockets.
- Introducing heavy frontend frameworks or charting dependencies.
- Changing Docker/Systemd/Pironman control permissions.

### Retrieval metadata
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria
- Summary:
-
- Success looks like:
-

## 2. Discovery Review (Discovery Persona)
### Problem framing
- Problem stated in approved spec context
- Core rule to preserve: Dashboard stream path must support lightweight updates for high-frequency cards. Metrics panel and chart panel updates must be independently schedulable.

### Constraints and dependencies
- Constraints: Constraints identified in approved spec
- Dependencies: Dependencies identified in approved spec

### Success metrics
- Acceptance criteria defined in approved spec

### Key assumptions
- Assumptions embedded in approved spec scope

### Discovery rigor profile
- Discovery interview mode: quick
- Planning policy: Plan a constrained first slice and keep assumptions explicit.
- Follow-up gate: Before broad implementation, re-run discovery in standard/deep mode if assumptions remain unresolved.

## 3. Scope
### In scope
-

### Out of scope
-

## 4. Product Review (Product Persona)
### Business value
- Address user pain by enforcing: Dashboard stream path must support lightweight updates for high-frequency cards. Metrics panel and chart panel updates must be independently schedulable.
- Secondary value from supporting rule: Dashboard must include temperature history chart. Temperature chart must use last collected samples from server collector history.

### Success metric
- Primary KPI: Acceptance criteria defined in approved spec
- Ship only if metric has baseline and target.

### Assumptions to validate
- Assumptions embedded in approved spec scope
- Validate dependency and constraint impact before implementation start.
- Discovery rigor policy: Before broad implementation, re-run discovery in standard/deep mode if assumptions remain unresolved.

## 5. Architecture (Architect Persona)
### Components
- Go HTTP handler or CLI entrypoint
- Go package: dashboard-stream-efficiency-service
- Go package: auth-service

### Data flow
- Request enters Go handler and is validated at boundary.
- Service package applies FR rules and coordinates adapters.
- Storage/integration package persists data and returns typed results.

### Key decisions
- Keep FR to implementation traceability explicit by preserving story and TC identifiers.
- Use Go service aligned with detected stack (Go).
- Favor deterministic error paths over silent fallback behavior.

### Risks & mitigations
- Spec-to-code drift risk: enforce FR/US/TC traces in generated artifacts.
- Integration fragility risk: isolate external calls behind adapters with clear contracts.
- Scope drift risk: block changes not linked to approved FR/AC entries.

### Observability (logs/metrics/tracing)
- Context-aware logs with request identifiers.
- Metrics for route latency and failure classes.
- Tracing spans for external calls.

### Domain quality profile
- Domain: Web/SaaS (web)
- Stack constraint: Use a component-based UI stack (for example React + Tailwind/shadcn or equivalent). Avoid raw static HTML/CSS-only scaffolds.
- Forbidden defaults: Raw HTML tables, default browser typography, and layout-only placeholders as final UI baseline.

## 6. Security (Security Persona)
### Threats
- Review spec for domain-specific threat model.
- Derived from spec security section: - Keep existing privilege separation: web process remains unprivileged and does not execute host-privileged commands.

### Required controls
- - Keep existing privilege separation: web process remains unprivileged and does not execute host-privileged commands.
- Do not introduce new external endpoints or unauthenticated routes for dashboard stream changes.
- Keep server-side template rendering safe; avoid eval/dynamic script injection.

### Validation rules
- Security controls must be verified before delivery gate.

## 7. UX/UI Review (UX/UI Persona, if user-facing)
### Primary user flow
- Flow must be explicit and testable.

### Key states (empty/loading/error/success)
- Define deterministic behavior for empty/loading/error/success states.

### Accessibility baseline
- Keyboard and screen-reader baseline for user-facing interactions.

### Asset and placeholder strategy
- Use credible placeholder/image/icon sources (for example placehold.co, Lucide/Heroicons) and define an explicit fallback strategy.
- Avoid default primitive-only output when domain requires visual fidelity.

## 8. Backlog
> Create as many epics/stories as needed. Do not impose artificial limits.

### Epics
- Epic 1:
  - Outcome:
  - Notes:
- Epic 2:
  - Outcome:
  - Notes:

### User Stories
For each story include clear Acceptance Criteria (Given/When/Then).

#### Story:
- As a <actor>, I want <capability>, so that <benefit>.
- Acceptance Criteria:
  - Given ..., when ..., then ...
  - Given ..., when ..., then ...

(repeat as needed)

## 9. Test Cases (QA Persona)
> Create as many test cases as needed. Include negative and edge cases.

### Functional
1.
2.

### Negative / Abuse
1.
2.

### Security
1.
2.

### Edge cases
1.
2.

## 10. Implementation Notes (Developer Persona)
- Suggested sequence:
-
- Dependencies:
-
- Rollout / fallback:
-
