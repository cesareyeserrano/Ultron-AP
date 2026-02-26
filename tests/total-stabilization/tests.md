# Test Cases: total-stabilization

> Trace rule: every test maps to US-*, FR-* and AC-* where applicable.

## Functional

### TC-1 Backup failure observability is explicit
- Trace: US-1, FR-2, AC-2
- Steps:
  1. Given local backup creation succeeds and Telegram upload fails.
  2. When automated scheduler run executes.
  3. Then failure outcome contains explicit cause and is auditable.

### TC-2 Retention still executes on Telegram failure
- Trace: US-2, FR-3, AC-2
- Steps:
  1. Given backup directory already exceeds retention limit.
  2. And Telegram upload fails or is disabled.
  3. When backup run finishes.
  4. Then retained local backups equal configured maximum.

### TC-3 Unsafe endpoint rejects invalid CSRF token
- Trace: US-3, FR-4, AC-3
- Steps:
  1. Given authenticated session.
  2. When state-changing endpoint receives invalid or missing CSRF token.
  3. Then request is denied deterministically.

### TC-4 Unsafe endpoint rejects invalid origin context
- Trace: US-3, FR-4, AC-3
- Steps:
  1. Given authenticated session with unsafe method.
  2. When origin/referer policy fails.
  3. Then request is denied and rejection is auditable.

### TC-5 Session cookie flags under direct TLS
- Trace: US-4, FR-4, AC-3
- Steps:
  1. Given login over direct TLS.
  2. When session cookie is set.
  3. Then secure cookie semantics are present.

### TC-6 Session cookie flags under proxied TLS
- Trace: US-4, FR-4, AC-3
- Steps:
  1. Given login through trusted proxy with `X-Forwarded-Proto=https`.
  2. When session cookie is set.
  3. Then secure cookie semantics are present.

### TC-7 External hardware boundary compatibility
- Trace: US-5, FR-5, AC-4
- Steps:
  1. Given Ultron running with helper and external Pironman service.
  2. When integration diagnostics and routes are validated.
  3. Then no Pironman control endpoint/action exists in Ultron core.

### TC-8 Hardware apply checkbox-off semantics
- Trace: US-5, FR-5, AC-4
- Steps:
  1. Given hardware apply request where checkbox fields are absent.
  2. When handler parses payload.
  3. Then toggles are interpreted as `false` and CSRF is still enforced.

### TC-9 Brute-force tracker cleanup keeps behavior stable
- Trace: US-6, FR-5, AC-4
- Steps:
  1. Given stale and active brute-force entries.
  2. When cleanup executes.
  3. Then stale entries are removed while lockout behavior remains correct.

### TC-10 Full regression suite remains green
- Trace: US-5, FR-5, AC-4
- Steps:
  1. Given all stabilization slices merged.
  2. When `go test ./...` runs.
  3. Then all tests pass.

## Documentation Hygiene

### TC-11 Markdown inventory classification completeness
- Trace: US-7, FR-1, AC-1
- Steps:
  1. Given repository markdown inventory.
  2. When classification runs.
  3. Then every markdown file has keep/consolidate/archive/delete + rationale + owner.

### TC-12 Reference-safe deletion gate
- Trace: US-7, FR-1, AC-1
- Steps:
  1. Given a file marked for delete but still referenced.
  2. When cleanup attempts deletion.
  3. Then deletion is blocked until references are resolved.

### TC-13 File-length optimization policy
- Trace: US-8, FR-1, AC-1
- Steps:
  1. Given markdown files above policy threshold.
  2. When optimization pass runs.
  3. Then files are split/condensed and navigation remains valid.

### TC-14 Internal markdown links integrity
- Trace: US-8, FR-1, AC-1
- Steps:
  1. Given updated documentation set.
  2. When internal link validation runs.
  3. Then no broken internal markdown links remain.

## Negative / Abuse

### TC-15 Spoofed origin with valid CSRF token
- Trace: US-3, FR-4, AC-3
- Steps:
  1. Given unsafe request includes valid CSRF token but forged origin.
  2. When endpoint validates request.
  3. Then request is denied.

### TC-16 Backup runner repeated failures do not become silent
- Trace: US-1, FR-2, AC-2
- Steps:
  1. Given repeated Telegram delivery failures across runs.
  2. When scheduler continues execution.
  3. Then each run reports deterministic failure outcome without silent suppression.
