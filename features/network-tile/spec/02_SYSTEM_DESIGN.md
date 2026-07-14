# System Design — network-tile

Realises FR-085 (link-state verdict), FR-086 (throughput subtitle + collapsed detail), FR-087 (absorb the WAN chip).

## Executive Summary

This feature adds **two pure functions and one template rewrite**. Nothing else.

| Decision | Choice | Justification |
|---|---|---|
| Where the verdict is computed | A pure Go function over `DashboardData`, called from the template | Every input (`Network []*gatewayprobe.Snapshot`, `WAN *wanmonitor.Snapshot`, `Metrics.Networks`) **already reaches the partial** — `gatherDashboardData` populates all three (sse.go:590–597). No new field, no new SSE event, no new query, no goroutine. A pure function is also the whole testability story: given probes, assert a word. |
| Where the thresholds come from | The existing `latencyWarnRTTMs` / `latencyCritRTTMs` / `latencyCritLossPct` constants in `helpers_sparkline.go` | A second threshold set is a bug waiting to happen: the tile would say "Stable" while the sparkline three inches below it rendered red. One source, two consumers. |
| How "the internet is reachable" is decided | Structurally: the gateway is the probe with the routing-table-resolved host; **any** other probe answering means the internet is up | The name-matching approach is what caused BG-074 (a `case "cloudflare"` that no default target carries, so the flag was never set). Structure survives an operator renaming their targets; names do not. |
| Throughput headline | `max(sent+recv)` over the non-virtual interfaces — **never the sum** | `tailscale0` tunnels over `eth0`. Summing them double-counts the same bytes the moment the VPN carries traffic — silently inflating the number precisely when the admin would be looking at it. |
| Per-interface disclosure | Native `<details>`/`<summary>` | The pattern `sse-summary.html` already uses. No JS file, no widget, no client state, nothing to re-bind after an hx-boost swap. |
| Loss threshold for "Unstable" | `>= 10%` (the existing crit constant), **not** "> 0%" | The probe's loss window is 20 samples (`historyWindow`, gatewayprobe.go:58), so **one dropped ping = 5%**. A `>0%` rule would flap between Stable and Unstable on a single lost packet — the tile would cry wolf on a healthy home network. |

## System Architecture

```
   gatewayprobe.Probe          wanmonitor.Monitor         metrics.Collector
   (5s: RTT/jitter/loss        (up/down/unknown,          (per-interface
    /status per target)         3 failures → down)         bytes/s)
          │                            │                          │
          └────────────┬───────────────┴──────────────────────────┘
                       ▼
        gatherDashboardData()          internal/server/sse.go:540
        DashboardData{ Network[], WAN, Metrics.Networks }
                       │
                       │  (all three already arrive — no plumbing added)
                       ▼
   ┌───────────────────────────────────────────────────────────┐
   │ NEW, pure, no I/O:            internal/server/helpers.go   │
   │                                                            │
   │  dashboardLinkState(probes, wan) → LinkState               │
   │      { Verdict: "stable"|"unstable"|"offline"|"unknown",   │
   │        Reason:  "WAN up · 0% loss" | "15% loss · 8.8.8.8 ✕"│
   │        WorstLoss float64 }                                 │
   │                                                            │
   │  primaryNetwork(ifaces) → *metrics.NetworkIface            │
   │      max(sent+recv) over dashboardNetworks(ifaces)         │
   └───────────────────────────────────────────────────────────┘
                       │  registered in BOTH FuncMaps (templates.go)
                       ▼
        web/templates/partials/sse-metrics.html
        ┌──────────────────────────────┐
        │ Network                      │
        │ Stable            ← verdict  │  metric-warning / metric-critical
        │ eth0 · WAN up · 0% loss      │  ← primaryNetwork + reason
        │ ▸ per-interface   ← <details>│  ← collapsed; rows inside
        └──────────────────────────────┘
        (the standalone WAN chip below the grid is DELETED — FR-087)
```

