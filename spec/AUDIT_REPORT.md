# Project Audit — Ultron-AP

**Latest refresh:** 2026-06-15 · **Auditor:** Claude (Opus 4.8) — on-demand `aitri audit`
**Scope:** `/Users/cesareyeserrano/Documents/PROJECTS/Ultron` — `cmd/`, `internal/`
**Dimensions:** Code Quality · Architecture · Logic · Security · Stack
**Method:** 4 parallel deep-reads (security core · HTTP server · network/alerts · notify/insights/metrics/docker); top-severity findings re-verified by hand.

> **Prior audit (2026-05-03) — all findings resolved.** BUG-1..4 and BL-1..6 from the May review are fixed and re-verified this pass: SO_PEERCRED peer check (`peercred_linux.go`), empty-allowlist fail-closed (BG-043), plaintext-secret refusal (BG-044), login CSRF ordering, XFF trusted-proxy gating, backup path validation (`backup_path.go`), persistent brute-force lockout (`database/bruteforce.go`), atomic audit-log tx (`actions_atomic_test.go`), helper log filtering (`logfilter/`). The original May section is archived at the bottom of this file.

---

## Audit 2026-06-15 — Findings

### Findings → Bugs

**[BUG-A]** `[severity: high]` — `RingBuffer.All()` re-acquires its own read lock (recursive RLock → deadlock)
- File: `internal/metrics/ringbuffer.go:68-72` → calls `History()` at `:48-50`
- Problem: `All()` takes `mu.RLock()` then calls `History()`, which takes `mu.RLock()` again. Go's `sync.RWMutex` is write-preferring and explicitly forbids recursive read-locking: if a `collector` `Add()` (the writer) queues between the two acquisitions, the second `RLock` blocks forever and the goroutine deadlocks holding the buffer — metrics collection and every reader of that buffer stall permanently. Trigger is timing-dependent (writer must arrive mid-call) but `Add()` fires every collection interval, so the window is live.
- Suggested: `aitri bug add --title "RingBuffer.All recursive RLock deadlocks under concurrent Add" --severity high --description "All() calls History() while already holding RLock. Inline the body or add an unlocked historyLocked(n) helper so the lock is taken once."`

**[BUG-B]** `[severity: medium]` — Dashboard chart window is server-global state shared across all clients
- File: `internal/server/sse.go:421-424` (write), `:427-429` (read)
- Problem: `setDashboardChartWindow` stores the selected window into `s.dashboardChartWindow` / `s.dashboardHistoryPoints`, which are **server-wide** atomics read by `gatherDashboardData` for every session. Two operators (or two browser tabs) selecting different windows clobber each other — there is no per-client/per-connection window state.
- Suggested: `aitri bug add --title "Dashboard chart window is server-global, leaks across clients" --severity medium --description "Move chart-window/points to per-connection (SSE client) or per-request state instead of a Server-level atomic."`

**[BUG-C]** `[severity: medium]` — Docker `Monitor.client` read without lock (data race / use-after-close)
- File: `internal/docker/monitor.go:128` (`ContainerDetail`), `:222`/`:242` (refresh), `:104` (`Stop` close)
- Problem: `refresh()` writes/closes `m.client` under `m.mu`, but `ContainerDetail()` and `fetchStats()` read/deref `m.client` without the lock. Concurrent on-demand detail requests during a refresh (or during `Stop()`, which closes the client after `wg.Wait()`) are a data race that `-race` will flag and can call methods on a closed client.
- Suggested: `aitri bug add --title "Docker Monitor.client accessed without lock in ContainerDetail/fetchStats" --severity medium --description "Read m.client under m.mu (or snapshot it once under lock) in all accessors; guard against use-after-Close."`

**[BUG-D]** `[severity: medium]` — ICMP probes accept replies without matching source/ID/Seq (cross-talk)
- File: `internal/network/gatewayprobe/gatewayprobe.go:417`, `internal/network/landevices/sweep/icmp_transport.go:88`
- Problem: Both probers open per-target SOCK_DGRAM sockets and return success on the first echo reply seen, discarding the `ReadFrom` source address and never matching Echo ID/Seq. With concurrent probes (sweep runs 32 workers), a reply for host A can satisfy host B's probe — false "responders" in LAN discovery and cross-talked gateway RTTs. Correctness, not security (single-admin LAN tool).
- Suggested: `aitri bug add --title "ICMP probes don't match reply source/ID/Seq under concurrency" --severity medium --description "Verify ReadFrom source == target and Echo ID/Seq match the sent packet; continue on mismatch instead of accepting."`

