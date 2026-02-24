# Plan: total-stabilization

STATUS: DRAFT

## 1. Intent (from approved spec)
- Retrieval mode: section-level

### Context snapshot
- A deep total stabilization and optimization pass for Ultron-AP covering runtime reliability, security hardening, automated test expansion, and documentation hygiene optimization.
- Primary actor: Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.
- Expected outcome: The system remains stable under backup and operational failures with observable error reporting, critical security/session paths are strongly validated, all core tests pass, and project documentation is reduced to a clear, non-duplicated, maintained set with controlled file lengths.

### Actors snapshot
- Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.

### Functional rules snapshot
- The system must enforce a documentation asset strategy where only repository markdown and source files are used; no external media assets are introduced during stabilization and every referenced document path must resolve locally.

### Acceptance criteria snapshot
- Given the markdown inventory and reference scan, when documentation hygiene runs, then every markdown file is classified as keep/consolidate/archive/delete with justification, no broken internal links remain, and files above the defined max length are split or reduced.

### Security snapshot
- State-changing endpoints must enforce CSRF token checks plus same-origin protections (Origin/Referer policy), and session cookies must remain secure under direct TLS and trusted proxy TLS termination.

### Out-of-scope snapshot
- No visual redesign, no new end-user features, no migration away from Go/HTMX/SQLite, and no infrastructure platform change.

### Retrieval metadata
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria
- Summary:
-
- Success looks like:
-

## 2. Discovery Review (Discovery Persona)
### Problem framing
- Current pain is medium-high: some reliability and security paths had limited observability or test coverage, and documentation can drift with overlapping artifacts. Frequency is recurring during maintenance and release cycles.
- Core rule to preserve: The system must enforce a documentation asset strategy where only repository markdown and source files are used; no external media assets are introduced during stabilization and every referenced document path must resolve locally.

### Constraints and dependencies
- Constraints: Must keep existing Go monolith architecture and behavior, no new product features, no UI redesign, no infrastructure migration, and keep performance/resource footprint suitable for Raspberry Pi.
- Dependencies: Dependencies include SQLite storage, Docker daemon/socket, systemd CLI integration, Pironman5 CLI integration, and Telegram API integration for backup delivery.

### Success metrics
- go test ./... remains green; zero open P0 stabilization items; backup failure outcomes are explicitly logged/persisted; all targeted new reliability/security tests pass; 100% markdown files classified as keep/consolidate/archive/delete with rationale; zero broken internal markdown links.; Baseline: Current baseline: go test ./... is green; stabilization backlog BL0001 remains open with pending P0/P1/P2 actions; documentation classification (keep/consolidate/archive/delete) is not yet complete.

### Key assumptions
- Assume documentation cleanup can be done without breaking SDLC workflow references; assume backup observability changes will not increase operational noise excessively; assume CSRF origin checks can be implemented without blocking valid proxied requests.; Why now: Recent hardening work surfaced residual quality debt; closing it now reduces regression risk before further feature evolution and keeps maintenance cost low.

### Discovery rigor profile
- Discovery interview mode: deep
- Planning policy: Plan for full decomposition (explicit risks, constraints, and dependency handling).
- Follow-up gate: No extra discovery depth required before implementation unless scope changes.

## 3. Scope
### In scope
-

### Out of scope
-

## 4. Product Review (Product Persona)
### Business value
- Address user pain by enforcing: The system must enforce a documentation asset strategy where only repository markdown and source files are used; no external media assets are introduced during stabilization and every referenced document path must resolve locally.
- Secondary value from supporting rule: The system must enforce a documentation asset strategy where only repository markdown and source files are used; no external media assets are introduced during stabilization and every referenced document path must resolve locally.

### Success metric
- Primary KPI: go test ./... remains green; zero open P0 stabilization items; backup failure outcomes are explicitly logged/persisted; all targeted new reliability/security tests pass; 100% markdown files classified as keep/consolidate/archive/delete with rationale; zero broken internal markdown links.; Baseline: Current baseline: go test ./... is green; stabilization backlog BL0001 remains open with pending P0/P1/P2 actions; documentation classification (keep/consolidate/archive/delete) is not yet complete.
- Ship only if metric has baseline and target.

### Assumptions to validate
- Assume documentation cleanup can be done without breaking SDLC workflow references; assume backup observability changes will not increase operational noise excessively; assume CSRF origin checks can be implemented without blocking valid proxied requests.; Why now: Recent hardening work surfaced residual quality debt; closing it now reduces regression risk before further feature evolution and keeps maintenance cost low.
- Validate dependency and constraint impact before implementation start.
- Discovery rigor policy: No extra discovery depth required before implementation unless scope changes.

## 5. Architecture (Architect Persona)
### Components
- CLI command parser
- Command handler service
- Module: total-stabilization-service

### Data flow
- Operator executes command with validated inputs.
- Service layer enforces FR logic and delegates to adapters.
- Result is persisted/emitted with deterministic status and error text.

### Key decisions
- Keep FR to implementation traceability explicit by preserving story and TC identifiers.
- Use Node.js CLI modules aligned with detected stack (Node.js CLI).
- Favor deterministic error paths over silent fallback behavior.

### Risks & mitigations
- Spec-to-code drift risk: enforce FR/US/TC traces in generated artifacts.
- Integration fragility risk: isolate external calls behind adapters with clear contracts.
- Scope drift risk: block changes not linked to approved FR/AC entries.

### Observability (logs/metrics/tracing)
- Structured command logs with feature and story IDs.
- Metrics for command success/failure and runtime duration.
- Trace markers for dependency boundaries.

### Domain quality profile
- Domain: CLI/Automation (cli)
- Stack constraint: Use structured command modules and formatted terminal output (for example chalk/ora or equivalent patterns).
- Forbidden defaults: Unstructured raw console output as final UX baseline.

## 6. Security (Security Persona)
### Threats
- Review spec for domain-specific threat model.
- Derived from spec security section: - State-changing endpoints must enforce CSRF token checks plus same-origin protections (Origin/Referer policy), and session cookies must remain secure under direct TLS and trusted proxy TLS termination.

### Required controls
- - State-changing endpoints must enforce CSRF token checks plus same-origin protections (Origin/Referer policy), and session cookies must remain secure under direct TLS and trusted proxy TLS termination.

### Validation rules
- Security controls must be verified before delivery gate.

## 7. UX/UI Review (UX/UI Persona, if user-facing)
### Primary user flow
- Flow must include complete state coverage and fallback paths.

### Key states (empty/loading/error/success)
- Define deterministic behavior for empty/loading/error/success states.

### Accessibility baseline
- Keyboard and screen-reader baseline for user-facing interactions.

### Asset and placeholder strategy
- Define output templates/examples and fallback text for non-interactive logs.
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
