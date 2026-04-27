# System Design — Ultron-AP

## Executive Summary

Ultron-AP is a single-node Raspberry Pi monitoring panel. It is delivered as one statically-linked Go binary that serves the web UI, runs all monitors (system metrics, Docker, Systemd, Tailscale), evaluates alert rules, persists state to a local SQLite file, and dispatches notifications to Telegram and SMTP. A second small binary — the privileged helper — runs as root and listens on a local Unix socket; the web process delegates host-level actions (service start/stop/restart, system power) to it.

Design priorities: low memory footprint (~15 MB at idle), zero runtime dependencies beyond the binary, 5-second metric latency end-to-end, and strict privilege separation between the web layer and the host. The runtime is a modular monolith — bounded internal Go packages — chosen over microservices to keep operational and failure surfaces minimal on constrained hardware.

## System Architecture

Single-node deployment with two binaries (`ultron-ap` web/monitor process, `ultron-helper` privileged helper) communicating over a Unix socket.

**Components:**
- **Web App** — Go HTTP server with HTMX endpoints, SSE broadcaster, and server-rendered HTML templates.
- **Metrics Collector** — Reads CPU, RAM, Disk, Network, and temperature; maintains an in-memory ring buffer and snapshot.
- **Docker Monitor** — Polls Docker socket for container state and resource stats.
- **Systemd Monitor** — Reads D-Bus / `systemctl` output for service state.
- **Tailscale Monitor** — Reads Tailscale status for VPN peer visibility.
- **Alert Engine** — Evaluates threshold rules against the current snapshot; persists alerts and triggers notifications.
- **Notify Dispatcher** — Sends Telegram and Email notifications on alert fire/resolution.
- **SQLite Data Store** — Persists configuration, users, sessions, alerts, action history, notifications.
- **Privileged Helper** — Root-owned binary listening on a Unix socket; executes host-level actions on behalf of the unprivileged web process.

### C4 Level 2 diagram

```mermaid
flowchart LR
  user["Raspberry Pi Operator\n(Browser)"] --> web["Ultron Web App\n(Go · HTMX · SSE)"]
  web --> db["SQLite\n(alerts · history · config · auth)"]
  web --> collector["Metrics Collector\n(CPU · RAM · Disk · Net · Temp)"]
  web --> docker["Docker Monitor"]
  web --> systemd["Systemd Monitor"]
  web --> tailscale["Tailscale Monitor"]
  collector --> web
  docker --> web
  systemd --> web
  tailscale --> web
  web --> alerts["Alert Engine"]
  alerts --> db
  alerts --> notify["Notify Dispatcher\n(Telegram · Email)"]
  web -. "Unix socket\n(restricted boundary)" .-> helper["Privileged Helper\n(root)"]
```

### Module map

| Package | Responsibility |
|---|---|
| `internal/server` | HTTP handlers, SSE broker, middleware, templates, render |
| `internal/metrics` | SystemReader, Collector, RingBuffer, snapshot models |
| `internal/docker` | Docker client, monitor, controls, models |
| `internal/systemd` | Systemd monitor, controls, parser, runner, models |
| `internal/tailscale` | Tailscale status reader |
| `internal/alerts` | Alert rule engine |
| `internal/notify` | Dispatcher, Telegram, Email notifiers |
| `internal/database` | SQLite layer (auth, alerts, notifications, history, actions, backup, secrets, perf) |
| `internal/auth` | CSRF token management, brute-force tracker |
| `internal/config` | Environment-based configuration |
| `internal/privileged` | Unix socket client for privileged helper IPC |
| `web/` | Embedded static assets, CSS, JS, templates |

## Data Model

State is partitioned by purpose:

**In-memory (process):**
- `metrics.RingBuffer` — last N samples per metric for chart history.
- `metrics.Snapshot` — most recent reading for SSE push.
- `auth.CSRFManager` — per-session token map.
- `auth.BruteForceTracker` — IP → (failed_count, locked_until).

**Persistent (SQLite — `internal/database`):**