**[BUG-E]** `[severity: medium]` — Telegram `message_id` parse failure swallowed → storm coalescing degrades silently
- File: `internal/notify/telegram.go:255-259`
- Problem: A successful send whose JSON body fails to decode returns `(0, nil)`. The caller records `(ruleID, 0)`, so the next in-window fire cannot edit the prior message and posts a fresh one — storm protection silently degrades with no log line.
- Suggested: `aitri bug add --title "Telegram message_id decode failure swallowed, breaks storm coalescing" --severity medium --description "Log/propagate the decode error so a sent-but-unparseable response doesn't silently disable edit-in-place coalescing."`

**[BUG-F]** `[severity: medium]` — Notify dispatcher `Stop()` does unguarded `close(cancel)` → panic
- File: `internal/notify/dispatcher.go:150-154`
- Problem: `Stop()` calls `close(d.cancel)` with no guard. A second `Stop()`, or `Stop()` before `Start()` (when `cancel` is nil), panics with "close of nil/closed channel".
- Suggested: `aitri bug add --title "Dispatcher Stop() panics on double-stop or stop-before-start" --severity medium --description "Guard cancel with sync.Once or a nil check so Stop is idempotent and safe before Start."`

**[BUG-G]** `[severity: low]` — Login dummy bcrypt hash is malformed → username-enumeration timing oracle
- File: `internal/server/handlers_auth.go:73`
- Problem: The constant-time dummy hash used for unknown usernames is too short to be a valid bcrypt hash, so `bcrypt.CompareHashAndPassword` returns `ErrHashTooShort` immediately instead of running the key-derivation. The unknown-username path is measurably faster than the known-username path, reintroducing the timing oracle the code intends to close.
- Suggested: `aitri bug add --title "Login dummy bcrypt hash malformed, reopens username-enumeration timing oracle" --severity low --description "Replace with a valid precomputed bcrypt hash (correct length/cost) so the compare runs full key derivation on the unknown-user path."`

**[BUG-H]** `[severity: low]` — Notification save ignores unmarshal error of existing config → drops saved fields
- File: `internal/server/handlers_notifications.go:72`
- Problem: `json.Unmarshal([]byte(existing.Config), &config)` discards its error. If the stored config is corrupt, the handler starts from an empty map and overwrites previously-saved fields (e.g. an unedited bot token) instead of surfacing the corruption. Distinct from the already-fixed swallow on the test path (`:130`).
- Suggested: `aitri bug add --title "Notification save ignores unmarshal error of existing config" --severity low --description "Capture the err; on corrupt stored config return an error to the operator rather than silently overwriting with an empty config."`

### Findings → Backlog

**[BL-A]** `[priority: P2]` — Insights per-rule `compiledRule.state` mutated with `s.mu` released (latent data race)
- File: `internal/insights/insights.go:246-291` (and `:439-491`)
- Problem: `Eval` snapshots `compiledRules` under `s.mu`, releases the lock, then mutates each `cr.state` (firstEmittedAt, lastValue, lastEvaluatedAt, hysteresis fields). Safe only while `Eval` runs on a single goroutine; any concurrent `Eval` or a concurrent reader of `cr.state` is an immediate `-race` violation. Latent today, a footgun for any future concurrent caller.
- Suggested: `aitri backlog add --title "Guard insights per-rule state mutation under the service lock" --priority P2 --problem "cr.state is mutated with s.mu released. Either hold the lock across the eval loop or move per-rule state behind its own mutex."`

**[BL-B]** `[priority: P2]` — Alert engine shared maps read unlocked; `processedNet` grows unbounded
- File: `internal/alerts/engine.go:501-502`
- Problem: `processedNet`, `prevDocker`, `prevSystemd` are accessed without/with inconsistent locking (safe only because one `run` goroutine touches them today). Separately, `processedNet` keys (`ruleID:eventID:kind`) are never pruned, unlike `cooldowns`/`firingFirst`, so the map leaks one entry per net event for the process lifetime.
- Suggested: `aitri backlog add --title "Prune processedNet and document single-goroutine invariant in alert engine" --priority P2 --problem "processedNet grows unbounded; add TTL/prune like cooldowns. Also gate prevDocker/prevSystemd/processedNet behind the engine lock to survive a future concurrent reader."`

