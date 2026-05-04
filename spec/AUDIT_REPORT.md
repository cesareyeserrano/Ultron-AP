# Project Audit — Ultron-AP

**Executed:** 2026-05-03
**Auditor:** Claude (Opus 4.7) — on-demand `aitri audit`
**Scope:** `/Users/cesareyeserrano/Documents/PROJECTS/Ultron` — `cmd/`, `internal/`, `web/`
**Dimensions covered:** Code Quality · Architecture · Logic · Security · Stack

---

## Findings → Bugs

**[BUG-1]** `[severity: high]` — Privileged helper IPC has no peer-process authentication
- File: `cmd/ultron-helper/main.go:43-62, 87-105`
- Problem: The Unix socket is created with mode 0o660 and an optional group via `ULTRON_HELPER_SOCKET_GROUP` / `ULTRON_HELPER_SOCKET_GID`. That is the *only* gate. `handleConn()` reads JSON and dispatches without verifying the connecting process UID, signing the request, or checking a shared secret. If an operator misconfigures group membership (or any process running as a member of that group is compromised), it gains full root command execution: `systemctl start/stop/restart`, `reboot`, `poweroff`, log reads. This is "trust the filesystem" — fine for the happy path, fragile under misconfiguration.
- Suggested: `aitri bug add --title "Helper IPC lacks peer-credential check" --severity high --description "Add SO_PEERCRED verification in handleConn() so the helper rejects connections from processes whose UID is not the configured ultron-ap user. Defense-in-depth above socket file perms."`

**[BUG-2]** `[severity: medium]` — Login CSRF token consumed before auth check, blocking legitimate retries
- File: `internal/server/handlers_auth.go:49-80`
- Problem: `s.loginTokens.LoadAndDelete(submitted)` removes the token (line 51) before bcrypt comparison (line 76). On a failed login attempt the user gets "Invalid username or password" but the next POST with the same form will fail with 403 Forbidden because the token is gone. The `renderLoginWithError` path does not mint a new token, so the user must reload `/login` to retry — and the failed-login response itself is not a CSRF-protected re-render.
- Suggested: `aitri bug add --title "Login CSRF token consumed before bcrypt check" --severity medium --description "Move LoadAndDelete to after successful auth, OR re-mint a CSRF token in the renderLoginWithError path so users can retry without reloading."`

**[BUG-3]** `[severity: medium]` — SSE per-IP connection limit is bypassable via X-Forwarded-For
- File: `internal/server/sse.go:181-184, 479-493`
- Problem: `clientIPFromRequest()` blindly trusts the first value of the `X-Forwarded-For` header. SSE uses this for per-IP connection capping in `addClientForIP()`. An attacker on the LAN/VPN (the only network surface, but still) can exhaust SSE slots by rotating fake XFF values per request, denying service to legitimate clients. The auth/login `clientIP()` (handlers_auth.go:208-214) correctly uses only `RemoteAddr` — so brute-force lockout is **not** affected, only SSE.
- Suggested: `aitri bug add --title "SSE per-IP cap trusts X-Forwarded-For without proxy allowlist" --severity medium --description "Either ignore XFF and use RemoteAddr (default deployment is direct, not proxied), or only honor XFF when RemoteAddr is in a configured trusted-proxy list."`

**[BUG-4]** `[severity: medium]` — Notification config JSON unmarshal errors silently swallowed
- File: `internal/server/handlers_notifications.go:89-90` (and the corresponding marshal site)
- Problem: `json.Unmarshal([]byte(nc.Config), &cfg)` ignores the error return. If a stored notification config row gets corrupted (manual DB edit, partial write, schema drift), the handler silently treats it as `{}` and may overwrite the legitimate config on next save. There's no log line and no UI signal.
- Suggested: `aitri bug add --title "Notification config unmarshal errors silently dropped" --severity medium --description "Capture err from json.Unmarshal; log it and return 500 so the operator sees corruption rather than overwriting it."`

## Findings → Backlog

**[BL-1]** `[priority: P1]` — Plaintext-secrets fallback warning is not surfaced at startup
- File: `internal/database/secrets.go:33-44`, `cmd/ultron-ap/main.go`
- Problem: When `ULTRON_SECRET_KEY` is unset, secrets (Telegram tokens, SMTP passwords) are stored in plaintext. The warning is gated behind `sync.Once` and fires only when the first secret is encrypted/decrypted — possibly long after process start. An operator who deploys without the env var, then configures notifications a week later, sees the warning buried in `journalctl`. There's no startup banner and no health-endpoint signal.
- Suggested: `aitri backlog add --title "Surface missing ULTRON_SECRET_KEY at startup, not lazily" --priority P1 --problem "Log the plaintext-storage warning unconditionally during cmd/ultron-ap startup, and expose it on /healthz so monitoring dashboards can flag it."`

**[BL-2]** `[priority: P1]` — Backup destination path not validated before VACUUM INTO
- File: `internal/database/sqlite.go:205-215`
- Problem: `Backup(dstPath)` does `os.Remove(dstPath)` and then `VACUUM INTO '<escaped path>'`. Single-quote doubling is correct SQLite escaping (no SQL-injection here — verified), but `dstPath` is operator-supplied via the backup settings. There is zero validation that the path is absolute, normalized, under a designated backup directory, or that the daemon has write permission. A misconfigured or malicious config can overwrite arbitrary files writable by the ultron-ap user (e.g., its own SQLite database, log files).
- Suggested: `aitri backlog add --title "Validate backup LocalPath against an allowed directory" --priority P1 --problem "Reject paths that are not absolute, contain .., or escape a configured backup root. Check writability and surface failures to the settings UI rather than failing silently at backup time."`

