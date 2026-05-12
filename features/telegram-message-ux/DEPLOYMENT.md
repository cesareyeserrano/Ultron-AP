# Deployment — telegram-message-ux

This feature is a **server-side refactor of the alert-notification rendering layer**. It ships as additional Go source files inside the existing single static binary that already runs as a systemd unit on the Raspberry Pi.

Per the project's Phase-5 rejection history (2026-03-18) and the `settings-revamp` / `help-page` / `insights-engine` / `lan-devices` precedents, **the deployment target is Raspberry Pi via systemd — not Docker**. No Dockerfile or docker-compose.yml is produced for this feature.

---

## Prerequisites

- Raspberry Pi 5 (linux/arm64) running Raspberry Pi OS Bookworm with the existing Ultron-AP install.
- Go 1.25+ on the build host (for cross-compilation via `make build-arm`).
- Existing systemd units installed: `ultron-ap.service` (unprivileged web) and `ultron-ap-helper.service` (privileged helper) — see the parent project's `deploy/` directory.
- Existing SQLite database at `/var/lib/ultron-ap/ultron-ap.db`. **No schema migration is required** — this feature does not add or alter any SQLite table.
- Telegram bot token + chat ID configured in the settings page (FR-005, unchanged).
- (Optional) SMTP credentials configured in the settings page (FR-006, unchanged).
- (Optional) `ULTRON_PUBLIC_URL` env var set in `/etc/ultron-ap/ultron-ap.env` — controls the deep-link footer URL. Falls back to `http://<configured-host>:<configured-port>` when unset (FR-023).

## Environment variables

| Name | Type | Required | Example | Purpose |
|---|---|---:|---|---|
| `ULTRON_PUBLIC_URL` | URL string | No | `https://ultron.example.com` | Base URL for the `[Open dashboard](.../alerts)` footer in Telegram and email alerts. |
| `TELEGRAM_BOT_TOKEN` | Secret string | Existing FR-005 config | `123456:REDACTED` | Existing Telegram bot credential. Stored through the settings flow, unchanged by this feature. |
| `TELEGRAM_CHAT_ID` | String/integer | Existing FR-005 config | `123456789` | Existing Telegram chat target. Stored through the settings flow, unchanged by this feature. |
| `SMTP_*` | Existing SMTP settings | Existing FR-006 config | `SMTP_HOST=smtp.example.com` | Existing email notifier configuration. Unchanged by this feature. |

## Dev setup

```bash
# Build for local dev (host arch).
make build

# Run the full test suite the feature ships with.
go test ./internal/notify/... ./internal/server/... ./internal/alerts/... -v

# Run only the FR-coverage tests (TC-TMU-* prefixed).
go test ./internal/notify/... ./internal/server/... -run TestTC_TMU -v
```

## Production deploy

This feature has zero new infrastructure surface. The standard build & deploy flow is unchanged:

```bash
# 1. Cross-compile for the Pi.
make build-arm

# 2. Copy the binary onto the Pi.
scp bin/ultron-ap pi@192.168.1.29:/tmp/ultron-ap.new

# 3. On the Pi: stage, restart, verify.
ssh pi@192.168.1.29 'sudo install -m0755 /tmp/ultron-ap.new /usr/local/bin/ultron-ap \
   && sudo systemctl restart ultron-ap.service \
   && systemctl status --no-pager ultron-ap.service \
   && curl -sf http://localhost:8080/health'
```

(Optional, when changing the deep-link footer base URL.)

```bash
ssh pi@192.168.1.29 'sudo sh -c "echo ULTRON_PUBLIC_URL=https://ultron.example.com >> /etc/ultron-ap/ultron-ap.env" \
   && sudo systemctl restart ultron-ap.service'
```

Smoke test on the Pi (the Test Telegram button preview, FR-026):

```
1. Open https://ultron.example.com/settings (or the LAN URL).
2. Confirm Telegram bot token + chat ID are configured.
3. Click "Test" next to the Telegram channel.
4. Within ≤3s, a Telegram message arrives whose first line begins
   with "TEST — " and which contains:
       - 🔴 severity glyph
       - "CPU usage critical on <host>" subject
       - "ALERT FIRED — CPU 92% (threshold > 80%) for ~1m 30s"
       - "[Open dashboard](<base-url>/alerts)" footer link
```

## Health check

The existing `GET /health` endpoint is unchanged. There is no new health surface — the feature does not add a new HTTP endpoint or background daemon. Per NFR-010, healthcheck applicability is "not applicable in the new sense".

```bash
curl -sf http://localhost:8080/health
# expected: 200 OK
```

## Rollback

This feature is a pure refactor with no schema changes. Rollback is the same as any other Ultron-AP rollback:

```bash
# 1. Reinstall the previous binary (kept by the deploy scripts in /usr/local/bin/ultron-ap.bak).
ssh pi@192.168.1.29 'sudo install -m0755 /usr/local/bin/ultron-ap.bak /usr/local/bin/ultron-ap \
   && sudo systemctl restart ultron-ap.service'

# 2. Verify.
ssh pi@192.168.1.29 'curl -sf http://localhost:8080/health \
   && systemctl status --no-pager ultron-ap.service'
```

No SQLite restore is needed — the storm cache and first-fired-at map are in-memory only. The first alert after a rollback simply starts a fresh storm window.

## What changed (for the operator's mental model)

- **Telegram messages now use MarkdownV2** instead of plain Markdown. Existing chats keep working; older messages are not retroactively rewritten.
- **Same-rule fires within a 60-second window collapse into a single chat row** that updates in place (storm protection, FR-024). The lock-screen does NOT re-buzz on edits.
- **Resolve messages have a distinct template** (✓, "RESOLVED", duration the alert was active). Resolves are emitted by future engine code paths; the rendering path is in place.
- **Email body is now multipart/alternative** (HTML + plain-text). Mail clients that strip HTML still see a clean plain-text version. Subject and recipient logic unchanged.
- **Deep-link footer**: every message ends with `[Open dashboard](<base>/alerts)`. The base URL is `ULTRON_PUBLIC_URL` when set; otherwise derived from the configured host/port.

## Known limitations (technical debt — see `spec/04_IMPLEMENTATION_MANIFEST.json`)

- Resource trend hint, systemd journal tail, and docker log tail are not yet auto-populated by the dispatcher (FR-022 / FR-020 / FR-021). The renderer fully supports them; production messages currently render the rule-context blocks (subject + threshold + footer + timestamp) without surface-specific bodies. Filling these in is a small dispatcher adapter — declared in technical_debt.
- Resolve event emission is not yet wired in the alert engine (FR-018 — explicit no_go_zone item from Phase 1). Renderer support is in place for when that work lands.
- ProcFSReader.TopProcesses does not compute live CPU% (returns Comm + RSS only). The probable-cause line for CPU alerts will show "top: <comm> (0%)" until the metrics-collector adapter lands. Memory alerts and exit-code-mapped Docker fires already produce correct output.