**[BL-C]** `[priority: P2]` — Email SMTP send leaks a goroutine on timeout / ctx-cancel
- File: `internal/notify/email.go:87-100`
- Problem: On the 10s timeout or `ctx.Done()` branch, the spawned goroutine keeps running the non-context-aware `smtp.SendMail` until the OS socket times out. Under repeated slow-SMTP fan-out these goroutines + connections accumulate.
- Suggested: `aitri backlog add --title "Bound SMTP send goroutine lifetime on timeout" --priority P2 --problem "smtp.SendMail isn't ctx-aware; on timeout the goroutine lingers. Use a Dialer with deadline + explicit Close so the worker exits when the send is abandoned."`

**[BL-D]** `[priority: P2]` — Docker refresh passes long-lived ctx with no per-call timeout → hung socket stalls `Stop()`
- File: `internal/docker/monitor.go:242, 323`
- Problem: The refresh loop hands the process-lifetime ctx straight to `ContainerList`/`ContainerStats` with no per-iteration timeout. A hung Docker socket blocks a refresh (and `wg.Wait()` in `Stop`) indefinitely.
- Suggested: `aitri backlog add --title "Add per-call timeout to docker refresh requests" --priority P2 --problem "Wrap each ContainerList/ContainerStats in context.WithTimeout so a stuck docker socket can't block the refresh loop or Stop()."`

**[BL-E]** `[priority: P2]` — Backup retention deletes oldest-by-filename across all directory entries
- File: `internal/server/server.go:382-397`
- Problem: The retention sweep `os.ReadDir(backupDir)` then removes `files[0:len-retention]`, assuming lexical order == chronological and that every entry is a backup. With a mixed `.db`/`.db.enc` set, stray files, or a custom `local_path`/BackupRoot containing unrelated files, it can delete the wrong files and counts non-backups toward the limit.
- Suggested: `aitri backlog add --title "Scope backup retention to backup files, sort by mtime" --priority P2 --problem "Filter ReadDir to the ultron-*.db(.enc) prefix and sort by modtime before pruning, so retention never deletes unrelated files in a custom backup dir."`

**[BL-F]** `[priority: P3]` — Dispatcher rebuilds notifiers + `storm.Cache` per send → coalescing/janitor dead in prod path
- File: `internal/notify/dispatcher.go:241, 601-621`; `internal/notify/storm/storm.go:154`
- Problem: Each `send` re-queries SQLite and constructs fresh `TelegramSender` instances, each with a brand-new `storm.Cache`; `RunJanitor` is never started from the dispatcher. So edit-in-place storm coalescing never persists across consecutive fires through the dispatcher path, and the janitor is effectively dead code.
- Suggested: `aitri backlog add --title "Reuse long-lived notifier/storm cache in dispatcher" --priority P3 --problem "Cache constructed senders across sends (rebuild only on config change) and start RunJanitor once, so storm coalescing actually works in the dispatcher path."`

**[BL-G]** `[priority: P3]` — Sustained-window cadence estimated from min observed gap → window fires off by a cadence
- File: `internal/insights/lang/lang.go:311-341`
- Problem: `cadenceMS` uses the smallest-ever tick gap (not a median, despite the comment) and `covered = (now-start)+cadence`; a single fast tick permanently shrinks the cadence estimate and skews when `window_ms` is considered satisfied.
- Suggested: `aitri backlog add --title "Use median/expected cadence for sustained-window coverage" --priority P3 --problem "Min-gap cadence biases sustained rules. Track the configured/expected cadence or a median so window satisfaction matches window_ms intent."`

**[BL-H]** `[priority: P3]` — Brute-force tracker fails open on DB error with no alerting
- File: `internal/auth/bruteforce.go:103-105, 82-84`
- Problem: `IsLocked` returns `false` on any lookup error and `RecordFailure` only logs on write failure. Sustained SQLite errors (lock contention / disk-full on the Pi) disable lockout entirely. The fail-open posture is intentional for availability but is an auth-control gap with no signal.
- Suggested: `aitri backlog add --title "Surface brute-force tracker DB failures" --priority P3 --problem "On repeated lookup/write errors, emit a warning/health signal so a silently-disabled lockout is visible rather than fully fail-open."`

