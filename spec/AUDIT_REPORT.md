# Project Audit — Ultron-AP (on-demand)

**Executed:** 2026-04-22
**Auditor:** Claude (Anthropic Opus 4.7)
**Scope:** `/Users/cesareyeserrano/Documents/PROJECTS/ultron`
**Dimensions covered:** Code Quality · Architecture · Logic · Security · Stack

---

## Findings → Bugs

**[BUG-1]** `[severity: high]` — Secrets silently stored in plaintext when `ULTRON_SECRET_KEY` is not configured
- File: `internal/database/secrets.go:26-30`
- Problem: `encryptSecret()` checks for the env-provided master key and, if missing, returns the plaintext unchanged with no warning or log. Any notification credential (Telegram bot token, SMTP password) persisted through the settings UI is therefore written to `ultron.db` in plaintext when the operator forgets to set `ULTRON_SECRET_KEY`. FR-011 (privilege separation) and FR-015 (encrypted backup) both implicitly assume credentials at rest are protected; silent fail-open breaks that expectation. A user who later adds the env var will still have the old plaintext rows in the DB (no migration).
- Suggested: `aitri bug add --title "encryptSecret silently falls back to plaintext when ULTRON_SECRET_KEY missing" --severity high --fr FR-011 --description "internal/database/secrets.go:26 returns plaintext with no warning if ULTRON_SECRET_KEY env is unset, causing Telegram bot token / SMTP password to be stored in SQLite in the clear. Must either (a) require the key and fail startup, or (b) log a loud warning and refuse to persist secret-flagged columns."`

**[BUG-2]** `[severity: low]` — CSRF token compared non-constant-time (inconsistency with `auth.ValidateToken`)
- File: `internal/server/handlers_csrf.go:28`
- Problem: The request's `csrf_token` is compared to `session.CSRFToken` with `!=`. Elsewhere (`internal/auth/csrf.go:21`) the code uses `subtle.ConstantTimeCompare`. In practice the 64-char hex token is too long for timing attacks to be cheap, but the inconsistency is a regression vector — a future change to CSRF token format (shorter, structured) would expose the weakness without anyone noticing.
- Suggested: `aitri bug add --title "CSRF token compared with != instead of subtle.ConstantTimeCompare" --severity low --fr FR-012 --description "handlers_csrf.go:28 uses direct string compare; unify with auth.ValidateToken to stay timing-safe by default."`

**[BUG-3]** `[severity: low]` — Alert engine creates a timeout context that is never used
- File: `internal/alerts/engine.go:114-116`
- Problem: `evaluate()` creates a 10s timeout context with `_, cancel := context.WithTimeout(...)` and immediately discards the returned context. The inline comment acknowledges it: "Adding the timeout here for future safety." The dead timeout gives a false sense of protection — DB calls inside `evaluate()` inherit no deadline, so a stuck query on a slow I/O path can block the entire alert evaluation loop indefinitely (tick + evaluate run serially on one goroutine).
- Suggested: `aitri bug add --title "alerts.engine evaluate() discards its timeout context" --severity low --fr FR-004 --description "engine.go:115 creates a WithTimeout context never passed to DB methods. Either propagate the ctx into ListEnabledAlertConfigs/CreateAlert/ListAlerts, or remove the dead code. A stuck DB query today halts the alert loop with no timeout protection."`

---

## Findings → Backlog

**[BL-1]** `[priority: P2]` — Docker/Systemd alert cooldowns are hardcoded to 15 minutes, contradicting FR-004
- File: `internal/alerts/engine.go:206, 248`
- Problem: FR-004 acceptance criterion: "Configurable cooldown per alert rule to prevent spam (default: 15 min)." Metric-based rules honor `cfg.CooldownMinutes` (line 167). Docker container state transitions and Systemd service-failed transitions use a literal `15 * time.Minute`, ignoring user configuration. The spec says cooldown is per-rule; code only implements it for metric rules.
- Suggested: `aitri backlog add --title "Docker/Systemd alert cooldowns hardcoded (FR-004 drift)" --priority P2 --problem "engine.go:206,248 use a literal 15-minute cooldown for Docker/Systemd transition alerts while FR-004 promises per-rule configurable cooldown. Either add cooldown config to Docker/Systemd rule types or document the divergence in FR-004."`