## Data Model

### Preservation contract — MUST NOT change

- **No schema change. No new table, no new column, no migration.** This feature touches no database.
- `DashboardData` (sse.go:26–52) — no field added or removed. The verdict is derived at render from `Network`, `WAN` and `Metrics.Networks`, all already populated.
- The SSE payload — same events (`metrics`, `summary`, `charts`, `alert-count`, `verdicts`), same cadence, same partials. A client on the old build renders the new HTML fine because it is just HTML.
- `gatewayprobe.Snapshot` and `wanmonitor.Snapshot` — read-only consumers here; no field changes.
- `metrics.NetworkIface` — unchanged.

### Delta — one new value type (in-memory only)

```go
// LinkState is the tile's verdict. It exists only between the helper and the
// template; it is never persisted, never serialised, never sent over SSE.
type LinkState struct {
    Verdict   string  // "stable" | "unstable" | "offline" | "unknown"
    Reason    string  // human-readable: "WAN up · 0% loss" / "15% loss · 8.8.8.8 ✕"
    WorstLoss float64 // 0..100, the worst LossPct across probes
}
```

Verdict rules (in order — first match wins):

| # | Condition | Verdict | Why this order |
|---|---|---|---|
| 1 | No probes at all (`len(probes) == 0`) | `unknown` | Claiming a state we cannot know is worse than admitting we don't (AC-085-005). |
| 2 | `WAN.State == "down"` | `offline` | The WAN monitor is the authority on WAN reachability; it needs 3 consecutive failures to flip, so it does not flap. |
| 3 | The gateway probe's status is not `ok` | `offline` | A box that cannot reach its own gateway is not "unstable" — its LAN is broken. Different problem, different word (AC-085-004). |
| 4 | Any probe: `LossPct >= 10` **or** `RTTMs >= 200` **or** status not `ok` | `unstable` | The existing crit thresholds. A non-ok off-box target (8.8.8.8 timing out while the gateway answers) is exactly "unstable", not "offline". |
| 5 | otherwise | `stable` | — |

`Reason` is built from whichever condition matched: the failing target's label and its loss, or `WAN up · N% loss` when stable.

## API Design

### Contract being preserved

| Surface | Preserved |
|---|---|
| `GET /` (dashboard) | Same handler, same data, same SSE subscription. Only the HTML of one tile changes. |
| `GET /api/sse/dashboard` | Same events, same cadence, same payload. |
| `sse-charts.html` per-target sparklines | Untouched — still the drill-down, still reading history by label (BG-073). |
| `/network` | Untouched. |

### New internal API (unexported, no HTTP surface)

```go
// internal/server/helpers.go
func dashboardLinkState(probes []*gatewayprobe.Snapshot, wan *wanmonitor.Snapshot) LinkState
func primaryNetwork(ifaces []metrics.NetworkIface) *metrics.NetworkIface  // nil when none
```

Both registered as template funcs in **both** FuncMaps in `templates.go` (the file carries two — the SSE partial cache and the page cache; registering in one silently breaks the other path).

## Implementation Approach

### FR-085 — the verdict

- **Method.** `dashboardLinkState` walks the probe list once. It identifies the gateway probe by label (`gatewayProbeLabel`, the constant introduced by the BG-074 fix), tracks the worst loss and the worst offender, and applies the five rules in order. It reads `latencyCritLossPct` / `latencyCritRTTMs` from `helpers_sparkline.go` — no new constants.
- **I/O contract.** In: the probe snapshots and the WAN snapshot (both nil-safe). Out: a `LinkState`. No error return: a missing input yields `unknown`, which is a legitimate answer, not a failure.
- **Failure behaviour.** `probes == nil` or empty → `unknown` (the tile falls back to throughput only). `wan == nil` → the WAN clause is skipped; the probe rules still apply, and the subtitle omits the WAN phrase (AC-087-003). A nil element inside the slice is skipped rather than dereferenced.

### FR-086 — throughput subtitle and collapsed detail