**[BL-3]** `[priority: P2]` — Brute-force lockout state is in-memory only, with lazy cleanup
- File: `internal/auth/bruteforce.go:29-62`, `internal/server/handlers_auth.go:23, 166-184`
- Problem: Failed login attempts and IP locks live in process memory. A restart (or crash, or systemd-triggered restart from an alert) wipes lockout state, letting an attacker resume from zero. Cleanup is also lazy — only on the next login attempt — so an idle server's `loginTokens` and `bruteForce` maps grow unbounded until someone tries to log in.
- Suggested: `aitri backlog add --title "Persist brute-force lockout to SQLite + background cleanup" --priority P2 --problem "Move attempts/locks into a small table so restarts don't reset them. Run cleanupExpiredBruteForceAttempts on a 5-minute ticker instead of piggybacking on login requests."`

**[BL-4]** `[priority: P2]` — Audit-log write is not in the same transaction as the action
- File: `internal/database/actions.go:21-27`
- Problem: `LogAction()` is a standalone insert. If a Docker start succeeds but the subsequent log write fails (DB locked, disk full, connection drop), the system performed the action but the audit trail is missing it — exactly the case the audit log exists to prevent.
- Suggested: `aitri backlog add --title "Bind action execution and audit log into one transaction" --priority P2 --problem "Wrap the action dispatch and LogAction insert in a single sql.Tx so either both happen or both don't. If the audit write fails, surface it instead of silently completing the action."`

**[BL-5]** `[priority: P2]` — Privileged helper returns full log output without redaction
- File: `cmd/ultron-helper/main.go:190-232`
- Problem: The helper executes `dmesg`, `ps`, `journalctl` as root and ships the raw stdout to the unprivileged web process, which renders it for any authenticated user. `ps` output can include process command lines containing secrets passed as args; journal entries can include unit descriptions and environment dumps; dmesg leaks kernel/hardware info. Single-user threat model is mild but the surface is broader than necessary.
- Suggested: `aitri backlog add --title "Filter helper log output before returning to web process" --priority P2 --problem "Cap byte size, strip env-var-bearing ps fields, and define a per-source redaction policy so the unprivileged side never sees raw root-readable text it doesn't strictly need."`

**[BL-6]** `[priority: P3]` — CSP runs in Report-Only mode with no enforcement path
- File: `internal/server/middleware.go:63`
- Problem: Browser-side CSP violations are reported but not blocked. The intent appears to have been "soak in report-only first," but there's no `report-uri` collecting violations server-side, so nobody is watching the reports either, and there's no documented criterion for flipping to enforced mode.
- Suggested: `aitri backlog add --title "Promote CSP from Report-Only to enforced + add report endpoint" --priority P3 --problem "Add /api/csp-report, log violations for one release cycle, then switch the header from Content-Security-Policy-Report-Only to Content-Security-Policy."`

## Observations

**[OBS-1]** — Service-name validation depends on a single regex on both sides of the IPC
- Context: `internal/systemd/controls.go:17`, `cmd/ultron-helper/main.go:25`
- Concern: Both ends validate service names with `^[a-zA-Z0-9_.@\-]+$`, which correctly rejects shell metacharacters. Defense-in-depth is good. The risk is that any future weakening of *either* regex (e.g., adding `+` for templated units without re-checking shell-safety) silently re-opens command injection in `systemctl <action> <name>`.
- Why deferred: Currently safe; this is a code-organization invariant, not a defect. A regression test asserting both regexes reject `; ` `` ` `` and `|` would catch drift.

**[OBS-2]** — Docker container ID path params reach the Docker client without shape validation
- Context: `internal/server/handlers_docker.go:28-29`, `internal/docker/controls.go:22-91`
- Concern: `{id}` from the URL is passed straight to the Docker SDK. Malformed IDs return Docker errors, not panics, so this isn't exploitable — but it produces 500s instead of 400s and slightly muddies error logs.
- Why deferred: No security or correctness impact; a 4-line `^[a-f0-9]{12,64}$` check would clean it up but isn't urgent.

**[OBS-3]** — `clientIP()` (auth) and `clientIPFromRequest()` (SSE) implement different trust models
- Context: `internal/server/handlers_auth.go:208-214`, `internal/server/sse.go:479-493`
- Concern: Two functions with near-identical names and divergent XFF handling is a footgun for future contributors who pick the "wrong" one for a new feature.
- Why deferred: Behavior is intentional (auth must not trust XFF; SSE was probably written assuming a proxy). Once BUG-3 is fixed, consolidate to one helper with an explicit `trustForwardedFor bool` parameter.

**[OBS-4]** — Indirect dependency surface is broad; no `govulncheck` in CI
- Context: `go.mod`
- Concern: `docker/docker v27.5.1+incompatible` and the otel chain pull in dozens of indirect deps. Nothing visibly broken at audit time, but there's no automated CVE check.
- Why deferred: Requires running `govulncheck ./...` to know whether anything actually applies to the call graph; that's a CI task, not an audit finding per se.

---

## Coverage notes

- Read top-to-bottom: `cmd/ultron-helper/main.go`, `cmd/ultron-ap/main.go`, `internal/auth/`, `internal/database/sqlite.go` + `secrets.go` + `actions.go`, key handlers in `internal/server/`, `internal/docker/controls.go`, `internal/systemd/controls.go`, `internal/privileged/client.go`.
- Verified-and-corrected during writeup: an initially-flagged "VACUUM INTO SQL injection" was downgraded to BL-2 (path traversal) — single-quote doubling is correct SQLite escaping and modernc/sqlite Exec is single-statement. An initially-flagged "XFF brute-force bypass" was reframed (BUG-3) — login lockout uses RemoteAddr, only SSE trusts XFF.
- Did **not** check: `web/` template XSS surfaces, individual test files, the Tailwind JS bundle, or the SQLite schema migration files.
