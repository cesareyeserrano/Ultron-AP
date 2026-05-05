# 02_SYSTEM_DESIGN — LAN Devices (feature)

## Executive Summary

`lan-devices` is added as a single new internal Go package (`internal/network/landevices`) inside the existing Ultron-AP monolith — no new process, no new dependency on apt, no new datastore, no new privileged-helper endpoint. It runs one orchestrator goroutine that schedules a periodic ICMP sweep over the local /24, reads `/proc/net/arp` after each sweep, resolves vendors against an embedded IEEE OUI table, and upserts results into a single new SQLite table (`lan_devices`). Reads happen via two HTTP handlers (`GET /api/network/lan-devices` JSON + the existing `/network` HTML page extended with a HTMX-polled fragment).

Design priorities, in order:

1. **Stay unprivileged.** Reuse the same `pro-bing`-via-`ping_group_range` path already used by `gatewayprobe`. No `cap_net_raw`, no `setcap`, no helper extension.
2. **Match parent constraints.** Single Go binary, single SQLite file, FR-007 auth, FR-009 tokens, FR-015 backup pipeline. No new runtime dependencies.
3. **Bounded blast radius.** Sweep self-throttles (FR-038); even if a future kernel breaks `/proc/net/arp`, the feature degrades to ICMP-only (mac=null) without taking down the rest of the panel.
4. **Smallest possible surface.** One new table, two new endpoints, one new orchestrator. No background DB writers; the upsert volume (≤100 rows per sweep at default cadence) is well below SQLite's serialized-write contention threshold once BG-017's DSN pragmas are in place.

## System Architecture

```mermaid
graph TD
  user["Raspberry Pi Operator (Browser)"] -->|HTMX poll 30s| web["Ultron-AP Web App\n(Go HTTP server)"]
  web -->|GET /api/network/lan-devices| api["internal/network/landevices/api"]
  api --> store["internal/network/landevices/store\n(LanDevice CRUD)"]
  store --> db[("SQLite\n(parent file + lan_devices table)")]

  orch["internal/network/landevices.Orchestrator\n(single goroutine, 5-min ticker)"]
  orch -->|every cycle| subnet["subnet detector\n(/proc/net/route)"]
  orch -->|every cycle| sweep["ICMP sweep\n(pro-bing, 32-worker pool)"]
  orch -->|after sweep| arp["ARP-cache reader\n(/proc/net/arp parser)"]
  orch -->|per MAC| oui["OUI lookup\n(go:embed in-memory map)"]
  orch -->|upsert per device| store
  orch -.->|status flag| api

  main["cmd/ultron-ap/main.go"] -->|landevices.Start(ctx, db)| orch
```

Component responsibilities:

| Component | Path | Responsibility |
|---|---|---|
| `landevices` (root) | `internal/network/landevices/` | Public API: `Start(ctx, db) (*Orchestrator, error)`, `Stop()`, `Status() Status`. Single orchestrator goroutine. |
| `landevices/subnet` | | Resolve the `/24` to sweep from `/proc/net/route` + interface IP. Refresh every 10 min. |
| `landevices/sweep` | | Bounded-pool ICMP sweep using `pro-bing` (same lib + same `ping_group_range` path as `gatewayprobe`). Returns the set of IPs that replied within the per-host timeout. |
| `landevices/arp` | | Parse `/proc/net/arp` and return `map[ip]mac` for entries with flag 0x2. |
| `landevices/oui` | | `go:embed` the trimmed OUI prefix table; expose `Vendor(mac) string`. Handle the LAA bit. |
| `landevices/store` | | All `lan_devices` SQLite reads/writes. Single struct, no globals. |
| `landevices/api` | | HTTP handlers: `GET /api/network/lan-devices` (JSON), and the HTMX fragment for `/network`. Authenticated via parent FR-007 middleware. |
| `cmd/ultron-ap/main.go` | (existing) | Add a `landevices.Start()` call after `gatewayprobe.Start()`. |
| `web/templates/network.html` | (existing) | Add a `lan-devices` section above the WAN-events block; HTMX `hx-get` polls every 30 s. |

No parent package is forked. `gatewayprobe` is unaffected.

## Data Model

All persistence lives in the existing SQLite file. One new table.

```sql
CREATE TABLE IF NOT EXISTS lan_devices (
  mac            TEXT    PRIMARY KEY,            -- canonical lower-hex, colon-separated
  ip             TEXT    NOT NULL,               -- last-known IPv4
  vendor         TEXT    NOT NULL DEFAULT 'Unknown',
  first_seen     INTEGER NOT NULL,               -- ms epoch UTC
  last_seen      INTEGER NOT NULL,               -- ms epoch UTC of last successful sweep
  online         INTEGER NOT NULL DEFAULT 1,     -- 0/1
  missed_sweeps  INTEGER NOT NULL DEFAULT 0      -- consecutive missed sweep count
);
CREATE INDEX IF NOT EXISTS idx_lan_devices_online_lastseen
  ON lan_devices(online DESC, last_seen DESC);
```

