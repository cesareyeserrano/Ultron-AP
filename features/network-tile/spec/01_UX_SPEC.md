# UX Spec — network-tile

Scope: FR-085 (link-state verdict), FR-086 (throughput subtitle + collapsed per-interface detail), FR-087 (absorb the loose WAN chip).

One tile changes. No new screen, no new widget, no new JS. The design authority is **the four tiles standing next to it** — CPU, Memory, Disk, Temp — which already solved this problem: label, one big value, at most two lines of context, and a border that turns yellow or red when the value crosses a threshold. Network is the only tile in that row that never adopted the pattern, and that is the whole defect.

---

## User Flows

Persona: **Pi Admin (sole operator)** — the parent's only persona.

### Flow A — The glance (the 95% case)

- **Entry point:** the admin opens `/` for the ritual "is anything wrong?" check.
- **Steps:**
  1. The eye scans one row: `22.2%` · `45.7%` · `8.2%` · **`Stable`** · `51°C`.
  2. Nothing is yellow or red. Done — the admin closes the tab.
- **Exit point:** under a second, no reading, no interpretation.
- **What changed:** today step 2 is impossible. `eth0 ↑7.2 KB/s ↓54.7 KB/s` forces the admin to *decide* whether 7.2 is good — and it is neither good nor bad. The tile is the only one that cannot be scanned.

### Flow B — Something is wrong (the case the tile exists for)

- **Entry point:** the admin notices the Network tile is the one with a yellow border.
- **Steps:**
  1. Headline: **`Unstable`**.
  2. Subtitle answers "why?" without a click: `eth0 · 15% loss · 8.8.8.8 ✕`.
  3. If that is enough, the admin acts (reboot the router, check the ISP). If not, the per-target latency sparklines further down the same page (`sse-charts.html`) already carry RTT/jitter/loss history per target — the drill-down exists and is unchanged.
- **Exit point:** the admin knows whether the problem is the LAN, the WAN, or a single target.
- **Error path:** if the gateway itself is unreachable the verdict is **`Offline`**, not `Unstable` — a box that cannot reach its own gateway has a different problem from one with a lossy internet path, and the word must not blur that.

### Flow C — The rare throughput check

- **Entry point:** the admin wants to see what an interface is actually moving (his own words: *"casi nunca lo miro"*).
- **Steps:**
  1. Click `▸ per-interface` on the tile.
  2. Rows expand: `eth0 ↑7.2 ↓54.7`, `wlan0 ↑0 B ↓0 B`, `tailscale0 ↑0 B ↓0 B`.
  3. Click again to collapse.
- **Exit point:** the data he rarely needs is one click away, and never in his face.
- **Why not move it to `/network`:** it is the only throughput readout in the entire product. Deleting it from the dashboard would mean rebuilding it elsewhere (new handler, new route, new polling fragment) to end up with **less**. Collapsing it costs one `<details>` element.

---

## Component Inventory

### Screen: `/` (dashboard) — Network tile

| Component | States | Behavior | Nielsen heuristics applied |
|---|---|---|---|
| Verdict headline | `Stable` (ok) · `Unstable` (warn) · `Offline` (crit) · absent (no probes) | Server-rendered from `dashboardLinkState(.Network, .WAN)`. Rendered at the same size/weight as the other tiles' values (`text-2xl font-mono font-bold`) so the row scans as one unit. | #1 Visibility of system status; #2 Match with the real world ("Stable" is the word the System chip already uses) |
| Tile border | default · `metric-warning` · `metric-critical` | The **same** classes CPU/RAM/Temp use at their thresholds. The colour, not the word, is what carries across a room. | #4 Consistency and standards |
| Subtitle | stable: `eth0 · WAN up · 0% loss` · degraded: `eth0 · 15% loss · 8.8.8.8 ✕` · no-probe: throughput only | Names the busiest real interface, then the reason for the verdict. One line, `text-xs text-text-muted`. | #1 Visibility; #9 Help users diagnose |
| Per-interface disclosure | collapsed (**default**) · expanded | Native `<details>`/`<summary>` — the same element `sse-summary.html` already uses for Apps/VPN/Containers. No JS. Summary is the ≥44px touch target the project enforces. | #8 Aesthetic and minimalist design; #7 Flexibility (progressive disclosure) |
| Per-interface rows | one per non-virtual interface | `name ↑ sent/s ↓ recv/s`, `text-xs font-mono`. The BG-072 filter still hides docker0/br-*/veth*/lo. | #6 Recognition over recall |
| ~~Standalone WAN chip~~ | **removed** | Its state is now the tile's verdict and subtitle. One orphan element leaves the page (FR-087). | #8 Minimalist design; removes a second source of truth |

