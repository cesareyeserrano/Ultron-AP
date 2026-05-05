# LAN Devices — Deployment

## Deployment model

This feature is **not a standalone service**. It is a Go package set added
under `internal/network/landevices/*` plus three HTTP handlers in
`internal/server/` of the existing Ultron-AP monolith. Its lifecycle is the
parent's lifecycle: same binary, same `systemd` unit, same SQLite file, same
HTTP server, same auth middleware, same backup pipeline.

There is no Dockerfile, no `docker-compose.yml`, and no `.env.example` for
this feature. The parent project (`/opt/ultron-ap` on the Pi) defines those
surfaces — see `deploy/ultron-ap.service`, `deploy/ultron-helper.service`, and
`deploy/ultron-ap.sudoers` at the repo root. A previous project-level Phase 5
attempt added container artefacts and was rejected (see project
`spec/01_REQUIREMENTS.json` rejection history, 2026-03-18): the deployment
target is **Raspberry Pi via systemd**, not Docker.

## Prerequisites (host)

- Raspberry Pi running the existing Ultron-AP installation
  (`deploy/raspberry-stable` baseline). Pi 4/5 with arm64.
- `go 1.22+` available on the build host. Cross-compilation from x86 →
  linux/arm64 supported via `make build-arm` (parent Makefile).
- `net.ipv4.ping_group_range` already permits the unprivileged Ultron user —
  same kernel sysctl already used by `gatewayprobe`. No new kernel
  configuration required by this feature.
- `/proc/net/arp` and `/proc/net/route` must be readable by the unprivileged
  process — true on standard Linux kernels including Raspberry Pi OS.
- No new privileged-helper endpoints; no new sudoers rule; the helper unit is
  unaffected (NFR-011).

## Database migrations

This feature adds one new table — `lan_devices` — and one supporting index.
The schema is appended to the parent's `schema` constant in
`internal/database/sqlite.go`, so the existing `database.New()` schema-init
path picks it up automatically on first start after deploy.

```sql
CREATE TABLE IF NOT EXISTS lan_devices (
  mac            TEXT    PRIMARY KEY,
  ip             TEXT    NOT NULL,
  vendor         TEXT    NOT NULL DEFAULT 'Unknown',
  first_seen     INTEGER NOT NULL,
  last_seen      INTEGER NOT NULL,
  online         INTEGER NOT NULL DEFAULT 1,
  missed_sweeps  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_lan_devices_online_lastseen
  ON lan_devices(online DESC, last_seen DESC);
```

The table is captured by the parent's existing `Backup()` (`VACUUM INTO`)
pipeline — no manual configuration needed (NFR-013).

## Build & deploy

Cross-compile the parent binary (which now includes the lan-devices package
set), copy to the Pi, restart the unit:

```sh
# On the build host (from repo root):
make build-arm                                  # produces bin/ultron-ap-linux-arm64

# Run the Go test suite before shipping:
go test ./...

# Copy and restart on the Pi:
scp bin/ultron-ap-linux-arm64 pi@ultron:/tmp/ultron-ap-new
ssh pi@ultron 'sudo install -m0755 /tmp/ultron-ap-new /opt/ultron-ap/ultron-ap \
  && sudo systemctl restart ultron-ap.service'
```

The helper unit (`ultron-helper.service`) does NOT need to be rebuilt or
restarted — this feature does not touch the helper allow-list grammar.

## Configuration

This feature introduces **no new environment variables in v1**. Defaults
(per `internal/network/landevices/orchestrator.go`):

| Setting          | Default | Range allowed |
|------------------|---------|---------------|
| Sweep cadence    | 5 min   | 1–30 min      |
| Miss threshold N | 3       | 1–10          |
| ICMP workers     | 32      | (fixed)       |
| Per-host timeout | 1 s     | (fixed)       |

Future configurability (e.g. via a `lan_devices_config` row) is out of scope
for v1.

## Health checks

Inherits the parent's `/health` endpoint. The lan-devices feature also
exposes its own runtime status at `GET /api/network/lan-devices/status`:

```json
{
  "subnet": "192.168.1.0/24",
  "interface": "eth0",
  "subnet_status": "ok",
  "last_sweep_at": "2026-05-05T20:30:00Z",
  "last_sweep_duration_ms": 2143,
  "last_sweep_responders": 27,
  "overrun_count": 0,
  "self_throttled": false,
  "current_cadence_ms": 300000,
  "device_count": 27,
  "disabled": false
}
```

Recommended post-deploy smoke check:

```sh
ssh pi@ultron 'systemctl is-active ultron-ap \
  && curl -fsS http://127.0.0.1:8080/health \
  && curl -fsS -b "session=$ULTRON_SESSION" http://127.0.0.1:8080/api/network/lan-devices/status'
```

A populated `device_count` after one sweep cadence (≤5 min on default
settings) confirms the orchestrator is sweeping; `self_throttled=false`
confirms the resource-budget breaker (FR-038) is not engaged.

## Capability check (NFR-011)

The deployed binary must NOT acquire any new Linux capability beyond what
the parent project sets. Verify on the Pi:

```sh
ssh pi@ultron 'getcap /opt/ultron-ap/ultron-ap'
# Expected: empty (no capabilities required) — same as before this feature.
```

If `getcap` shows `cap_net_raw` or anything new, the build was misconfigured
and lan-devices' unprivileged-ICMP contract is broken. Roll back.

## Rollback

The lan-devices feature is forward-compatible only at the schema level —
i.e. an older binary will simply ignore the `lan_devices` table that the
new binary created. No reverse migration needed.

```sh
# Restore previous binary (kept by the install step's backup convention):
ssh pi@ultron 'sudo systemctl stop ultron-ap.service \
  && sudo install -m0755 /opt/ultron-ap/ultron-ap.previous /opt/ultron-ap/ultron-ap \
  && sudo systemctl start ultron-ap.service'
```

The `lan_devices` table will remain in the SQLite file but is harmless —
the older binary does not read or write it.

## Failure modes & runbook

| Symptom                                          | Likely cause                                           | Remedy |
|--------------------------------------------------|--------------------------------------------------------|--------|
| `subnet_status: "no-default-route"` after deploy | Pi has no default IPv4 route at boot                   | Wait for network. Module idles until route appears (FR-030 AC-002). |
| `last_sweep_responders` always 0                 | `net.ipv4.ping_group_range` regressed on this kernel   | Check `sysctl net.ipv4.ping_group_range`; lan-devices fails closed (NFR-011 AC-002). Re-apply the parent's sysctl drop-in. |
| All vendors render as `Unknown`                  | OUI embed corrupted / missing CSV at build time        | Rebuild from clean checkout — `make build-arm` re-embeds the table. Spot-check unit test `TestTC_LD_004h_Vendor_RaspberryPi`. |
| `self_throttled: true` persistently              | Sweep wall-clock >3 s for 2+ cycles, FR-038 engaged    | Investigate IO load on Pi. Cadence auto-restores after 30 min in-budget (FR-038 AC-002). |
| `/proc/net/arp` permission denied                | Hardened kernel removed world-readable `/proc/net/arp` | Devices appear with `mac=null` and vendor `Unknown` per FR-032 AC-003. Restore world-read on `/proc/net/arp` if vendor identification matters. |

## CI/CD

There is no CI/CD NFR explicitly declared by this feature; the parent
project's existing `.github/workflows/security-gate.yml` runs `go test ./...`
and `govulncheck` on every push/PR — both apply to this feature unchanged.