- **Method.** `primaryNetwork` runs `dashboardNetworks` (the BG-072 virtual-interface filter) and returns the interface with the largest `BytesSentPS + BytesRecvPS`. The template renders its name and both rates in the subtitle, then emits a `<details>` whose `<summary>` is the toggle and whose body is one row per filtered interface.
- **I/O contract.** In: `Metrics.Networks`. Out: one interface (or nil → the subtitle degrades to the verdict reason alone).
- **Failure behaviour.** No interfaces at all → no subtitle throughput, no disclosure; the tile still shows its verdict. All interfaces virtual → the BG-072 fallback returns the unfiltered list, so an unusual host still sees its traffic instead of an empty tile.
- **Explicitly not done.** The rates are never summed. `tailscale0` carries the same bytes as `eth0` when the VPN is active; a "total" would double-count them exactly when the admin is watching.

### FR-087 — absorb the WAN chip

- **Method.** Delete the `{{if and .Network .WAN}}…{{end}}` chip block that sits below the metric grid (sse-metrics.html). Its state now appears inside the tile: `WAN up` in the subtitle when stable, and as the driver of the `Offline` verdict when down.
- **Failure behaviour.** With no WAN snapshot, nothing is claimed — the same silence the chip's `{{if}}` guard produced.

## Security Design

- **Attack surface: none added.** No new route, no new handler, no new parameter, no new privileged call, no new database access. The feature is a pure function plus template markup.
- **Rendering.** Interface names come from the kernel (`/proc/net/dev`) and probe labels from operator config — both rendered through `html/template`, which escapes them. The verdict word is a closed set of four constants chosen by the server, never client input.
- **No information disclosure.** The tile shows what the dashboard already showed (interface rates, WAN state, loss) — nothing that was not already on the page, and the whole page is behind `requireAuth`.
- **CSP.** No inline `<script>`; the disclosure is a native `<details>` element. The policy moves no further from being enforceable.

## Performance & Scalability

Target: Raspberry Pi, ARM64, limited RAM. The owner's standing constraint is that dashboard work stay cheap.

| Path | Cost |
|---|---|
| `dashboardLinkState` | One pass over ≤4 probe snapshots (the default target list). No allocation beyond the returned struct and one reason string. Pure CPU, microseconds. |
| `primaryNetwork` | One pass over the filtered interface list (≤5 real interfaces after the BG-072 filter, from ~15 raw on this Pi). |
| Per SSE tick | Both run once per rendered `sse-metrics` partial — the same tick that already renders CPU/RAM/Disk/Temp. **No new query, no new goroutine, no new I/O.** |
| Payload | The tile's HTML gets *smaller* on this Pi: 3 visible rows → 1 headline + 1 subtitle + a collapsed `<details>`. The rows still ship (inside the disclosure), so the byte delta is ≈ neutral; the visual delta is the point. |

## Deployment Architecture

**Model: native binary + systemd on the Pi** — unchanged (the project rejected containers at the Phase 5 gate on 2026-03-18).

- Ships inside the existing `ultron-ap` binary. `ultron-helper` is not touched; **no privilege change**.
- No schema migration, so **rollback is trivially safe**: the previous binary renders the previous tile from the same `DashboardData`.
- CSS: the tile reuses existing classes (`metric-tile`, `metric-warning`, `metric-critical`, `text-2xl`, `font-mono`, `text-xs`). `make css` must still run — if the `<details>` markup introduces a class Tailwind has not seen (e.g. a `marker:` variant), the committed `app.css` artifact would otherwise be stale in production, which is exactly how the hardware section nearly shipped unstyled.
- CI: the existing workflow. The new tests are pure-function tests — no browser, no sleeps, no flake.
- Verification on the Pi: the tile must read `Stable` with the real probes running, and the loose WAN chip must be gone.

## Risk Analysis

