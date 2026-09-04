# Deployment — NUT_UPS_Pi_On_Dashboard

UPS monitoring for Ultron-AP. **Additive feature** on the existing native binary
deployment: a single statically-linked Go binary under systemd on the Raspberry
Pi. **Not containerized** (Docker deploy is in the project `no_go_zone`), so there
is no Dockerfile — it ships inside the existing `ultron-ap` binary.

## Deployment model
- **Native binary under systemd** on the Pi (linux/arm64), same unit as the rest
  of Ultron-AP (`deploy/ultron-ap.service`). This feature adds no new process,
  no new port, and no new privilege — it opens one localhost TCP connection to
  the already-running `upsd`.
- Config is read from `/etc/ultron-ap/ultron-ap.env` (the unit's `EnvironmentFile`).

## Prerequisites (on the Pi)
1. **NUT already running** with the UPS attached (this feature does not install or
   configure NUT — it only reads it). Verify:
   ```
   upsc powest        # should print ups.status, battery.voltage, etc.
   ```
2. **A dedicated read-only NUT user** for Ultron (RS-1 — do NOT reuse the
   homeassistant user). As root on the Pi, append to `/etc/nut/upsd.users`:
   ```
   [ultron]
       password = <a-long-random-password>
       # read-only: no "actions"/"instcmds" lines — Ultron never writes to the UPS
   ```
   Then reload upsd:
   ```
   sudo systemctl reload nut-server   # or: sudo upsd -c reload
   ```
   Confirm the credential works read-only:
   ```
   upsc powest@127.0.0.1 ups.status
   ```

## Configuration
Add the UPS keys to `/etc/ultron-ap/ultron-ap.env` (see `.env.example` in this
folder for the full list with defaults). Minimum:
```
ULTRON_UPS_ENABLED=true
ULTRON_NUT_USER=ultron
ULTRON_NUT_PASS=<the password you set in upsd.users>
```
Secrets: `ULTRON_NUT_PASS` lives only in this file (mode `0600`, root-owned) — never
in the repo, never in logs (verified by NFR-019). The systemd unit **must not** set
`ULTRON_UPS_MOCK`; with it unset the module only ever talks to the real read-only
NUT endpoint.

## Build & release (same as the parent project)
On the Mac (cross-compile for the Pi):
```
make build-arm        # produces the linux/arm64 binary (rebuilds CSS first)
```
Copy the binary to the Pi and restart the service. **The systemd unit runs
`/opt/ultron-ap/ultron-ap` (WorkingDirectory `/opt/ultron-ap`, User `ultron`) —
install to that exact path, not `/usr/local/bin`:**
```
scp bin/ultron-ap-linux-arm64 cesareyeserrano@<host>:/tmp/ultron-ap
ssh cesareyeserrano@<host> 'sudo install -m0755 /tmp/ultron-ap /opt/ultron-ap/ultron-ap && sudo systemctl restart ultron-ap && systemctl is-active ultron-ap'
```
Confirm the new build is live: `curl -s http://<host>:8080/version` should show the
commit you built (not the previous one).
No schema migration step is needed: the two new tables (`ups_samples`,
`ups_events`) are created by the existing `CREATE TABLE IF NOT EXISTS` path in
`internal/database/sqlite.go` on first boot. No existing table is altered.

## Local dev (no physical UPS, no deploy — NFR-022)
On the Mac, render the real card against mock data:
```
ULTRON_UPS_ENABLED=1 ULTRON_UPS_MOCK=OB make run     # fixed state: En batería
ULTRON_UPS_ENABLED=1 ULTRON_UPS_MOCK=1  make run     # cycles OL→OB→LB→unreachable
```
Mock values: `OL | OB | LB | RB | BYPASS | OFF | ALARM | unreachable | 1`.

## Health checks
- **App health:** unchanged — `GET /health` returns 200 when the process is alive
  (the UPS module never affects it; NFR-016).
- **UPS module health:** the dashboard UPS tile shows the live state; "Sin datos"
  (muted, dashed) means NUT is unreachable — the panel keeps working regardless.
- **Journal:** `journalctl -u ultron-ap -f | grep ups` shows startup
  (`UPS monitor started …`), outage lifecycle (`mains outage started` /
  `mains restored after …`), and `unreachable`/`reachable again` — all timestamped
  and rate-bounded (NFR-020).
- **DB spot-check:**
  ```
  sqlite3 /var/lib/ultron-ap/ultron.db "SELECT COUNT(*) FROM ups_samples;"
  sqlite3 /var/lib/ultron-ap/ultron.db "SELECT start_ts,end_ts,duration_s FROM ups_events ORDER BY id DESC LIMIT 5;"
  ```

## Rollback
The feature is gated and additive, so rollback is low-risk:
1. **Fast disable (no redeploy):** set `ULTRON_UPS_ENABLED=false` in
   `/etc/ultron-ap/ultron-ap.env` and `sudo systemctl restart ultron-ap`. The
   module goes fully inert; every other tile is unaffected. The `ups_samples` /
   `ups_events` tables remain (harmless) and resume on re-enable.
2. **Full binary rollback:** reinstall the previous `ultron-ap` binary and
   restart. The new tables are ignored by the old binary (`IF NOT EXISTS`), so
   no data migration or cleanup is required.
3. **Drop the data (optional):** `DROP TABLE ups_samples; DROP TABLE ups_events;`
   — only if you want to reclaim space; not needed for rollback.

## Verification after deploy
```
journalctl -u ultron-ap -n 20 | grep -i ups     # expect "UPS monitor started (…real NUT)"
curl -sf http://localhost:8080/health            # expect 200
```
Then open the dashboard — the UPS tile appears in the indicators row and the
"UPS · Powest" detail panel appears before Operational Indicators.

## CI
`.github/workflows/ci.yml` runs `go vet ./...` and `go test -race -count=1 ./...`
on every push / PR to `main`, which includes `./internal/ups/...` (NFR-021), plus
the linux/arm64 build. No new CI wiring is required.