Notes:
- `mac` is the natural primary key — DHCP renewals change `ip` but not `mac`. This satisfies FR-034 AC-002 directly.
- No history table in v1 (FR-034 explicit). When BL-017 (insights-engine) needs a sample history, a sibling `lan_device_samples` table can be added without migrating `lan_devices`.
- The index supports the API's "online-first then last_seen desc" ordering (FR-036 AC-001) without an `ORDER BY` table scan.
- Schema is appended to the parent's `schema` const in `internal/database/sqlite.go` so the existing `New()` migration path picks it up — same convention as `NetSample` / `NetEvent`.

## API Design

Two surfaces, both authenticated via the existing parent middleware.

### `GET /api/network/lan-devices`
- **Auth:** session cookie required (FR-007). No CSRF on GET.
- **Response 200:** JSON array, online entries first, then offline by `last_seen desc`:
  ```json
  [
    {
      "ip": "192.168.1.42",
      "mac": "b8:27:eb:11:22:33",
      "vendor": "Raspberry Pi Foundation",
      "online": true,
      "first_seen": "2026-04-30T11:02:14Z",
      "last_seen":  "2026-05-05T17:32:08Z",
      "missed_sweeps": 0
    }
  ]
  ```
- **Response 401:** unauthenticated (parent-standard auth-redirect for HTML, JSON 401 for `Accept: application/json`).
- **Status side-channel:** the array is wrapped neither in an envelope nor a metadata field. A separate `GET /api/network/lan-devices/status` returns:
  ```json
  { "subnet": "192.168.1.0/24", "interface": "eth0",
    "last_sweep_at": "...", "last_sweep_duration_ms": 2143,
    "self_throttled": false, "device_count": 27 }
  ```

### `GET /network` (HTMX fragment)
- The existing `/network` page template gains a `<section id="lan-devices-section">` placeholder that issues `hx-get="/network/lan-devices/fragment" hx-trigger="load, every 30s"`.
- The fragment endpoint renders a server-rendered HTML table snippet with columns IP / MAC / Vendor / Online / Last seen, using the parent's existing Tailwind tokens. Empty state is a single centred `<p>` per FR-037 AC-005.

### Wire-up
- Routes registered in `internal/server/server.go` next to existing `/api/network/*` handlers (network-monitoring feature). Both routes go through the existing `authMiddleware` chain.

## Security Design

- **Auth:** All new endpoints inherit FR-007 auth. No anonymous read access.
- **CSRF:** GET-only endpoints; no CSRF (matches parent convention).
- **Privilege:** No new privileged path. ICMP via unprivileged `ping_group_range`. `/proc/net/arp` is world-readable on standard kernels. `getcap` on the deployed binary returns the same set as before the feature.
- **Input boundary:** the API exposes server-derived data only. No user input is reflected; no path parameters; no query parameters in v1. XSS surface limited to the vendor string — the OUI table is checked into the repo and its source (IEEE registry) is auditable.
- **MAC privacy:** MACs are stored unhashed because they are the only stable device identifier. Backups (FR-015) are already encrypted via `ULTRON_BACKUP_KEY` (BL-007 lineage), so the at-rest exposure of MACs matches the existing notification-secret threat model.
- **Rate limit:** the API is protected by the parent's per-IP SSE/throttling, but a `/api/network/lan-devices` flood is bounded by SQLite read latency on a 100-row table — not exploitable.

## Performance & Scalability

| Resource | Budget | How met |
|---|---|---|
| CPU 5-min avg | ≤2% | Sweep is 250 ICMP packets in ~2-3 s every 5 min. Worker pool size 32 keeps peak concurrency bounded. The orchestrator sleeps the rest of the cycle. |
| RSS | ≤+20 MB | OUI map: ~5 KB compressed prefix → vendor strings (compiled from a trimmed IEEE list, target ~50k entries → ~1 MB in memory). Goroutine + buffers: <1 MB. |
| DB growth | ≤50 MB / yr | One row per distinct MAC ever seen. A 30-device LAN with full DHCP churn over a year stays well under 1 MB; the budget covers extreme cases (school dorm, café). |
| Sweep wall-clock | ≤3 s / cycle | 32-worker pool × 1-s per-host timeout × 8 batches = 8 s worst-case unreachable, but typical reply latency on a /24 LAN is <10 ms so a populated LAN finishes in well under 3 s. |
| API response time | ≤100 ms p99 for ≤100 devices | Single indexed read; indexed sort by `(online DESC, last_seen DESC)`. |

