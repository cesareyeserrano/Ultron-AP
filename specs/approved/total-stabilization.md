# AF-SPEC: total-stabilization

STATUS: APPROVED
## 1. Context
A deep total stabilization and optimization pass for Ultron-AP covering runtime reliability, security hardening, automated test expansion, and documentation hygiene optimization.

Primary actor: Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.
Expected outcome: The system remains stable under backup and operational failures with observable error reporting, critical security/session paths are strongly validated, all core tests pass, and project documentation is reduced to a clear, non-duplicated, maintained set with controlled file lengths.
In scope: Backup scheduler failure tracking and persistence, regression tests for retention when Telegram is disabled/fails, Pironman parser compatibility tests, hardware apply tests for unchecked toggles and CSRF, session cookie tests for TLS and X-Forwarded-Proto, brute-force tracker memory cleanup strategy, CSRF Origin/Referer defense-in-depth, documentation inventory of all markdown files, duplicate/residual detection, deletion or archival of unused docs with impact review, cross-link/index cleanup, and file-length optimization policy with measurable limits and splitting strategy for oversized docs.
Out of scope: No visual redesign, no new end-user features, no migration away from Go/HTMX/SQLite, and no infrastructure platform change.
Technology: Go monolith, HTMX, Tailwind CSS, SQLite, SSE, Docker SDK, systemd CLI integration.
Requirement source: Provided explicitly by user in guided draft.

## 2. Actors
- Admin (single Raspberry Pi owner/operator) and maintainers who evolve the codebase.

## 3. Functional Rules (traceable)
- FR-1: The system must enforce a documentation asset strategy where only repository markdown and source files are used; no external media assets are introduced during stabilization and every referenced document path must resolve locally.

## 4. Edge Cases
- A markdown document appears redundant by title or section overlap but is still referenced by automation/spec workflows; cleanup must detect these references before deletion and preserve traceability.

## 5. Failure Conditions
- If automated backup fails (for example Telegram upload failure), the run is marked as failed with a clear error cause and does not silently pass.
- If documentation cleanup proposes deleting a file that still has inbound references, deletion is blocked until references are removed or the file is reclassified.
- If stabilization changes break existing core behavior (monitoring, alerts, hardware controls, authentication), approval is blocked until regression tests pass.

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

## 10. Requirement Source Statement
- All requirements in this draft were provided explicitly by the user.
- Aitri structured the content and did not invent requirements.

## 11. Resource Strategy
- Use only repository documentation as source of truth (README, DEPLOY, sdlc-studio, backlog, specs) and generated local inventories; no external assets required.
