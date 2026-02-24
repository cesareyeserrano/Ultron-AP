# Discovery: total-stabilization

STATUS: DRAFT

## 1. Problem Statement
Derived from approved spec retrieval snapshot:
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria

### Context snapshot
- A deep total stabilization and optimization pass for Ultron-AP covering runtime reliability, security hardening, automated test expansion, and documentation hygiene optimization.
- Primary actor: Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.
- Expected outcome: The system remains stable under backup and operational failures with observable error reporting, critical security/session paths are strongly validated, all core tests pass, and project documentation is reduced to a clear, non-duplicated, maintained set with controlled file lengths.

### Actors snapshot
- Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.

### Functional rules snapshot
- The system must enforce a documentation asset strategy where only repository markdown and source files are used; no external media assets are introduced during stabilization and every referenced document path must resolve locally.

### Security snapshot
- State-changing endpoints must enforce CSRF token checks plus same-origin protections (Origin/Referer policy), and session cookies must remain secure under direct TLS and trusted proxy TLS termination.

### Out-of-scope snapshot
- No visual redesign, no new end-user features, no migration away from Go/HTMX/SQLite, and no infrastructure platform change.

Refined problem framing:
- What problem are we solving? Current pain is medium-high: some reliability and security paths had limited observability or test coverage, and documentation can drift with overlapping artifacts. Frequency is recurring during maintenance and release cycles.
- Why now? go test ./... remains green; zero open P0 stabilization items; backup failure outcomes are explicitly logged/persisted; all targeted new reliability/security tests pass; 100% markdown files classified as keep/consolidate/archive/delete with rationale; zero broken internal markdown links.; Baseline: Current baseline: go test ./... is green; stabilization backlog BL0001 remains open with pending P0/P1/P2 actions; documentation classification (keep/consolidate/archive/delete) is not yet complete.

## 2. Discovery Interview Summary (Discovery Persona)
- Primary users:
- Primary user is the single admin/operator of the Raspberry Pi. Secondary users are maintainers/contributors responsible for long-term reliability and documentation quality.

- Jobs to be done:
- Keep Ultron-AP stable under failure conditions, prevent security regressions, maintain high-confidence automated tests, and keep documentation concise, discoverable, and non-duplicated.

- Current pain:
- Current pain is medium-high: some reliability and security paths had limited observability or test coverage, and documentation can drift with overlapping artifacts. Frequency is recurring during maintenance and release cycles.

- Constraints (business/technical/compliance):
- Must keep existing Go monolith architecture and behavior, no new product features, no UI redesign, no infrastructure migration, and keep performance/resource footprint suitable for Raspberry Pi.

- Dependencies:
- Dependencies include SQLite storage, Docker daemon/socket, systemd CLI integration, Pironman5 CLI integration, and Telegram API integration for backup delivery.

- Success metrics:
- go test ./... remains green; zero open P0 stabilization items; backup failure outcomes are explicitly logged/persisted; all targeted new reliability/security tests pass; 100% markdown files classified as keep/consolidate/archive/delete with rationale; zero broken internal markdown links.; Baseline: Current baseline: go test ./... is green; stabilization backlog BL0001 remains open with pending P0/P1/P2 actions; documentation classification (keep/consolidate/archive/delete) is not yet complete.

- Assumptions:
- Assume documentation cleanup can be done without breaking SDLC workflow references; assume backup observability changes will not increase operational noise excessively; assume CSRF origin checks can be implemented without blocking valid proxied requests.; Why now: Recent hardening work surfaced residual quality debt; closing it now reduces regression risk before further feature evolution and keeps maintenance cost low.

- Interview mode:
- deep

## 3. Scope
### In scope
- 1) Backup scheduler failure tracking and surfaced outcomes. 2) Retention regression tests for Telegram-disabled/failure paths. 3) Pironman parse compatibility tests. 4) Hardware apply handler tests for checkbox-off and CSRF. 5) Session cookie policy tests for TLS/X-Forwarded-Proto. 6) Brute-force tracker cleanup strategy. 7) CSRF Origin/Referer defense-in-depth checks. 8) Full markdown inventory and classification. 9) Duplicate/residual/orphan detection and safe cleanup. 10) Cross-link/index repair. 11) File-length policy enforcement and oversized-doc split/condense actions.

### Out of scope
- UI redesign, new dashboard features, architecture rewrite, technology stack migration, and infrastructure topology changes.; No-go zone: Do not introduce scope creep into new product functionality, UI overhauls, or platform migrations during this stabilization phase.

## 4. Actors & User Journeys
Actors:
- Primary user is the single admin/operator of the Raspberry Pi. Secondary users are maintainers/contributors responsible for long-term reliability and documentation quality.

Primary journey:
- Admin runs or receives stabilization updates, validates reliable behavior under failure scenarios, and maintains a clean, trustworthy documentation set for ongoing operation.

## 5. Architecture (Architect Persona)
- Components:
-
- Data flow:
-
- Key decisions:
-
- Risks:
-

## 6. Security (Security Persona)
- Threats:
-
- Controls required:
-
- Validation rules:
-

## 7. Backlog Outline
Epic:
-

User stories:
1.
2.
3.

## 8. Test Strategy
- Smoke tests:
-
- Functional tests:
-
- Security tests:
-
- Edge cases:
-

## 9. Discovery Confidence
- Confidence:
-

- Reason:
-

- Evidence gaps:
-

- Handoff decision:
-
