# Ultron Pi5 E2E Audit Notes (Performance + Cybersecurity)

Date: 2026-02-24  
Scope: End-to-end audit of runtime architecture, request lifecycle, collectors, SSE/UI rendering, privileged operations, and deploy hardening.

## Executive Summary
- Current state is functionally solid and test coverage is improved, but there are still high-impact optimization and hardening opportunities.
- Main resource risk for Raspberry Pi 5 is unnecessary continuous work (SSE + monitors + rendering) when pages that do not need live dashboard data are open.
- Main security risk is operational privilege exposure (service/sudo model) and missing baseline HTTP hardening headers.

## Findings (Ordered by Severity)

### 1) P1-High: Global SSE connection on all authenticated pages creates avoidable CPU/render load
- Evidence:
  - `sse-connect` is on global layout body: `web/templates/base.html:12`.
  - SSE payload always renders all dashboard partials every tick: `internal/server/sse.go:156`, `internal/server/sse.go:161`, `internal/server/sse.go:165`, `internal/server/sse.go:169`, `internal/server/sse.go:173`.
- Impact:
  - Any open page (settings/history/services) still keeps dashboard SSE alive.
  - Per client, server renders metrics + docker + systemd + charts HTML every interval even when not visible.
  - This is exactly the opposite of "almost imperceptible" resource usage.
- Recommendation:
  - Move `sse-connect` from base layout to dashboard-only template.
  - Add event-selective payload generation (render only sections with active subscribers).
  - Add idle-mode throttling (e.g., 15-30s) when no dashboard tabs are active.

### 2) P1-High: Background monitors and alert loop run continuously regardless of active consumers
- Evidence:
  - Always started at boot: `cmd/ultron-ap/main.go:58`, `cmd/ultron-ap/main.go:64`, `cmd/ultron-ap/main.go:70`, `cmd/ultron-ap/main.go:86`.
- Impact:
  - Constant system polling and Docker/systemd scans even when nobody is using the panel.
  - In Pi5, this is still affordable but unnecessary baseline load and wakeups.
- Recommendation:
  - Introduce adaptive scheduler:
    - Low-power baseline when no active dashboard viewers.
    - Burst mode when at least one live viewer exists.
  - Decouple alert cadence from UI cadence, with conservative default alert interval.

### 3) P1-High: Missing baseline HTTP security headers
- Evidence:
  - No HSTS/CSP/XFO/XCTO/Referrer-Policy/Permissions-Policy set in server code.
  - Header search returned no matches for these headers in `internal/server`.
- Impact:
  - Reduced browser-side protection against clickjacking, MIME sniffing, referrer leakage, and script injection blast radius.
- Recommendation:
  - Add a security middleware for all responses:
    - `X-Content-Type-Options: nosniff`
    - `X-Frame-Options: DENY` (or CSP frame-ancestors)
    - `Referrer-Policy: no-referrer`
    - `Permissions-Policy` minimal allowlist
    - CSP compatible with HTMX/SSE setup
    - HSTS only when TLS is guaranteed at edge.

### 4) P1-High: systemd service hardening is permissive for a control-plane app
- Evidence:
  - `NoNewPrivileges=false`: `deploy/ultron-ap.service:16`.
  - App executes privileged operations via sudo (`shutdown`, `systemctl`, `journalctl`, `dmesg`) in handlers and control modules.
- Impact:
  - If web app is compromised, privilege boundaries are weak.
  - Increases blast radius from web compromise to host control plane.
- Recommendation:
  - Set `NoNewPrivileges=true` if operational model permits.
  - Minimize sudoers scope to explicit commands/units.
  - Add additional unit hardening: `PrivateTmp=true`, `ProtectKernelTunables=true`, `ProtectControlGroups=true`, `ProtectKernelLogs=true`, `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`, `SystemCallFilter` baseline.

### 5) P2-Medium: Global `WriteTimeout=0` for entire HTTP server can increase abuse window
- Evidence:
  - `WriteTimeout: 0` in server config: `internal/server/server.go:70`.
- Impact:
  - Infinite write deadline protects SSE but also weakens protection for non-SSE responses.
- Recommendation:
  - Use finite server `WriteTimeout` for normal requests.
  - Handle SSE separately (dedicated mux/endpoint with per-connection deadline management or separate server instance).

### 6) P2-Medium: Sensitive notification secrets stored in plaintext in SQLite
- Evidence:
  - Notification config stores raw JSON in `config TEXT`: `internal/database/sqlite.go:61`.
  - Upsert/read path uses plaintext `Config` blob: `internal/database/notifications.go`.
- Impact:
  - Token/password exposure risk if DB is exfiltrated or backup channel is compromised.