**[BL-2]** `[priority: P2]` — Backup encryption reads the whole database file into RAM
- File: `internal/server/backup_crypto.go:32`
- Problem: `encryptFileAESGCM()` calls `os.ReadFile(srcPath)` before encrypting. On a Raspberry Pi with ~512MB–2GB RAM and a growing SQLite DB (alerts + action history + notifications can accumulate), a 100MB backup file briefly consumes >100MB of Go heap for encryption. With the default `max_upload_size_mb=50` the fleet is mostly safe, but the encryption pass itself happens BEFORE the size check (server.go:362) — so a user who enables encryption and accumulates a large DB can OOM-kill the process. Stream encryption (chunked Seal + writer) avoids this.
- Suggested: `aitri backlog add --title "Stream-encrypt backup files instead of loading whole DB in RAM" --priority P2 --problem "backup_crypto.go:32 reads entire backup file into memory before AES-GCM. On Pi with low RAM + growing DB, large backups risk OOM. Also encryption runs before max-upload-size check. Replace with chunked AEAD (e.g. age, NaCl secretstream, or manual counter+Seal per chunk)."`

**[BL-3]** `[priority: P3]` — Alert cooldown map has no pruning; stale container/service names accumulate forever
- File: `internal/alerts/engine.go:28,171,210,252`
- Problem: `e.cooldowns` is keyed by `metric:<id>`, `docker:<container>`, `systemd:<service>`. Metric keys are bounded by AlertConfig rows, but Docker/Systemd keys are bounded by the set of names ever seen. Long-lived deployments with churning container names or one-off units leak a small entry per name indefinitely. Memory impact is tiny (bytes per entry) but this is a textbook unbounded-map bug that future monitoring systems should not replicate.
- Suggested: `aitri backlog add --title "Prune stale keys from alerts.Engine cooldowns map" --priority P3 --problem "engine.go:28 cooldowns map grows unbounded for Docker/Systemd keys as container/service names churn. Either prune during evaluate() (drop entries > 2x max cooldown old) or limit the map size."`

**[BL-4]** `[priority: P3]` — ALTER TABLE migrations swallow errors
- File: `internal/database/sqlite.go:183-187`
- Problem: The three `ALTER TABLE ... ADD COLUMN` migrations use `_, _ = db.Exec(...)`. The pattern is "idempotent ADD COLUMN" (safe to re-run), but other failure modes (disk full, DB corrupted, permission denied) are silently ignored. Startup then continues with a partially-migrated schema; later queries fail cryptically ("no such column"). At minimum the error should be logged and classified (ignore "duplicate column" string, surface everything else).
- Suggested: `aitri backlog add --title "ALTER TABLE migrations swallow all errors, not just duplicate-column" --priority P3 --problem "sqlite.go:183-187 uses _,_ = db.Exec(...) so any failure mode that isn't 'column already exists' is silent. Replace with an error check that logs non-'duplicate column' errors and optionally aborts startup."`

**[BL-5]** `[priority: P2]` — Backup destination path accepted from settings without server-side validation
- File: `internal/server/server.go:305-308` (consumer) / settings handler (input side)
- Problem: `performAutomatedBackup` reads `backupPathOverride` from the settings store and passes it through `filepath.Clean` + `os.MkdirAll`. The admin UI is the only writer, but there is no server-side validation that the path (a) does not traverse outside a sensible root (e.g. `/etc`, `/home/<other-user>`), (b) is not a symlink, (c) is writable by the ultron-ap user. Combined with the `VACUUM INTO '<path>'` string-concatenated SQL in `db.Backup()` (sqlite.go:210), the path is embedded into a SQL statement after only single-quote escaping. Admin-only access limits blast radius, but this is still a soft underbelly.
- Suggested: `aitri backlog add --title "Validate backup local_path server-side (allowlist root, reject symlinks, forbid non-printable chars)" --priority P2 --problem "server.go:305-308 + sqlite.go:210 accept any path the admin submits. Add validation: (a) must be absolute and under a configured BACKUP_ROOT, (b) reject symlinks, (c) reject NULL bytes and control chars (defense against SQL path injection in VACUUM INTO which cannot be parameterized)."`

