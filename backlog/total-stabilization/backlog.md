# Backlog: total-stabilization

> Generated in Aitri flow and manually refined for execution quality.
> Trace policy: each story maps to FR-* and AC-* from approved spec.

## Epics
- EP-1: Backup Reliability and Observability
  - Trace: FR-2, FR-3, AC-2
- EP-2: Security Hardening for State-Changing Paths
  - Trace: FR-4, AC-3
- EP-3: Reliability/Security Test Expansion
  - Trace: FR-5, AC-4
- EP-4: Documentation Hygiene and Optimization
  - Trace: FR-1, AC-1

## User Stories

### US-1 Backup failure observability
- As an admin, I want automated backup failures to be surfaced clearly, so that failures are actionable and never silent.
- Trace: FR-2, AC-2
- Acceptance Criteria:
  - Given a backup run where Telegram upload fails, when the scheduler executes, then a failure outcome with explicit cause is emitted and auditable.
  - Given a successful backup run, when scheduler executes, then success outcome is emitted with no false failure signal.

### US-2 Deterministic retention under delivery failure
- As an admin, I want local retention to execute even when Telegram delivery fails, so that disk usage remains controlled.
- Trace: FR-3, AC-2
- Acceptance Criteria:
  - Given local backup success and Telegram failure, when run completes, then retention keeps only configured local backups.
  - Given Telegram disabled, when run completes, then retention policy is still applied deterministically.

### US-3 CSRF + same-origin enforcement
- As an admin, I want unsafe endpoints to reject invalid CSRF/origin requests, so that cross-site attacks are blocked.
- Trace: FR-4, AC-3
- Acceptance Criteria:
  - Given unsafe request with invalid/missing CSRF token, when endpoint is called, then request is denied.
  - Given unsafe request with invalid origin policy, when endpoint is called, then request is denied and audit evidence exists.

### US-4 Proxy-aware secure session cookie behavior
- As an admin, I want session cookie security behavior to remain correct under TLS and proxied TLS, so that session transport remains hardened.
- Trace: FR-4, AC-3
- Acceptance Criteria:
  - Given direct TLS request, when login succeeds, then cookie includes secure semantics.
  - Given trusted proxy TLS termination (`X-Forwarded-Proto=https`), when login succeeds, then cookie security behavior remains correct.

### US-5 High-risk reliability/security tests
- As a maintainer, I want targeted tests for known fragile paths, so that regressions are caught before release.
- Trace: FR-5, AC-4
- Acceptance Criteria:
  - Given stabilization test suite, when executed, then backup failure, retention branch, cookie policy, CSRF/origin protections, and external hardware boundary checks are covered.
  - Given full repository tests, when executed, then legacy tests remain green.

### US-6 Brute-force tracker cleanup strategy
- As a maintainer, I want stale brute-force state to be cleaned, so that memory growth is bounded over long runtimes.
- Trace: FR-5, AC-4
- Acceptance Criteria:
  - Given stale tracker entries, when cleanup cycle runs, then expired entries are removed.
  - Given normal auth traffic, when cleanup is enabled, then lockout behavior remains correct.

### US-7 Documentation inventory and classification
- As a maintainer, I want every markdown file classified with rationale and owner, so that document lifecycle is explicit.
- Trace: FR-1, AC-1
- Acceptance Criteria:
  - Given markdown inventory, when classification completes, then each file is marked keep/consolidate/archive/delete with rationale.
  - Given candidate deletions, when inbound references exist, then deletion is blocked until resolved.

### US-8 Documentation optimization and link integrity
- As a maintainer, I want oversized documents reduced/split and links repaired, so that docs are maintainable and navigable.
- Trace: FR-1, AC-1
- Acceptance Criteria:
  - Given files above policy length, when optimization runs, then files are split/condensed with navigation preserved.
  - Given updated docs, when link validation runs, then no broken internal markdown links remain.

## Execution Status (2026-02-24)
- [x] US-1 completed
- [x] US-2 completed
- [x] US-3 completed
- [x] US-4 completed
- [x] US-5 completed
- [x] US-6 completed
- [x] US-7 completed
- [x] US-8 completed

## Post-Stabilization Root Remediation (Security Architecture)
- BL-ROOT-1: Privileged operations helper-service split (completed 2026-02-24)
  - Priority: P0
  - Problem: with `NoNewPrivileges=true`, web-process `sudo` paths (hardware/system operations) are intentionally blocked.
  - Root fix: move privileged actions to a dedicated root-owned local helper (systemd service + Unix socket IPC + strict allowlist), and make web panel an unprivileged client only.
  - Acceptance:
    - `ultron-ap.service` keeps `NoNewPrivileges=true`.
    - No direct `sudo` execution from web-process code paths.
    - Helper enforces command allowlist + parameter validation + auditable action log.
    - Hardware/system actions remain functional through helper IPC.
  - Trace: FR-4, FR-5
