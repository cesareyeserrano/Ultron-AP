# Ultron Production Readiness Execution (Pi5)

Date: 2026-02-24  
Target: Raspberry Pi 5 host `192.168.1.29` (`Ultron`)  
Goal: Execute 6 pending analyses for low-resource + secure production posture.

## 1) Load/Performance Profiling (Executed)

Method:
- Live measurements via SSH with `curl` latency sampling and process snapshots.
- Tool constraints on host: no `wrk`, `ab`, `hey`, `pidstat`, `go`, `govulncheck`.

Results:
- Process baseline:
  - `ultron-ap` RSS ~25.8 MB, CPU ~0.1%.
- `/health` latency (200 samples):
  - avg `0.0004s`, p95 `0.0005s`, p99 `0.0005s`.
- `/login` latency (100 samples):
  - avg `0.0004s`, p95 `0.0005s`, p99 `0.0005s`.
- Concurrent burst `/health` (20x100 requests):
  - Process RSS stable (~26.4 MB), CPU ~0.1%.

Interpretation:
- Excellent base footprint and low overhead for unauthenticated endpoints.
- Authenticated SSE-heavy scenario still needs dedicated profiling once test credentials/load tooling are available.

## 2) Resilience/Operational Stability (Executed, non-disruptive)

Results:
- `ultron-ap` service active and healthy.
- Restart policy:
  - `Restart=on-failure`, `RestartUSec=5s`, `NRestarts=0`, `ExecMainStatus=0`.
- Dependency context on host:
  - `docker.service` not installed.
  - `tailscaled.service` not installed.

Interpretation:
- Service stability is good in current environment.
- Product paths depending on Docker/Tailscale should be validated in an environment where those services are present.

## 3) Backup + Restore Drill (Executed)

Results:
- Backup inventory present under `/var/lib/ultron-ap/backups`.
- Integrity checks (`PRAGMA integrity_check`) on live DB + last backups: `ok`.
- Restore drill:
  - Copied latest backup to temp path.
  - Integrity `ok`.
  - Core tables present: `User`, `Session`, `Alert`, `AlertConfig`, `NotificationConfig`, `ActionLog`.

Interpretation:
- Backup artifacts are currently restorable and consistent.
- Security concern remains: backups are plain SQLite files.

## 4) Basic Pentest (Executed)

Results:
- Auth boundaries:
  - UI protected routes redirect (`303`) to login.
  - Protected API routes return `401` when unauthenticated.
- Login CSRF guard:
  - `POST /login` without CSRF token returns `403`.
- Security headers:
  - Missing global baseline headers on `/login` and `/health` (no CSP/XFO/Referrer-Policy/etc observed).

Interpretation:
- Core auth and login CSRF checks are functioning.
- Browser-hardening headers remain a confirmed gap.

## 5) Host Permissions + Secret Hygiene (Executed)

Permissions snapshot:
- `/etc/ultron-ap/ultron-ap.env` is strict (`600`, `ultron:ultron`) good.
- DB and backups are world-readable by mode:
  - `/var/lib/ultron-ap/ultron.db` -> `644`
  - `/var/lib/ultron-ap/backups/*` under `755` directory.

Sudo exposure (for `ultron`):
- NOPASSWD broad command patterns include:
  - `systemctl start/stop/restart *`
  - `pironman5 *`
  - `shutdown *`
  - `journalctl`, `dmesg`.

Journal secret scan:
- No obvious credential leaks found in last 500 lines.
- Repeated production error found:
  - hardware template failure on missing `CSRFToken` field.

Interpretation:
- Env file protection is good.
- DB/backup file modes and sudo scope are wider than ideal for hardening.

## 6) Supply Chain Review (Executed with limitations)

What was executed:
- Dependency inventory from `go.mod`.
- Local attempt to run `go list -m all` failed due network DNS restriction to `proxy.golang.org`.
- `govulncheck` unavailable in current toolchain.

Interpretation:
- Full CVE verification is blocked by environment/tool availability.
- Action required: run `govulncheck ./...` in connected CI or secured build host.

## Additional Production Finding

- Confirmed runtime bug in production logs:
  - `hardware.html` render failure due missing `.CSRFToken` in `hardwarePageData`.
- This is already recorded in Aitri feedback (`FB-2`).

## Aitri Feedback Registration Status

- `FB-1`: improvement (Pi5 efficiency + security hardening roadmap)
- `FB-2`: high bug (hardware page CSRFToken template failure in production)
- `FB-3`: requirement-gap (backup module must be fully parameterizable from settings)
- `FB-4`: improvement (production-readiness execution summary and pending hardening closures)

## Decision Snapshot

- Runtime base performance: PASS
- Backup/restore integrity drill: PASS
- Auth boundary + login CSRF baseline: PASS
- Browser hardening headers: FAIL
- Least-privilege host posture: FAIL (partial)
- Supply-chain CVE verification: INCOMPLETE (environment-blocked)

Overall readiness: **Conditional GO** for controlled/private usage only.  
Public/Internet-facing GO should wait for hardening items closure.
