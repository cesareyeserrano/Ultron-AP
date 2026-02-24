# Root Remediation Implementation Plan

Date: 2026-02-24  
Scope: Execute structural fixes (no patch-only approach) from Pi5 production audit and Aitri feedback (`FB-1`..`FB-4`).

## Goals
- Eliminate recurrent hardware template contract failures by unifying render/view-model contracts.
- Reduce Raspberry Pi 5 runtime background overhead, especially SSE-related waste.
- Raise cybersecurity baseline for browser, host privileges, and secret handling.
- Make backup subsystem fully parameterizable from Settings (requirement gap closure).

## Workstreams

### WS-1 View-Model Contract Unification
- Introduce typed view models with explicit shared fields for authenticated pages.
- Ensure `CSRFToken` propagation is enforced through a single render pipeline.
- Prohibit partial rendering with ad-hoc structs for authenticated HTML responses.
- Add integration tests to render high-risk pages (`/hardware`, `/settings`) and fail on template execution errors.

### WS-2 SSE + Runtime Efficiency (Pi5)
- Scope `sse-connect` to dashboard-only context.
- Add adaptive scheduler behavior:
  - idle mode when no active SSE clients.
  - active mode with current intervals when dashboard clients exist.
- Separate SSE streaming timeout strategy from regular HTTP responses.

### WS-3 Web Security Baseline
- Add centralized security middleware:
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY` (or `frame-ancestors` in CSP)
  - `Referrer-Policy: no-referrer`
  - `Permissions-Policy` restrictive baseline
  - phased CSP rollout (report-only then enforce)
- Keep compatibility with HTMX + SSE.

### WS-4 Host Hardening
- Harden systemd unit profile (`NoNewPrivileges=true`, stricter sandboxing).
- Tighten sudoers scope from wildcard actions to explicit allowlist.
- Tighten filesystem modes for DB/backups to least privilege.

### WS-5 Backup Subsystem Refactor (Settings-driven)
- Create first-class `BackupConfig` model (not generic channel blob).
- Settings-configurable fields:
  - enabled
  - interval
  - retention count
  - local path
  - destination mode
  - encrypt on/off
  - encryption key reference
  - upload timeout/size
- Runtime applies backup config dynamically without restart.
- Pipeline stages: snapshot -> retention -> encryption -> optional upload -> audit.

### WS-6 Supply Chain & Security Gate
- Run `govulncheck ./...` in connected CI security stage.
- Produce artifacted vulnerability report and remediation SLA.

## Suggested Execution Order
1. WS-1 (fix production bug root cause first)
2. WS-3 (security headers baseline)
3. WS-2 (resource optimization)
4. WS-5 (backup configurability + crypto)
5. WS-4 (host hardening rollout with deploy validation)
6. WS-6 (final security gate)

## Exit Criteria
- No template runtime errors in authenticated routes during integration tests.
- Dashboard SSE not active on non-dashboard pages.
- Security headers visible on HTML and API responses by policy.
- Backup fully configurable from settings and behavior proven in tests.
- Host privilege scope reduced and documented.
- CVE scan executed in CI and accepted.
