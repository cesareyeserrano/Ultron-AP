# UX Spec — network-tile

Scope: FR-085 (link-state verdict), FR-087 (absorb the loose WAN chip).

> **FR-086 retired 2026-07-14** (commit `1a298e6`). The throughput subtitle and the collapsed
> per-interface disclosure were cut on the owner's call after he saw the tile in production:
> *"una sola linea o quitar alguno… esto [el desplegable] se puede eliminar"*. The tile is now
> three lines — label, verdict, reason — exactly like CPU and Temp. This spec describes what
> ships; the throughput design it used to carry is gone, not merely collapsed.

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
  2. Subtitle answers "why?" without a click: `15% loss · 8.8.8.8 ✕`.
  3. If that is enough, the admin acts (reboot the router, check the ISP). If not, the per-target latency sparklines further down the same page (`sse-charts.html`) already carry RTT/jitter/loss history per target — the drill-down exists and is unchanged.
- **Exit point:** the admin knows whether the problem is the LAN, the WAN, or a single target.
- **Error path:** if the gateway itself is unreachable the verdict is **`Offline`**, not `Unstable` — a box that cannot reach its own gateway has a different problem from one with a lossy internet path, and the word must not blur that.

### Flow C — Withdrawn

An earlier draft kept a rare "throughput check" flow behind a `<details>` disclosure, on the
reasoning that per-interface rates were the product's only throughput readout and collapsing
them cost less than deleting them. The owner disagreed once he used it: he does not read KB/s
to decide anything, and a second line of it diluted the verdict the tile exists to deliver.
The flow, the disclosure and the readout were removed together. There is no throughput anywhere
in the product now, and that is the decision, not an oversight.

---

## Component Inventory

### Screen: `/` (dashboard) — Network tile

| Component | States | Behavior | Nielsen heuristics applied |
|---|---|---|---|
| Verdict headline | `Stable` (ok) · `Unstable` (warn) · `Offline` (crit) · absent (no probes) | Server-rendered from `dashboardLinkState(.Network, .WAN)`. Rendered at the same size/weight as the other tiles' values (`text-2xl font-mono font-bold`) so the row scans as one unit. | #1 Visibility of system status; #2 Match with the real world ("Stable" is the word the System chip already uses) |
| Tile border | default · `metric-warning` · `metric-critical` | The **same** classes CPU/RAM/Temp use at their thresholds. The colour, not the word, is what carries across a room. | #4 Consistency and standards |
| Subtitle | stable: `WAN up · 0% loss` · degraded: `15% loss · 8.8.8.8 ✕` · no-probe: `no probes` | The reason behind the verdict, and nothing else. One line, `text-xs text-text-muted`. | #1 Visibility; #9 Help users diagnose |
| ~~Per-interface disclosure and rows~~ | **removed** | Retired with FR-086. The BG-072 virtual-interface filter went with them — it existed only to tidy a list that no longer exists. | #8 Minimalist design |
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
| 4. Consistency and standards | Same value size and same `metric-warning`/`metric-critical` classes as the tiles beside it. Zero new patterns — and now the same three-line shape. | Constrains the design to what exists — which is the point. |
| 5. Error prevention | The verdict cannot disagree with the sparkline colours below it: both read the same constants. Two sources of truth on one page would be the real error. | — |
| 6. Recognition over recall | The admin does not have to remember what a normal KB/s looks like for his box — the tile no longer asks him to. | — |
| 7. Flexibility and efficiency | The per-target latency sparklines below remain the drill-down for the deep case. | Throughput is no longer reachable anywhere; the owner accepted that when he cut it. |
| 8. Aesthetic and minimalist design | Visible body drops from 3 rows (14 before BG-072) to one headline + one subtitle, with nothing hidden behind an interaction. The orphan WAN chip is removed. | The per-interface data is gone rather than collapsed — the owner chose deletion over disclosure ("casi nunca lo miro"). |
| 9. Recognize, diagnose, recover | The subtitle names the failing target. `Offline` vs `Unstable` distinguishes "my LAN is broken" from "the internet path is lossy" — different fixes. | — |

**Accessibility:** the tile is static text — no control to focus, so no touch-target obligation of its own. The verdict is never colour-only — the word carries the meaning, the colour reinforces it (WCAG 1.4.1). Contrast comes from the existing `--color-danger` / yellow-400 / green-400 tokens, already covered by the project's WCAG contrast test.

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
| Subtitle | `text-xs text-text-muted` | the verdict's reason | The existing tile-context size. |

Spacing: the existing `metric-tile` padding and `space-y-1`. Nothing new.