**ADR-1 — Reuse the sparkline thresholds instead of defining tile-specific ones.**
Two threshold sets on one page can disagree: the tile says "Stable" while the sparkline below it renders red. That contradiction is worse than either threshold being slightly off, and it is unfalsifiable to a user. One source (`helpers_sparkline.go`), two consumers.

**ADR-2 — "Unstable" at ≥10% loss, not at >0%.**
The probe's loss window is 20 samples, so a single dropped ping is 5%. A `>0%` rule would flip the tile to yellow on one lost packet on an otherwise healthy home link — the tile would cry wolf, the admin would learn to ignore it, and the feature would have made things worse than the byte counter it replaced.

**ADR-3 — Max interface, never the sum.**
`tailscale0` tunnels over `eth0`. A summed "total throughput" double-counts the same bytes whenever the VPN carries traffic — inflating the headline silently, precisely when the admin is looking. Max is boring and correct.

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| 1 | The verdict flaps (Stable ↔ Unstable) on a single lost packet, training the admin to ignore it. | high | ADR-2: the ≥10% crit threshold (2 of 20 samples). The WAN clause additionally inherits the monitor's 3-consecutive-failure hysteresis. |
| 2 | The tile disagrees with the sparklines below it. | medium | ADR-1: one constant set, two consumers. A test asserts a probe at 15% loss yields both `Unstable` on the tile and the warn colour on the sparkline. |
| 3 | "Offline" is shown when only one off-box target is timing out (crying wolf in the other direction). | medium | Rule order: `offline` requires the WAN monitor to be down or the **gateway** to fail. A single dead off-box target yields `unstable`, which is the honest word. |
| 4 | The throughput readout — the only one in the app — becomes unreachable. | medium | It stays on the tile, one click away, rather than being deleted or relocated. |
| 5 | Registering the template funcs in only one of the two FuncMaps (the file has two) silently breaks the SSE path or the page path. | medium | Both registration sites are named in the touch-points; a rendered-HTML test exercises the partial through `renderPartial`, which uses the SSE FuncMap. |
| 6 | The `<details>` marker or a new class is missing from the committed `app.css`. | low | `make css` runs before deploy and the diff is committed — the exact discipline that caught the hardware section's missing `grid-cols-4`. |

## Technical Risk Flags

[RISK] The verdict is a judgement the system makes on the admin's behalf
Conflict: FR-085 requires a single word where the truth is a distribution (four probes, each with RTT, jitter, loss, status). Any collapse loses information, and a wrong "Stable" is worse than no verdict at all — it actively misleads.
Mitigation: the collapse is conservative (the *worst* probe drives the verdict, never an average), the reason string always names what drove it, and the full per-target detail remains one screen down. The thresholds are the ones the product already trusts to colour its charts.
Severity: medium

[RISK] Probe labels are operator-configurable, and the gateway is identified by label
Conflict: `dashboardLinkState` finds the gateway probe by matching `gatewayProbeLabel` ("gateway"). An operator who renames their gateway target via `ULTRON_NET_TARGETS` would leave the verdict with no gateway clause — every probe would be treated as off-box, so a broken LAN would read `Unstable` instead of `Offline`.
Mitigation: accepted, and it is strictly better than the status quo (BG-074 was exactly this class of bug, and it silently produced a rule that could never fire). The degradation here is graceful: the tile still warns, just with a less precise word. A future refactor should carry `Host == ""` through to the snapshot so the gateway is identified structurally rather than by name — that is the real fix and it belongs in `gatewayprobe`, not here.
Severity: low

[RISK] SSE renders this tile on every tick; a panic in the helper would kill the dashboard for every client
Conflict: NFR-088b requires the dashboard keep refreshing. `dashboardLinkState` dereferences pointers from two sources that can legitimately be nil (`WAN` before the monitor's first observation; nil entries in the probe slice).
Mitigation: every dereference is guarded, nil inputs return `unknown` rather than panicking, and the nil-WAN and empty-probe cases are explicit test cases (AC-085-005, AC-087-003), not afterthoughts.
Severity: low