**[BL-6]** `[priority: P3]` — `tmpl.Execute` errors ignored in login renderer
- File: `internal/server/handlers_auth.go:154`
- Problem: If template execution partially succeeds then errors, the client sees a truncated HTML page with HTTP 200 and no diagnostic. Logging the error would make production debugging easier and is the standard Go pattern.
- Suggested: `aitri backlog add --title "Log template.Execute error in renderLogin" --priority P3 --problem "handlers_auth.go:154 ignores tmpl.Execute error. Other render paths (render.go) do similar but at least log. Wrap errors with log.Printf and unify template error handling across handlers."`

---

## Observations

**[OBS-1]** — Privileged helper trust model is socket-permission only
- Context: `cmd/ultron-helper/main.go:43-62`, `internal/privileged/client.go`
- Concern: The helper listens on a Unix socket with mode `0660`. Any process owned by the socket's owning group can send a JSON request and cause `systemctl start/stop/restart`, `shutdown -r now`, `shutdown -h now`, or pull `journalctl` for arbitrary units. Authorization is "if you can open the socket, you can issue any command." For the intended single-operator Raspberry Pi this is fine, but there is no defense-in-depth if the ultron-ap process is compromised (since it runs as the same group).
- Why deferred: The stated architecture (ADR-1) explicitly picks the socket-permission model as the trust boundary. Adding per-command authorization (e.g. HMAC over request body, or inode-based ucred check) would be a real architectural change, not a quick patch. Documenting the limitation in `DEPLOYMENT.md` may be the right immediate step — a new feature later.

**[OBS-2]** — Dangerous-action countdown is purely client-side UX
- Context: `internal/server/handlers_system.go:73-95` (`validateDangerousAction`)
- Concern: The server only verifies that `countdown_ack=1` was submitted. A malicious script that bypassed CSRF + origin + confirm-word could still submit the flag and skip the countdown. The UX delay doesn't protect against an already-compromised session, it only slows an accidental click.
- Why deferred: Genuine protection (countdown) requires server-side temporal state (nonce issued N seconds ago). That's meaningful complexity for a soft control that sits behind authenticated-admin + CSRF + same-origin + typed-word confirmation — three real gates. Noting the layering, not fixing.

**[OBS-3]** — Content Security Policy is in `Content-Security-Policy-Report-Only`
- Context: `internal/server/middleware.go:63`
- Concern: Any XSS that slipped past `html/template` escaping would not be blocked by the browser, only reported (and the app does not collect reports). `'unsafe-inline'` in both script-src and style-src means a future switch to enforcing CSP still wouldn't block inline injection.
- Why deferred: The inline comment explicitly acknowledges this is staged: "Enforce-ready CSP can break inline templates/scripts; run in report-only first." Switching to enforcing requires template rework to nonce-ify or externalize scripts/styles — not a quick fix. Legitimate staging choice, not a bug.

**[OBS-4]** — Docker/Systemd previous-state maps rebuilt every cycle
- Context: `internal/alerts/engine.go:190-228, 232-271`
- Concern: `prevDocker` and `prevSystemd` are fully replaced each tick (`e.prevDocker = current`). This is correct and intentional (removed containers stop generating alerts), but coupled with a single-goroutine evaluation loop it means if monitor cadence ever drifts to sub-second, the `evaluate` → DB query path becomes the bottleneck before the map becomes a concern.
- Why deferred: Current cadence (5s system, 10s docker, 30s systemd) leaves plenty of headroom. Mentioning for future tuning only.

**[OBS-5]** — `bin/aitri.js VERSION` mismatch-at-rest pattern
- Context: external — see Aitri feedback report
- Concern: Not an Ultron finding; moved to feedback report on Aitri. Kept here only as a breadcrumb: the Aitri pipeline experience on Ultron surfaced several Aitri-side issues that are documented separately.
- Why deferred: Out of scope for an Ultron audit.
