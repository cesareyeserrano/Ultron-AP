# AF-SPEC: total-stabilization

STATUS: DRAFT

## 1. Context
A total stabilization pass for Ultron-AP focused on reliability, security hardening, and test coverage for critical operational paths.

Primary actor: Admin (single Raspberry Pi owner/operator).
Expected outcome: Backup scheduler reports failures with actionable traces, retention behavior is validated in failure paths, hardware and session security edge-cases are covered by tests, and the platform remains stable with all tests passing.
In scope: Backup failure tracking in scheduler loop, regression tests for backup retention when Telegram is disabled/fails, Pironman parser compatibility tests, hardware apply handler tests for unchecked toggles and CSRF, session cookie policy tests for HTTPS/proxy, brute-force tracker memory cleanup strategy, and CSRF origin/referer defense-in-depth checks.
Out of scope: No UI redesign, no new product features, no architectural rewrite, and no changes to deployment topology.
Technology: Go monolith, HTMX, Tailwind CSS, SQLite, SSE, Docker SDK, systemd CLI integration.
Requirement source: Provided explicitly by user in guided draft.

## 2. Actors
- Admin (single Raspberry Pi owner/operator).

## 3. Functional Rules (traceable)
- FR-1: The system must preserve operational reliability under failure conditions by handling backup failures explicitly, enforcing retention behavior, and validating security-sensitive request/session behavior through automated tests.
- FR-2: The stabilization work must not regress existing monitoring, alerts, hardware controls, or authentication flows.

## 4. Edge Cases
- Backup generation succeeds but Telegram upload fails, and the system still needs deterministic retention and observable failure reporting without crashing or silently hiding errors.

## 5. Failure Conditions
- TBD (refine during review)

## 6. Non-Functional Requirements
- TBD (refine during review)

## 7. Security Considerations
- Enforce CSRF token plus same-origin validation for state-changing endpoints, and preserve secure session-cookie behavior across direct HTTPS and reverse-proxy HTTPS scenarios.

## 8. Out of Scope
- No UI redesign, no new product features, no architectural rewrite, and no changes to deployment topology.

## 9. Acceptance Criteria
- AC-1: Given a completed local backup and a failed Telegram send, when automated backup runs, then retention still keeps only the configured latest local backups and a failure outcome is recorded.

## 10. Requirement Source Statement
- All requirements in this draft were provided explicitly by the user.
- Aitri structured the content and did not invent requirements.

## 11. Resource Strategy
- No external assets required; use existing project documentation and current codebase only.
