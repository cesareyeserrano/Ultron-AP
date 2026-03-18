# Security Review: Ultron Monitoring Stabilization

## 1. Threat Profile
- Scenario A: Session hijack/CSRF on state-changing endpoints.
  - Goal: execute unauthorized actions as authenticated operator.
  - Likelihood: Medium.
  - Impact: High.
- Scenario B: SSE/HTMX endpoint abuse (high-frequency calls, connection flooding).
  - Goal: degrade service availability on low-resource Raspberry Pi.
  - Likelihood: Medium.
  - Impact: Medium-High.
- Scenario C: Privileged-boundary bypass via malformed local requests.
  - Goal: escalate capability from web layer to privileged operations.
  - Likelihood: Low-Medium.
  - Impact: Critical.

## 2. Security Requirements (Must-Haves)
- AuthN/AuthZ:
  - Enforce authenticated session for all dashboard and operational routes.
  - Reject unauthorized access by default.
- Data handling:
  - Session cookies: HttpOnly + Secure (proxy-aware) + SameSite.
  - Do not expose sensitive configuration values in templates/stream payloads.
- Request integrity:
  - CSRF token checks for all state-changing endpoints.
  - Same-origin checks (Origin/Referer) for defense in depth.
- Boundary validation:
  - Strict allowlist validation for any helper-boundary input.
  - No direct privileged execution from web handlers.

## 3. Operational Guardrails
- Rate limiting:
  - Per-session/IP limits for auth and high-cost endpoints.
  - SSE connection cap and idle timeout per client.
- Audit logging:
  - Structured logs for auth failures, CSRF rejections, boundary-validation failures, and repeated stream reconnects.
- Input validation:
  - Whitelist-first validation on all query/form/path inputs.
  - Canonicalization before policy checks.
- Secret management:
  - Secrets loaded from environment or local protected files, never in templates/client payloads.

## 4. Dependency Check
- Go + SQLite + HTMX/SSE stack: acceptable for local single-node profile when patched.
- High-risk areas to monitor:
  - External monitoring adapters and helper boundary glue code.
  - Any unpinned or outdated dependencies in Go module tree.
- Mitigation:
  - Enforce periodic dependency audit and pinned versions for critical libs.
  - Block introducing unvetted external services without threat review.

## 5. Risk Decision and Trade-off Summary
- Risk: CSRF/session misuse on state-changing paths.
  - Decision: Mitigate.
  - Owner: Backend maintainer.
  - Reason: high impact and directly exploitable if missed.
  - Review date: 2026-04-15.
- Risk: SSE abuse causing resource exhaustion.
  - Decision: Mitigate.
  - Owner: Platform maintainer.
  - Reason: availability risk on Raspberry Pi constraints.
  - Review date: 2026-04-15.
- Risk: Privileged-boundary bypass.
  - Decision: Block until validated controls are present.
  - Owner: Security + backend maintainer.
  - Reason: potential critical compromise path.
  - Review date: before release gate.
- Risk: Minor dependency drift without known exploit path.
  - Decision: Accept (time-boxed).
  - Owner: Maintainer.
  - Reason: low immediate impact; handled by scheduled patch cycle.
  - Review date: 2026-03-31.

## Stage Gates
- Ready for Dev:
  - Must-have controls above are defined and testable.
- Ready for Prod:
  - CSRF/origin checks validated, boundary validation tests passing, no unresolved high/critical security findings.

## Threats
- Session/CSRF misuse on state-changing endpoints.
- SSE/HTMX abuse causing availability degradation on low-resource hardware.
- Privileged-boundary bypass attempts via malformed inputs.

## Required controls
- Authenticated access for protected routes.
- CSRF token validation plus Origin/Referer checks for state-changing routes.
- Secure session cookie settings (HttpOnly, Secure, SameSite with proxy awareness).
- Whitelist-first input validation and rate limiting for auth/high-cost endpoints.
- Audit logging for rejects and security-relevant failures.

### Threats
- Forged shutdown/restart request: attacker goal is unauthorized state change → service disruption.
- CSRF/origin abuse on settings endpoints: attacker goal is executing dangerous action from victim browser → unauthorized host control.
- Misclick-driven dangerous action on mobile: attacker/accidental user goal is unintended restart/shutdown → avoidable downtime.

### Required controls
- Session validation: require authenticated valid session for all state-changing settings and dangerous actions.
- Request integrity: enforce CSRF token + same-origin checks on every state-changing request.
- Intent verification: require typed confirmation word and countdown cancel window before dangerous action execution.
- Auditability: log all rejects and dangerous-action outcomes with reason, target, and actor context.