- Recommendation:
  - Encrypt at rest application secrets (e.g., libsodium/AES-GCM with key outside DB, via env/file with strict perms).
  - Optionally move secrets to OS secret store where available.

### 7) P2-Medium: Full database backup sent to Telegram channel
- Evidence:
  - Automated backup creates DB copy and uploads to Telegram: `internal/server/server.go:152`, `internal/server/server.go:190`.
- Impact:
  - Backup contains sessions, config, and notification credentials; external channel introduces high confidentiality risk.
- Recommendation:
  - Encrypt backup artifact before upload (age/gpg with rotation policy).
  - Add explicit "local-only backup" mode as secure default.
  - Add retention + integrity metadata for encrypted archives.

### 8) P2-Medium: Brute-force keying by `RemoteAddr` may cause proxy-wide lockouts
- Evidence:
  - IP extracted only from `RemoteAddr`: `internal/server/handlers_auth.go:208`.
  - Locking logic keyed by IP in memory map: `internal/auth/bruteforce.go`.
- Impact:
  - Behind reverse proxy, many users can appear from one source and lock each other out.
- Recommendation:
  - Introduce trusted proxy mode with strict source allowlist and safe client IP extraction.
  - Add username+IP compound rate limiting and short jittered backoff.

### 9) P3-Low: DB query hot paths lack explicit indexes for long-term growth
- Evidence:
  - Frequent queries on `Alert.created_at`, `Alert.acknowledged`, `ActionLog.created_at` in `internal/database/alerts.go` and `internal/database/actions.go`.
  - Schema defines tables but no explicit indexes in `internal/database/sqlite.go`.
- Impact:
  - As data grows between prune cycles, query cost rises and can produce avoidable IO on Pi.
- Recommendation:
  - Add indexes:
    - `Alert(acknowledged, created_at DESC)`
    - `Alert(created_at DESC)`
    - `ActionLog(created_at DESC)`
    - `Session(expires_at)`

## Pi5 Performance Target Profile (Recommended)
- Idle mode (no active dashboard clients):
  - SSE effectively off, monitors at low cadence.
- Active mode (1+ dashboard clients):
  - SSE 5-10s.
  - Docker >= 15s unless docker page is active.
  - Systemd >= 30-60s unless services page is active.
- Security mode:
  - HTTPS only via reverse proxy/VPN.
  - Hardened systemd unit + strict sudoers.
  - Security headers + encrypted secret storage.

## Notes Prepared for `aitri feedback`

Use this text as the feedback payload:

```text
Audit focus: Raspberry Pi 5 minimal resource footprint + cybersecurity hardening.

Critical outcomes:
1) High waste path: SSE is globally enabled in base layout, so non-dashboard pages still trigger full dashboard payload rendering every tick. Make SSE dashboard-only and event-selective.
2) Monitors/alerts always run at full cadence even with no active viewers. Add adaptive scheduler (idle vs active mode).
3) Missing baseline HTTP security headers (CSP/XFO/XCTO/Referrer-Policy/Permissions-Policy/HSTS-at-edge).
4) Service hardening is weak for control-plane app (`NoNewPrivileges=false`) and app depends on sudo privileged actions; reduce blast radius with stricter unit and sudo policy.
5) Backup model uploads full SQLite DB to Telegram; require encrypted backups and secure default local-only mode.
6) Notification credentials are stored plaintext in DB config blob; add encryption-at-rest for secrets.
7) Improve reliability/security economics for Pi5 with DB indexes on alert/action/session hot paths and split SSE/non-SSE timeout policy.

Requested backlog conversion:
- EP: Pi5 Runtime Efficiency & Adaptive Polling
- EP: Web Security Headers & Host Hardening
- EP: Secret Management & Encrypted Backup Chain
- EP: Proxy-Aware Auth Throttling and IP Trust Model
- EP: SQLite Hot-Path Indexing and IO Budgeting
```

## Live Raspberry Pi Validation (SSH 192.168.1.29)

### Runtime Status
- Host: `Ultron` (`Linux 6.12.62+rpt-rpi-2712`, aarch64).
- `ultron-ap` service: active/running for ~2h21m.
- Health endpoint:
  - `GET /health` returned `200` with `{"status":"ok"}`.

### Resource Snapshot (Live)
- Process (`ultron-ap`) memory:
  - RSS ~`25 MB` (`25856 KB`), `%MEM ~0.3`.
- System memory:
  - `7.9 GiB` total, ~`554 MiB` used.
- CPU/load at capture:
  - `load average 0.31 / 0.42 / 0.44` and CPU mostly idle.