**Self-throttling (FR-038).** The orchestrator measures wall-clock per cycle and reads the parent's CPU collector for the 5-min average. Two consecutive over-budget cycles → cadence × 2 (capped at 1800 s). 30 min of in-budget cycles → restore.

**Scalability ceiling (out of scope but documented).** A /16 LAN (65k hosts) is rejected by the subnet detector via FR-030 AC-004 (clamp to /24). Beyond that, the scheduler is single-cycle so adding workers won't help; multi-subnet support is a v2 design change (per-subnet orchestrator).

## Deployment Architecture

- **Binary:** the existing `ultron-ap` binary gains the new package. No new artifact.
- **Service unit:** `deploy/ultron-ap.service` is unchanged. No new env vars in v1 (cadence and miss-threshold defaults are hard-coded; configurability comes via DB-backed `lan_devices_config` if a follow-up needs it — out of scope).
- **Migration:** the new `lan_devices` table is created by the existing `sqlite.New()` schema-init path. No data migration; on first deploy after upgrade, the table appears empty and populates from sweep #1.
- **Backup:** captured automatically by the existing `Backup()` (`VACUUM INTO`). No new file paths.
- **Rollback:** dropping back to a pre-feature binary leaves the table behind (harmless — old binary ignores it). No reverse migration needed.
- **Pi target:** ARM64 binary built via `make build-arm`, copied to `/opt/ultron-ap/ultron-ap`, restart `ultron-ap` systemd unit. Same flow used for BG-017 / BL-007.

## Risk Analysis

**Failure blast radius.**

| Component fails | What breaks | What survives | Mitigation |
|---|---|---|---|
| ICMP sweep cannot send (kernel changes `ping_group_range` semantics) | `lan_devices` stops updating; rows go offline after N misses | All other monitoring (gatewayprobe, alerts, dashboards) | Single log line at startup; FR-031 AC-002 covers the test. Operator notices via the offline state. |
| `/proc/net/arp` unreadable | Devices appear with `mac=null`, vendor `Unknown` | ICMP sweep continues to mark online state by IP | FR-032 AC-003 — degraded mode. |
| OUI embed corrupted at build | Vendor column shows `Unknown` for everything | Sweep + persistence + state machine all work | A unit test on the OUI map asserts `B8:27:EB → contains "Raspberry Pi"`. |
| SQLite write contention | Sweep cycle blocks for up to 5 s on `busy_timeout` (BG-017 fix) | Reads (API) keep going under WAL | DSN pragmas already applied; sweep uses single-row UPSERT, not bulk. |
| Sweep takes longer than cadence | Cycle is skipped (FR-031 AC-003), no overlap | Next cycle proceeds at scheduled time | Already covered by FR-031 + FR-038 combo. |

**Top risks.**
1. **OUI table staleness.** The bundled IEEE list goes out of date over months. *Mitigation:* document the refresh script in `tools/oui/`; CI / cron is out of scope for v1 — operator-driven refresh on rebuild is enough.
2. **DHCP renewals → temporary duplicate rows.** If a device gets a new IP between sweeps without us seeing the old MAC, the row updates atomically (mac is PK). No duplicate. *Confirmed:* the schema makes the failure mode impossible.
3. **MAC-randomising mobile clients (iOS 14+, Android 10+).** A phone may appear under multiple MACs over weeks. The device list grows without ever being pruned in v1. *Mitigation:* documented as known limitation; a future "prune offline >30d" job is one row deletion away if it becomes a real problem. Out of scope for v1.

## Technical Risk Flags

- **[RISK] DHCP-MAC churn** — *severity: low.* MAC randomisation could grow the table unboundedly over months. Bound is naturally low (LAN size × randomisation rate); a 30d-offline prune is a 5-line follow-up. Accept for v1.
- **[RISK] OUI table aging** — *severity: low.* Vendor column becomes "Unknown" for new vendors over time. Acceptable degradation; no functional impact on discovery / online state. Refresh on rebuild.
- **[RISK] `pro-bing` already used as `gatewayprobe`** — *severity: low.* Both subsystems share the same kernel ICMP socket family; under burst conditions, an in-flight gateway probe could share-of-loss with a sweep packet. *Mitigation:* the kernel queue is per-socket; pro-bing opens distinct sockets per `Pinger`. Confirmed by the network-monitoring feature's existing concurrent use. Accept.

No critical or high-severity technical risks identified. The feature reuses every parent contract; the only new external dependency (the OUI list) ships as data in the binary, not as a runtime dependency.