| Table | Key columns | Purpose |
|---|---|---|
| `users` | `id`, `username`, `password_hash` (bcrypt) | Single-admin auth |
| `sessions` | `token`, `user_id`, `expires_at` | Cookie-based sessions |
| `alerts` | `id`, `severity`, `metric`, `value`, `threshold`, `fired_at`, `resolved_at` | Alert history |
| `alert_rules` | `id`, `metric`, `op`, `threshold`, `cooldown_min`, `severity` | Configurable thresholds |
| `notifications` | `id`, `channel`, `enabled`, `config_json` | Telegram/Email settings |
| `action_history` | `id`, `source`, `target`, `action`, `actor`, `result`, `at` | Audit trail for service controls + backups |
| `backups` | `id`, `path`, `created_at`, `size_bytes`, `encrypted` | Backup index |
| `secrets` | `key`, `ciphertext` | Encrypted secrets at rest (Telegram tokens, SMTP creds) |
| `performance` | `id`, `kind`, `value`, `at` | Internal performance counters (verify metric latency NFR) |

Snapshots are not persisted — only ring-buffer in memory, regenerated on restart. Alert history and action history are durable.

## API Design

The panel is server-rendered HTML + HTMX swaps; there is no external JSON API for clients. The internal HTTP surface is documented below.

