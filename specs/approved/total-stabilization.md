# AF-SPEC: total-stabilization

STATUS: APPROVED
## 1. Context
A deep total stabilization and optimization pass for Ultron-AP covering runtime reliability, security hardening, automated test expansion, and documentation hygiene optimization.

Primary actor: Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.
Expected outcome: The system remains stable under backup and operational failures with observable error reporting, critical security/session paths are strongly validated, all core tests pass, and project documentation is reduced to a clear, non-duplicated, maintained set with controlled file lengths.
In scope: Backup scheduler failure tracking and persistence, regression tests for retention when Telegram is disabled/fails, external hardware boundary checks (no in-app Pironman control), session cookie tests for TLS and X-Forwarded-Proto, brute-force tracker memory cleanup strategy, CSRF Origin/Referer defense-in-depth, documentation inventory of all markdown files, duplicate/residual detection, deletion or archival of unused docs with impact review, cross-link/index cleanup, and file-length optimization policy with measurable limits and splitting strategy for oversized docs.
Out of scope: No visual redesign, no new end-user features, no migration away from Go/HTMX/SQLite, and no infrastructure platform change.
Technology: Go monolith, HTMX, Tailwind CSS, SQLite, SSE, Docker SDK, systemd CLI integration.
Requirement source: Provided explicitly by user in guided draft.

## 2. Actors
- Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.

## 3. Functional Rules (traceable)
- FR-1: The system must enforce a documentation asset strategy where only repository markdown and source files are used; no external media assets are introduced during stabilization and every referenced document path must resolve locally.
- FR-2: The system must surface automated backup failures with explicit, actionable outcomes (logs and/or persisted evidence) and never silently ignore failed backup runs.
- FR-3: The system must preserve deterministic local backup retention even when Telegram delivery is disabled or fails.
- FR-4: The system must strengthen security posture by enforcing CSRF token checks plus same-origin protections for state-changing endpoints, with proxy-aware session-cookie security behavior.
- FR-5: The system must add targeted automated tests for high-risk reliability/security paths (backup failures, retention branches, external hardware boundary, session cookie flags, and CSRF protections) without regressing existing behavior.

## 4. Edge Cases
- A markdown document appears redundant by title or section overlap but is still referenced by automation/spec workflows; cleanup must detect these references before deletion and preserve traceability.

## 5. Failure Conditions
- If automated backup fails (for example Telegram upload failure), the run is marked as failed with a clear error cause and does not silently pass.
- If documentation cleanup proposes deleting a file that still has inbound references, deletion is blocked until references are removed or the file is reclassified.
- If stabilization changes break existing core behavior (monitoring, alerts, authentication, and external-hardware boundary), approval is blocked until regression tests pass.

## 6. Non-Functional Requirements
- Determinism: Documentation hygiene outputs must be reproducible from the same repository state.
- Traceability: Every `consolidate/archive/delete` decision must include a rationale and owner.
- Maintainability: Markdown files should follow a controlled length policy; oversized files must be split or condensed while preserving navigability.
- Performance safety: Stabilization work must not introduce materially worse runtime characteristics for normal operation.

## 7. Security Considerations
- State-changing endpoints must enforce CSRF token checks plus same-origin protections (Origin/Referer policy), and session cookies must remain secure under direct TLS and trusted proxy TLS termination.

## 8. Out of Scope
- No visual redesign, no new end-user features, no migration away from Go/HTMX/SQLite, and no infrastructure platform change.

## 9. Acceptance Criteria
- AC-1: Given the markdown inventory and reference scan, when documentation hygiene runs, then every markdown file is classified as keep/consolidate/archive/delete with justification, no broken internal links remain, and files above the defined max length are split or reduced.
- AC-2: Given an automated backup run where local backup succeeds and Telegram upload fails, when the scheduler executes, then the run is marked failed with clear cause and retention still enforces the configured local backup limit.
- AC-3: Given a state-changing request without valid CSRF and/or invalid origin context, when the endpoint is called, then the request is denied deterministically and audited as a rejected action.
- AC-4: Given stabilization changes merged, when the full and targeted test suites execute, then all legacy tests remain green and all new stabilization tests pass.

## 10. Requirement Source Statement
- All requirements in this draft were provided explicitly by the user.
- Aitri structured the content and did not invent requirements.

## 11. Resource Strategy
- Use only repository documentation as source of truth (README, DEPLOY, sdlc-studio, backlog, specs) and generated local inventories; no external assets required.