### Security Header Validation (Live)
- `GET /login` response headers do not include baseline hardening headers (no CSP/XFO/XCTO/Referrer-Policy observed).
- `GET /health` likewise returns minimal headers only.
- Confirms static audit findings around missing HTTP hardening middleware.

### New Production Bug Found
- Repeated runtime error in journal:
  - `Failed to execute template hardware.html: template: hardware-form.html:3:52: executing "partials/hardware-form.html" at <.CSRFToken>: can't evaluate field CSRFToken in type server.hardwarePageData`
- Operational impact:
  - Hardware page render failure and repeated noisy sudo/pironman activity in logs.
- Immediate remediation:
  - Ensure `CSRFToken` is passed in `hardwarePageData` path or render through `PageData` consistently.
  - Add integration test that renders `/hardware` and asserts no template execution errors.

## Root-Solution Implementation Blueprint (No Patch Strategy)

### A) Frontend/Template Contract Unification (Root cause of hardware render failure)
- Problem class:
  - Partial templates depend on fields not guaranteed by page-specific structs.
- Root solution:
  - Define a single typed view-model contract for all pages with mandatory shared fields (`CSRFToken`, `Username`, `Version`, etc.).
  - Enforce compile-time usage by rendering through one canonical `PageData` envelope.
  - Ban direct partial execution with ad-hoc structs for authenticated pages.
- Definition of done:
  - All template render entrypoints use a common presenter/builder.
  - `/hardware` render path covered by integration test + template compile smoke test in CI.

### B) Backup Module as First-Class Configurable Subsystem (Settings-driven)
- Requirement (new):
  - Backup module must be fully parameterizable from Settings.
- Root solution:
  - Introduce `BackupConfig` domain object persisted in DB (separate from generic notification config blob).
  - Configurable fields:
    - `enabled`
    - `interval_hours`
    - `retention_count`
    - `destination_mode` (`local_only`, `local_plus_telegram`, future providers)
    - `local_path`
    - `encrypt_enabled`
    - `encryption_key_ref`
    - `max_upload_size_mb`
    - `upload_timeout_sec`
  - Scheduler consumes config dynamically without restart.
  - Backup execution pipeline split into stages:
    - snapshot -> local retention -> encryption -> optional upload -> audit event.
- Definition of done:
  - Settings UI exposes and validates all backup parameters.
  - Config changes apply live.
  - Tests cover retention, encryption on/off, upload success/failure, and rollback-safe behavior.

### C) Security/Operations Baseline by Design
- Standardize HTTP security middleware and systemd hardening profile as non-optional defaults.
- Separate privileged operations into narrow service adapters with explicit allowlists.
- Add threat-model checklist gate before delivery (`auth`, `secrets`, `backup`, `privileged exec`, `headers`).

## Suggested Resolution Plan (Implementation-Oriented)

### Wave 1 (Immediate hardening, low risk)
- SSE scope reduction:
  - Move `sse-connect` from global layout to dashboard template only.
  - Keep `sse.js` loaded globally only if required by dashboard route; otherwise lazy-load.
- Security headers middleware:
  - Add central middleware in HTTP stack with `nosniff`, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`.
  - Add CSP in report-only mode first, then enforce after template/script cleanup.
- systemd unit hardening:
  - Set `NoNewPrivileges=true`.
  - Add `PrivateTmp=true`, `ProtectKernel*` flags, and restrictive `RestrictAddressFamilies`.
  - Keep explicit `ReadWritePaths` minimum.

### Wave 2 (Resource optimization for Pi5)
- Adaptive runtime scheduler:
  - Track active SSE clients.
  - Use low-frequency polling when zero clients.
  - Use current frequency only when one or more dashboard clients are active.
- Timeout split:
  - Keep SSE-friendly behavior on SSE endpoint only.
  - Restore finite `WriteTimeout` for non-stream endpoints.
- DB indexes migration:
  - Add indexes for `Alert`, `ActionLog`, and `Session` hot paths with idempotent migration.

### Wave 3 (Confidentiality and backup security)
- Secret encryption at rest:
  - Encrypt `NotificationConfig.config` fields with a master key from env or root-owned file.
  - Never store master key in DB.
- Backup chain security:
  - Encrypt backup artifact before Telegram upload.
  - Add secure default mode: local encrypted backup only.
  - Add checksum and restore-test routine.

### Wave 4 (Proxy-safe auth and abuse resistance)
- Trusted proxy mode:
  - Explicit allowlist for proxy source IPs.
  - Parse `X-Forwarded-For` only when request comes from trusted proxy.
- Login protection:
  - Rate-limit per `username+IP`.
  - Add exponential/jitter backoff.
  - Add telemetry counters for lockouts and failed attempts.
