# Network Monitoring — Deployment

## Deployment model

This feature is **not a standalone service**. It is a Go package set added under
`internal/network/*` of the existing Ultron-AP monolith. Its lifecycle is the
parent's lifecycle: same binary, same `systemd` unit, same SQLite file, same
HTTP server, same authentication and CSRF middleware, same privileged helper.

There is no Dockerfile, no `docker-compose.yml`, and no `.env.example` for
this feature. The parent project (`/opt/ultron-ap` on the Pi) defines those
surfaces — see `deploy/ultron-ap.service`, `deploy/ultron-helper.service`, and
`deploy/ultron-ap.sudoers` at the repo root. A previous Phase 5 attempt added
container artefacts and was rejected (see `01_REQUIREMENTS.json` rejection
history): the deployment target is **Raspberry Pi via systemd**, not Docker.

## Prerequisites (host)

- Raspberry Pi running the existing Ultron-AP installation (`raspberry-stable`
  baseline under `deploy/`). Pi 4/5 with arm64.
- `go 1.22+` available on the build host (parent toolchain). Cross-compilation
  from x86 → linux/arm64 supported.
- `librespeed-cli` installed at `/usr/local/bin/librespeed-cli` **only if
  FR-024 on-demand bandwidth tests are needed**. Off by default; allow-listed
  in the helper. Install via `INSTALL_SPEEDTEST=1` flag in the parent's
  installer.
- `iw` available on the host (already shipped with most Pi OS images) for the
  WiFi panel (FR-028) — used through the helper allow-list.

## Database migrations

The feature introduces new `net_*` tables. Migrations live under the parent's
`internal/database/migrations/` directory. Apply them with the parent's
existing migration runner — no new tooling.

The canonical list of tables that must be included in the FR-015 backup is
returned by `internal/network/store.BackupTables()`. The backup runner reads
this contract; any new `net_*` table added in the future MUST be added to that
list in the same change that creates it.

## Build & deploy

Cross-compile the parent binary (which now includes `internal/network/*`),
copy to the Pi, restart the unit:

```sh
# On the build host (from repo root):
GOOS=linux GOARCH=arm64 go build -o build/ultron-ap-arm64 ./cmd/ultron-ap

# Verify Go tests on the build host before shipping:
go test ./...
# (CI/CD runs this on every push/PR via .github/workflows/security-gate.yml,
#  plus govulncheck — covers NFR-010.)

# Copy and restart on the Pi:
scp build/ultron-ap-arm64 pi@ultron:/tmp/ultron-ap-new
ssh pi@ultron 'sudo install -m0755 /tmp/ultron-ap-new /opt/ultron-ap/ultron-ap \
  && sudo systemctl restart ultron-ap.service'
```

The helper unit (`ultron-helper.service`) does not need to be rebuilt or
restarted unless the helper allow-list grammar changed (it did — see
`internal/network/helper/allowlist.go`; restart helper if upgrading from a
build that predates this feature).

## Health checks

Inherits the parent's `/health` endpoint. The parent reports overall
healthiness; the network feature exposes its collector status separately at
`/api/network/health`, returning `{collector: "ok"|"degraded"|"down",
last_sample_ts}`. UI banners surface a "collector down" state when this
endpoint reports `down` (TC-NM-004f, currently manual).

Recommended post-deploy smoke check:

```sh
ssh pi@ultron 'systemctl is-active ultron-ap ultron-helper && \
  curl -fsS http://127.0.0.1:8080/health'
```

A clean restart should produce `active` for both units and HTTP 200 for
`/health` within ~5 seconds.

## Rollback

The parent's `systemd` unit is the rollback boundary. Two paths:

1. **Binary rollback** (preferred): keep the previous binary as
   `/opt/ultron-ap/ultron-ap.previous` during deploy; on regression, swap and
   restart:

   ```sh
   ssh pi@ultron 'sudo mv /opt/ultron-ap/ultron-ap /opt/ultron-ap/ultron-ap.bad \
     && sudo mv /opt/ultron-ap/ultron-ap.previous /opt/ultron-ap/ultron-ap \
     && sudo systemctl restart ultron-ap.service'
   ```

2. **Schema rollback**: the `net_*` tables are additive — no parent table is
   altered. Rolling back the binary leaves the new tables in place; they are
   read-only from the older binary's perspective. No destructive migration
   step is required during rollback. If the new tables must also be dropped
   (e.g. corruption suspected), restore from the FR-015 encrypted backup
   captured immediately before the deploy.

## Environment variables

This feature introduces no new environment variables. All configuration —
default targets, cadences, retention, budgets, alert rules — lives in the
SQLite tables (`net_targets`, `net_settings`, `net_alert_rules`) and is
manipulated through the existing `/network/settings` UI. The parent's
`EnvironmentFile=-/etc/ultron-ap/ultron-ap.env` setting is unchanged.

Optional install-time toggles (read by the parent's installer, not by this
binary):

- `INSTALL_SPEEDTEST=1` — install `librespeed-cli` on the Pi (FR-024).

## Observability

Every probe emits a single structured log line per cycle to stdout (NFR-008):

```
[ts] PROBE target=<host> kind=icmp|dns|st status=ok|fail|timeout rtt_ms=<n>
```

The parent's `journalctl -u ultron-ap` is the canonical log source. Log level
is configurable via the existing parent setting (info default; warn suppresses
PROBE info lines but keeps failures — TC-NM-025f).

## Known limitations (Phase 4 vertical slice)

The implementation manifest declares technical debt for most FRs — the worker
goroutines and HTTP handlers are skeletons that return `ErrSkeleton` /
`501 Not Implemented`. The pure-logic primitives (validators, allow-list,
grading, run-guard, parser, args builder) are in place and pinned by tests.
Subsequent iterations will land the workers, the HTTP handlers, and the UI
templates. Production deployment of this binary therefore exposes the routes
but they are not yet user-facing functional — the parent's existing routes
continue to work unaffected because `internal/network/api.Register` only adds
new routes under `/network/*` and `/api/network/*`, none of which collide
with the parent's mux.