### HTTP routes (web)

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/login` | public | Login form |
| POST | `/login` | public | Submit credentials; sets session cookie |
| POST | `/logout` | session | Invalidate session |
| GET | `/health` | public | Liveness probe |
| GET | `/` | session | Dashboard (metrics + alert chip) |
| GET | `/docker` | session | Docker container list |
| GET | `/services` | session | Systemd services list |
| GET | `/alerts` | session | Alerts panel with severity filters |
| GET | `/history` | session | Action history (filterable by source) |
| GET | `/logs` | session | Log drawer for a target |
| GET | `/settings` | session | Settings (notifications + backup + hardware) |
| GET | `/events` | session | SSE stream (`text/event-stream`) |
| POST | `/services/:name/{start,stop,restart}` | session + CSRF | Systemd control via privileged helper |
| POST | `/docker/:id/{start,stop,restart}` | session + CSRF | Docker control |
| POST | `/settings/{notifications,backup,hardware}` | session + CSRF | Save settings |
| POST | `/alerts/:id/{ack,mute}` | session + CSRF | Alert lifecycle |

All `POST/PUT/DELETE` routes require a valid CSRF token. All routes except `/login` and `/health` require an authenticated session (302 redirect to `/login` otherwise).

### Privileged helper IPC

Unix socket protocol — one length-prefixed JSON request per connection:

```
{"action": "systemctl", "verb": "restart", "unit": "ultron-ap.service"}
→ {"ok": true, "stdout": "...", "stderr": "", "exit": 0}
```

The helper validates `verb ∈ {start, stop, restart}` and `unit` against an allow-list before executing. Unknown verbs or units return `{"ok": false, "error": "not allowed"}`.

## Security Design

Layered controls aligned to OWASP basics for a self-hosted admin panel:

1. **Authentication** — single admin user; password stored as bcrypt hash; first-run setup or env-var seed; session cookie is HttpOnly, SameSite=Lax, Secure when behind an HTTPS proxy (detected via `X-Forwarded-Proto`).
2. **Brute-force defence** — per-IP failure counter; 5 failures → 15-minute lockout.
3. **CSRF** — per-session token embedded in HTML and validated on every state-changing request; missing/invalid → 403.
4. **Privilege separation (ADR-4)** — the web process runs with `NoNewPrivileges=true` and a non-root user; host-level actions (service controls, system power) are routed through a Unix socket to a root-owned helper that validates against an allow-list.
5. **Secrets at rest** — Telegram tokens and SMTP credentials are encrypted in SQLite (`secrets` table) using a key derived from a host-resident file (mode 0600, owned by the service user).
6. **Backup encryption** — backup files are encrypted at rest using the same key derivation.
7. **Transport** — TLS termination is delegated to a reverse proxy (Nginx / Caddy / Tailscale Funnel). The binary itself listens on plaintext bound to the loopback or Tailscale interface.
8. **Static analysis** — `govulncheck ./...` runs in CI on every push and PR (`security-gate.yml`).

### Threat model — out of scope (deliberate)
- Multi-user access control (single-admin model).
- Public-internet exposure (LAN/VPN only).
- Cluster-wide secrets management.

## Performance & Scalability

Targets:
- Memory: ≤ 15 MB at idle on ARM Cortex-A.
- CPU: < 5 % single-core at idle.
- Metric latency end-to-end (host change → browser): ≤ 5 s.
- SSE clients supported: ≥ 5 concurrent on Pi 4 / Pi 5.

Mechanisms:
- **Differentiated SSE cadences** — metrics 5 s, Docker 10 s, Systemd 30 s. Reduces fan-out cost on constrained hardware.
- **In-memory ring buffer** — chart history is bounded (60 minutes ≈ 720 samples at 5 s) to avoid SQLite write amplification.
- **Single SQLite writer** — short transactions, bounded retries with jitter on `SQLITE_BUSY`. WAL mode enabled.
- **Embedded assets** — CSS/JS/templates compiled into the binary (`go:embed`); no filesystem cost beyond binary size.
- **Bounded notifier queue** — Dispatcher drops oldest message under sustained pressure rather than blocking the alert engine.

Scalability is intentionally not a property of this system — it is a single-node panel for a single host. Horizontal scaling is out of scope.

## Deployment Architecture

Target: Raspberry Pi (ARM64) running Linux with Systemd. Two systemd services:

| Service | User | Path | Purpose |
|---|---|---|---|
| `ultron-ap.service` | unprivileged | `/usr/local/bin/ultron-ap` | Web app + monitors |
| `ultron-helper.service` | root | `/usr/local/bin/ultron-helper` | Privileged actions over Unix socket |

Both are managed via systemd. `ultron-ap.service` declares `NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true`. The helper socket lives at `/run/ultron-helper.sock` (mode 0660, group `ultron`).

### Build pipeline

`make build-arm` cross-compiles `linux/arm64` static binaries via `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build`. Output: `bin/ultron-ap-linux-arm64` and `bin/ultron-helper-linux-arm64`. Tailwind CSS is compiled by `make css` into `web/static/css/app.css` and embedded via `go:embed`. There is no Docker image — the deployment artifact is the binary plus the systemd unit files, sudoers entry, and `.env` template under `deploy/`.

### Configuration

Environment variables only — no config file. See `04_IMPLEMENTATION_MANIFEST.json:environment_variables` for the canonical list.

### Backup target

`/var/lib/ultron-ap/db.sqlite` (DB) + `/var/lib/ultron-ap/backups/*.enc` (encrypted snapshots). Retained per the schedule configured in settings.

## Risk Analysis

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Helper socket misconfigured (writable by web user) | Low | High — privilege escalation | Socket mode 0660, group `ultron`, audit on every command |
| SQLite write contention under burst alerts | Medium | Medium — alerts lost or delayed | Bounded retries with jitter; WAL mode |
| SSE fanout drives CPU on multi-client Pi | Low | Medium — dashboard lag | Per-channel cadence; compact payloads |
| Telegram/SMTP outage masks alert delivery | Medium | Medium — silent failure | Alert still recorded in SQLite; Test buttons in settings; failure visible in action history |
| Disk full breaks SQLite writes | Low | High — silent data loss | `performance` table monitors free space; alert when < 10 % |
| Backup key file lost | Low | High — backups un-restorable | Operator instructed to keep key offsite; documented in DEPLOYMENT.md |

## Technical Risk Flags

- [RISK] **Single point of failure on host node.** No redundancy. Out of scope to address; documented in Discovery as accepted scope. Mitigation: backups + restore documented procedure.
- [RISK] **Privileged helper is the security boundary.** Any bug in helper command parsing is a host compromise vector. Mitigation: allow-list of (verb, unit) pairs; refuse on any deviation; unit tests on the parser.
- [RISK] **No structured rate-limiting on /events.** A single client can hold an SSE connection forever. Mitigation: per-IP connection cap = 3 (configured in `internal/server/sse.go`); not yet exposed as a config knob.
- [RISK] **Tailwind CSS purge happens at build time.** A class added in a template after a CSS build will not be present. Mitigation: `make` target depends on templates; CI fails on missing classes (style-lint guard pending — see BACKLOG.json).