### Observations

**[OBS-A]** — Scheduled backup does not re-validate the stored path for symlinks (TOCTOU)
- Context: `internal/database/sqlite.go:303-327` (`Backup`) consuming the stored `LocalPath`; scheduled call site `internal/server/server.go:343`
- Concern: `Backup` only rejects NUL/control/relative paths; the full `rejectSymlinkChain` check in `ValidateBackupPath` runs only at config-save time. A component symlinked into the backup dir *after* save can redirect `VACUUM INTO` outside `BackupRoot` on the next scheduled run. Requires write access to the backup dir (same trust domain), so impact is bounded.
- Why deferred: Defense-in-depth, not an exploit on the single-admin model; fix is to re-run `ValidateBackupPath` at backup time.

**[OBS-B]** — Helper `handleShutdown` lacks the timeout / process-group hardening of `run()`
- Context: `cmd/ultron-helper/main.go:282-286`
- Concern: Shutdown uses a bare `exec.Command(...).Start()` with no context timeout and no `Setpgid`, unlike the hardened `run()` path; a hung `shutdown` binary leaks an unsupervised privileged child. Low impact (single-shot, allowlisted mode).
- Why deferred: Inconsistency, not a defect; aligning it with `run()` is a small cleanup.

**[OBS-C]** — Non-Linux helper stub disables peer-cred enforcement
- Context: `cmd/ultron-helper/peercred_other.go:13-17` with `main.go:124`
- Concern: On non-Linux builds `peerCredSupported=false`, so the allowlist/fail-closed block is skipped and the helper trusts socket file perms alone. Correct for the linux/arm64 production target; latent risk only if the helper is ever run on macOS/BSD in dev.
- Why deferred: Out of the deployment target; documenting the latent gap.

**[OBS-D]** — DB backup download serves plaintext DB even when encryption is enabled
- Context: `internal/server/handlers_performance.go:184-197`
- Concern: `GET /api/settings/backup` streams an unencrypted copy of the whole DB (session tokens, bcrypt hashes, notification secrets) to any authenticated user even when `backupEncrypt` is on. Acceptable for single-admin, but a secrets-at-rest exposure if the file is shared.
- Why deferred: Matches the single-operator threat model; flagging the expectation mismatch with the encrypt setting.

**[OBS-E]** — WAN monitor may never declare `WAN_DOWN` if a gateway sample hasn't been observed
- Context: `internal/network/wanmonitor/wanmonitor.go:119`
- Concern: Down requires `lastGatewayOK==true` when the public-failure threshold is crossed; if the gateway snapshot hasn't arrived yet (`lastGatewayOK` defaults false), repeated public failures never transition to down and nothing re-evaluates once the gateway is later confirmed.
- Why deferred: Edge timing at startup; needs a small state-machine review rather than a one-line fix.

