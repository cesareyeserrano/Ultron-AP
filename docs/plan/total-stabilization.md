# Plan: total-stabilization

STATUS: DRAFT

## 1. Intent (from approved spec)
- Deliver a deep stabilization and optimization pass for Ultron-AP without scope creep.
- Cover runtime reliability, security hardening, automated test expansion, and documentation hygiene.
- Keep existing architecture (Go monolith + HTMX + SQLite) and avoid feature redesign.

### Actors
- Primary: Admin (single Raspberry Pi owner/operator).
- Secondary: Maintainers/contributors.

### Functional Rules in Scope
- FR-1: Documentation asset strategy (local repository sources only; all references must resolve).
- FR-2: Backup failures must be observable and not silently ignored.
- FR-3: Local retention must remain deterministic even when Telegram delivery fails/disabled.
- FR-4: Enforce CSRF + same-origin protections with proxy-aware session cookie security.
- FR-5: Add targeted tests for high-risk reliability/security paths with zero regression.

### Acceptance Criteria in Scope
- AC-1: Full markdown classification + link integrity + file-length governance.
- AC-2: Backup failure remains observable while retention still enforced.
- AC-3: Invalid CSRF/origin requests are denied and audited.
- AC-4: Full legacy suite green + all new stabilization tests green.

## 2. Discovery Review
### Problem framing
- Reliability/security hardening exists but has residual gaps in observability and focused regression coverage.
- Documentation has potential overlap/residual risk and no formal keep/consolidate/archive/delete matrix.

### Constraints
- No architecture rewrite.
- No UX redesign or new product capabilities.
- Maintain Raspberry Pi-appropriate runtime profile.

### Dependencies
- SQLite persistence.
- Docker daemon/socket.
- systemd integration.
- Pironman5 CLI.
- Telegram integration for backup delivery.

### Success metrics
- `go test ./...` green.
- Zero open P0 stabilization items.
- Backup failure outcomes explicitly reported.
- 100% markdown files classified with rationale/owner.
- Zero broken internal markdown links.

## 3. Scope
### In scope
- Backup scheduler failure observability and outcome propagation.
- Backup retention behavior verification under Telegram failure/disabled paths.
- CSRF origin/referer hardening for state-changing endpoints.
- Session cookie policy validation under TLS and proxied TLS.
- Brute-force tracker cleanup strategy.
- Pironman compatibility parser tests.
- Hardware apply endpoint tests (checkbox-off semantics and CSRF).
- Documentation inventory, classification, dedupe/consolidation decisions, link cleanup, and file-length optimization.

### Out of scope
- New user-facing features.
- UI visual redesign.
- Database/infra platform migration.

## 4. Product Review (Product Persona)
### Business value
- Reduces operational risk and maintenance burden.
- Improves auditability and incident diagnosis.
- Increases confidence for future feature delivery.

### Success metric
- Primary KPI bundle:
  - `go test ./...` passes.
  - Zero open P0 stabilization items.
  - Backup failure outcomes are explicitly surfaced.
  - 100% markdown files classified with rationale/owner.
  - Zero broken internal markdown links.
- Release target:
  - AC-1 through AC-4 fully satisfied.

### Assumptions to validate
- Proxy-aware origin policy can be enforced without blocking valid admin traffic behind trusted reverse proxy.
- Backup failure reporting can be made explicit without creating noisy duplicate logs.
- Documentation cleanup can be completed without removing files still required by SDLC workflow references.

### Release quality bar
- Do not ship unless AC-1..AC-4 are satisfied.

## 5. Architecture (Architect Persona)
### Components
- `internal/server/server.go` (backup scheduler behavior).
- `internal/server/handlers_settings.go` and security middleware/validators.
- `internal/auth/*` and session/cookie handling.
- `internal/metrics/*`, `internal/notify/*`, `internal/pironman/*` tests and reliability guards.
- Documentation tree: `README.md`, `DEPLOY.md`, `sdlc-studio/**`, `specs/**`, `backlog/**`, `docs/**`.

### Data flow
1. Scheduler triggers automated backup run.
2. Backup result produces explicit success/failure outcome with actionable cause.
3. Retention enforcement executes regardless of Telegram delivery outcome.
4. Security middleware/handlers validate CSRF + origin context before unsafe actions.
5. Test suite and documentation hygiene tooling validate gates prior to release.

### Key decisions
- Favor explicit failure surfaces over silent fallback.
- Keep test coverage close to failure-prone boundaries.
- Documentation cleanup must be reference-safe: never delete referenced artifacts without migration.

### Risks & mitigations
- Risk: accidental doc deletion of referenced files.
  - Mitigation: reference scan gate before delete/archive.
- Risk: stricter CSRF origin checks break valid proxied traffic.
  - Mitigation: proxy-aware allowlist logic + tests.
- Risk: over-logging from backup failures.
  - Mitigation: structured, bounded severity and deduplicated messages.

### Observability (logs/metrics/tracing)
- Log backup scheduler outcomes with structured status and error cause.
- Log rejection reasons for denied unsafe requests (without leaking sensitive token values).
- Track stabilization gate status via test execution results and documentation hygiene reports.

## 6. Security Review
### Threats addressed
- CSRF bypass on state-changing endpoints.
- Origin spoofing/incorrect same-site assumptions behind reverse proxy.
- Session cookie misconfiguration under TLS termination.

### Required controls
- Enforce CSRF token and origin policy for unsafe methods.
- Maintain secure cookie semantics under TLS/proxy.
- Ensure denied requests are auditable.

## 7. UX/UI Review (UX/UI Persona, if user-facing)
- No redesign required.
- Maintain existing operational UX behavior for admin controls.
- Any security/reliability messaging must be concise and actionable.

## 8. Backlog decomposition
- See [backlog/total-stabilization/backlog.md](../../backlog/total-stabilization/backlog.md).

## 9. Test decomposition
- See [tests/total-stabilization/tests.md](../../tests/total-stabilization/tests.md).

## 10. Implementation Notes
### Suggested sequence
1. Backup observability and retention regression paths (FR-2, FR-3, AC-2).
2. Security hardening + cookie/origin validation (FR-4, AC-3).
3. Focused reliability/security test expansion (FR-5, AC-4).
4. Documentation hygiene and optimization pass (FR-1, AC-1).

### Rollout / fallback
- Roll changes in small slices, keep full suite green between slices.
- For doc cleanup, prefer `archive` before permanent delete when uncertainty exists.