### Not changed (regression boundary)

| Component | Contract |
|---|---|
| CPU / Memory / Disk / Temp tiles | Identical markup, identical thresholds, identical values. |
| Per-target latency sparklines (`sse-charts.html`) | Unchanged — they remain the drill-down for *why* the link is unstable, including the BG-073 fix that made gateway/dns history load at all. |
| SSE refresh | Same event, same cadence, same payload shape. The verdict is computed at render from data already in `DashboardData`. |

---

## Nielsen Compliance

| Heuristic | How the design satisfies it | Trade-off |
|---|---|---|
| 1. Visibility of system status | The tile finally reports a *status* instead of a measurement. The subtitle says why, so a yellow border is never a mystery. | — |
| 2. Match with the real world | "Stable / Unstable / Offline" is how an admin describes a link. "7.2 KB/s" is how a byte counter describes it. | "Unstable" is a judgement the system makes on the admin's behalf — it must therefore be *right*, which is why it reuses the thresholds the sparklines already colour by rather than a fresh guess. |
| 4. Consistency and standards | Same value size, same `metric-warning`/`metric-critical` classes, same `<details>` disclosure as elsewhere on the page. Zero new patterns. | Constrains the design to what exists — which is the point. |
| 5. Error prevention | The verdict cannot disagree with the sparkline colours below it: both read the same constants. Two sources of truth on one page would be the real error. | — |
| 6. Recognition over recall | The admin does not have to remember what a normal KB/s looks like for his box. | — |
| 7. Flexibility and efficiency | Throughput is one click away for the rare case; the sparklines below remain for the deep case. | — |
| 8. Aesthetic and minimalist design | Visible body drops from 3 rows (14 before BG-072) to one headline + one subtitle. The orphan WAN chip is removed. | The per-interface data is one interaction away rather than zero — the owner explicitly accepted this ("casi nunca lo miro"). |
| 9. Recognize, diagnose, recover | The subtitle names the failing target. `Offline` vs `Unstable` distinguishes "my LAN is broken" from "the internet path is lossy" — different fixes. | — |

**Accessibility:** the `<summary>` is natively focusable and keyboard-operable (Enter/Space) and carries the ≥44px touch target the parent's AC-009-004 enforces. The verdict is never colour-only — the word carries the meaning, the colour reinforces it (WCAG 1.4.1). Contrast comes from the existing `--color-danger` / yellow-400 / green-400 tokens, already covered by the project's WCAG contrast test.

---

## Design Tokens

**Authority: the existing product.** No new token. The tile borrows exactly what the other four already use — inventing a colour here would break the WCAG contrast test that now computes ratios from `input.css`.

| Role | Token / class | Used for | Reason |
|---|---|---|---|
| Verdict — ok | `text-green-400` | `Stable` | The colour the product already means "healthy" by (active services, applied pills). |
| Verdict — warn | `text-yellow-400` + `metric-warning` on the tile | `Unstable` | Same pair CPU uses above 75%. |
| Verdict — crit | `text-danger` (`#e34b6a`) + `metric-critical` | `Offline` | Same pair CPU uses above 90% and Temp above 75°C. |
| Verdict — unknown | `text-text-muted` (`#9ca3af`) | no probes | The product's "we don't know" grey (7.3:1 on base — still AA body, so "unknown" is readable, not decorative). |
| Value size | `text-2xl font-mono font-bold` | the verdict word | Identical to `22.2%` / `51°C`, so the row reads as one scale. |
| Subtitle | `text-xs text-text-muted` | interface + reason | The existing tile-context size. |
| Detail rows | `text-xs font-mono` | per-interface rates | Monospace so the rates column-align. |
| Disclosure | native `<details>` + `min-h-[44px]` summary | per-interface toggle | Reuses `sse-summary.html`'s pattern and the project's 44px touch-target floor. |

Spacing: the existing `metric-tile` padding and `space-y-1`. Nothing new.