**[OBS-F]** — Telegram 4096 truncation counts bytes on a rune/escape-encoded body
- Context: `internal/notify/telegram.go:196, 222, 265`
- Concern: `len(text) > 4096` and `text[:4093]` slice on bytes can split a multi-byte UTF-8 rune or a `\`-escape pair, producing a MarkdownV2 body Telegram rejects with 400. Only bites very long messages.
- Why deferred: Rare path; correct fix is rune-aware truncation that won't split an escape.

**[OBS-G]** — Alert engine `==` operator uses exact float equality (effectively dead)
- Context: `internal/alerts/engine.go:885`
- Concern: Exact `==` on floats (CPU%, RTT) will almost never match a real sampled metric, so any rule using the `==` operator is effectively inert.
- Why deferred: No incorrect behavior for the common `>`/`<` rules; revisit if `==` rules are ever offered in the UI.

---

## Archived Findings — 2026-05-03 (all resolved, retained for history)

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

---

### Requirements Coverage

**Executed:** 2026-06-15 · `aitri audit coverage`
**Direction:** backward — intent (`00_DISCOVERY.md` + `original_brief`) → functional requirements.
**Sources traced:** approved discovery (problem, users/JTBD, success criteria, out-of-scope) and the Phase-1 `original_brief`. No standalone `IDEA.md` on disk; the brief is its absorbed form.

**Verdict: 24 needs traced · 24 covered · 0 partial · 0 uncovered.**

Complete — every traced need maps to an FR/NFR or an explicit out-of-scope line. Two FRs add scope the intent never expressed (reverse-check questions below), but neither is a coverage gap.

**Needs traced → covering requirement:**

| # | Client-expressed need (source) | Covered by |
|---|---|---|
| 1 | Centralized real-time system visibility *without SSH* (discovery problem; brief) | FR-001 + FR-010 |
| 2 | CPU monitoring (brief; discovery) | FR-001 |
| 3 | RAM monitoring | FR-001 |
| 4 | Disk monitoring | FR-001 |
| 5 | Network traffic monitoring | FR-001 |
| 6 | CPU temperature monitoring | FR-001 |
| 7 | Live updates via SSE | FR-001 (+ all SSE-fed views) |
| 8 | Docker container visibility | FR-002 |
| 9 | Systemd service visibility | FR-003 |
| 10 | Service controls — restart/start/stop *without SSH* (brief: "restarting containers") | FR-008 |
| 11 | View logs without SSH (brief: "viewing logs") | FR-010 |
| 12 | Configure alerts (brief: "configuring alerts") | FR-004 |
| 13 | Threshold-based alert engine (brief) | FR-004 |
| 14 | Telegram notifications (brief; out-of-scope allows Telegram) | FR-005 |
| 15 | Email/SMTP notifications (brief; out-of-scope allows SMTP) | FR-006 |
| 16 | Pironman 5 hardware integration (brief) | FR-013 |
| 17 | CSRF protection (brief) | FR-012 |
| 18 | bcrypt sessions (brief) | FR-007 |
| 19 | Brute-force protection (brief) | FR-007 |
| 20 | Full action audit trail (brief) | FR-008 (AC: audit-trail in SQLite) |
| 21 | Privilege separation — unprivileged web + root helper over Unix socket (brief; discovery constraint) | FR-011 |
| 22 | Single-admin auth; panel not open to anyone on LAN (discovery JTBD; US-007) | FR-007 |
| 23 | Dark, responsive, WCAG 2.1 AA UI (discovery success criterion; brief "professional dashboard") | FR-009 |
| 24 | Act on issues in ≤ 2 interactions / critical state above the fold (discovery success criteria) | FR-001 (above-the-fold AC) + FR-008/FR-009 |

**Quantified success criteria → NFRs:** latency ≤ 5 s → NFR-004 · ≤ 15 MB RAM on ARM → NFR-001 · single Go binary / zero runtime deps → NFR-002 / NFR-003 · WCAG AA contrast → FR-009 ACs.

**Out-of-scope boundaries respected (not reported as gaps):** multi-node/cluster · external cloud beyond Telegram/SMTP · SPA frameworks · runtime deps beyond the Go binary · public-internet exposure. None of these surfaced as a missing need.

**Reverse-check — FRs with no traceable client need (questions, not gaps):**

- **[RC-1]** FR-014 *VPN status (Tailscale)* `[COULD]` — neither the discovery nor the brief mentions Tailscale or VPN status; it is additive scope. **RESOLVED 2026-06-15 — accepted *with caveat*.** Kept as an optional `[COULD]` value-add (already implemented and verified). Caveat recorded against the discovery out-of-scope line *"Runtime dependencies beyond the Go binary itself"*: FR-014 functions only when `tailscale`/`tailscaled` is present on the host (its ACs are conditional — *"given Tailscale is installed and running"*). It is therefore an **optional, gracefully-degrading** capability, explicitly **NOT a runtime dependency of the binary** — on a host without Tailscale it simply renders nothing and the binary still runs standalone. It is also a *VPN/mesh* feature, not a cloud integration, so it does not contradict *"External cloud integrations beyond Telegram/SMTP."* No Phase-1 change; no action.
- **[RC-2]** FR-015 *Database backup* `[SHOULD]` — ~~the brief lists SQLite only for persistence ("config, users, alerts, history"); neither discovery nor brief asks for backup/restore.~~ **RESOLVED 2026-06-15 — accepted as intentional scope.** Encrypted backup/restore is a deliberate disk-failure-resilience feature for the Pi and is live in production (the `ULTRON_BACKUP_KEY` is provisioned on the device and recoverable from the operator's Keychain). Not a coverage concern; no action.

No action needed to close gaps — there are none. The two reverse-check items are optional housekeeping (record why the extra FRs exist).
