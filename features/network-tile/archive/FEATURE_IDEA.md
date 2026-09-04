## Feature
Turn the dashboard's Network tile into a link-state indicator — stable / unstable / offline — with the throughput as secondary context and the per-interface breakdown collapsed behind one click.

## Problem / Why
Every other tile in that row answers a question at a glance: CPU 22.2%, Temp 51°C, Memory 45.7%. Network answers nothing — it lists rows:

```
Network
eth0        ↑ 7.2 KB/s  ↓ 54.7 KB/s
wlan0       ↑ 0 B/s     ↓ 0 B/s
tailscale0  ↑ 0 B/s     ↓ 0 B/s
```

The owner's words on 2026-07-14, looking at production: *"es confuso para un elemento principal; si requiere tanto detalle, no estaría bueno ponerlo allí. O cambiarlo por un indicador general, o un estable/no-estable."* Asked whether he ever reads the KB/s to decide something, he answered: **"casi nunca lo miro"**.

Two things make this worth fixing rather than papering over:

1. **The tile shows throughput, which has no "good" or "bad" value.** 54 KB/s is neither healthy nor sick. The other four tiles all carry thresholds (CPU >90 red, temp >75 red) — that is why they read at a glance and this one does not.
2. **The panel already measures what the owner actually wants.** `gatewayprobe` pings the gateway, 1.1.1.1, 8.8.8.8 and a DNS resolver every 5 seconds, recording RTT, jitter, packet loss and status. There is even an existing threshold function — `latencyState` in `internal/server/helpers_sparkline.go` (warn ≥80 ms or any loss; crit ≥200 ms or ≥10% loss) — used today only to colour the sparklines. A link-state verdict needs no new data and no new thresholds; it needs the tile to *ask the right question*.

## Target Users
The single Pi admin (the parent's only persona). No new user type.

## New Behavior
- The Network tile must show a one-word link state — **Stable**, **Unstable** or **Offline** — as its headline, coloured and bordered like the other tiles' warning/critical states.
- The state must be derived from the probes the panel already runs (gateway + off-box targets + the WAN monitor), never from throughput.
- The tile must show the throughput of the busiest real interface as secondary context, plus the reason behind the state (e.g. "eth0 · WAN up · 0% loss", or "eth0 · 15% loss · 8.8.8.8 timeout").
- The per-interface breakdown must remain reachable on the same tile, collapsed by default, expanding on one click.
- The tile must absorb the "WAN up" chip that currently floats loose under the metric grid.

## Success Criteria
- GIVEN every probe answers with low latency and no loss, WHEN the dashboard renders, THEN the tile reads **Stable** in the ok colour.
- GIVEN any probe reports ≥10% packet loss (2 of its 20-sample window), WHEN the dashboard renders, THEN the tile reads **Unstable** with the warning border and the subtitle names the failing target.
- GIVEN the WAN monitor reports `down`, or the gateway probe is not `ok`, WHEN the dashboard renders, THEN the tile reads **Offline** with the critical border.
- GIVEN a host with eth0 at 7.2/54.7 KB/s and two idle interfaces, WHEN the dashboard renders, THEN no per-interface row is visible until the toggle is clicked — the tile is one headline + one subtitle + one collapsed toggle.
- GIVEN the toggle is clicked, THEN every non-virtual interface's send/receive rate is shown.

## Touch Points
MODIFIES:
- `web/templates/partials/sse-metrics.html` — the Network tile, and the loose WAN chip below the grid (absorbed into it).
- `internal/server/helpers.go` — new helpers: the link-state verdict and the busiest-interface pick.
- `internal/server/templates.go` — register them in **both** FuncMaps (the file carries two).
- `internal/server/ac_coverage_test.go` — TC-001c asserts the per-interface rows render; it must now assert they render inside the collapsed detail.
- Parent AC-001-004 — its wording ("bytes in/out per second are shown per interface") must be updated to "reachable in one interaction on the tile", because the rows move behind a toggle.

PURELY ADDS: the link-state verdict itself (no FR describes it today).

DOES NOT TOUCH: `gatewayprobe`, `wanmonitor`, the metrics collector, the SSE payload shape, or `/network`. Every input already reaches the template.

## Must Not Break (Regression Boundary)
- The other four tiles (CPU, Memory, Disk, Temp) keep rendering their values and their warning/critical thresholds unchanged.
- The dashboard keeps refreshing over SSE on its existing cadence; no new event, no new payload field.
- The virtual-interface filter from BG-072 keeps working: docker0, br-*, veth* and lo stay hidden, and the filter still falls back to the unfiltered list rather than emptying the tile.
- The per-target latency sparklines further down the dashboard (`sse-charts.html`) keep working, including the BG-073 fix that made gateway/dns history load at all.
- The WAN state shown in the tile must agree with the chip it replaces — same source (`wanmonitor.Snapshot.State`), no second opinion.
- All 86 existing test cases keep passing.

## Out of Scope
- Changing what `gatewayprobe` measures, its cadence, or its targets.
- Inventing new thresholds: the verdict reuses the existing latency/loss constants rather than a second set that could disagree with the sparkline colours.
- A history or sparkline of link state over time.
- Moving throughput to `/network` or building a per-interface section there — the owner chose to keep throughput on the dashboard.
- Alerting on the link state (the alert engine already supports latency/loss rules; this is a display, not a new alert source).
- Per-interface state: this is one verdict for the box's connectivity, not one per NIC.
