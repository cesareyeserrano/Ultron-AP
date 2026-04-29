# UX/UI Specification — Network Monitoring (feature)

**Archetype: PRO-TECH/DASHBOARD** — reason: monitoring/data-ops surface running on a Raspberry Pi panel; high information density, dark-first, monospace for numeric data, muted accents (green/cyan/amber/red). Inherits the parent Ultron-AP visual identity (FR-009 — semantic tokens already defined: `ok`, `warn`, `critical`, `muted`, dark mode, ≥4.5:1 contrast, ≥44×44 px touch targets, WCAG AA).

This spec adds **one new top-level section ("Network")** to the existing panel. All chrome (header, auth, logout, nav) and global tokens are inherited unchanged.

Scope mapping (FRs → screens/components is in §2):
FR-016, FR-017, FR-018, FR-019, FR-020, FR-021, FR-022, FR-023 (MUST) · FR-024, FR-025, FR-026, FR-027 (SHOULD) · FR-028, FR-029 (COULD).

---

## User Flows

Single persona: **Raspberry Pi Operator** (intermediate, single-admin, already authenticated). All flows assume FR-007 session cookie + FR-012 CSRF on writes.

### Flow A — Diagnose perceived slowness (North Star path)
Entry: operator notices a videocall stutter → opens panel → clicks `Network` in main nav.
Steps:
1. `Network → Overview` renders within 5 s. Above the fold: WAN status badge, current-IP chip, per-target RTT/jitter/loss tiles, throughput sparkline, "likely cause" hint chip.
2. If `WAN status = critical`, the badge reads `WAN down since HH:MM` (red). Operator clicks the badge → drills into `Outages` tab.
3. If `WAN status = ok` but a tile is yellow/red, operator clicks the tile → drills into `Latency detail` (per-target chart, 1h/24h/7d/30d).
4. Operator can pivot to `DNS`, `Devices`, `WiFi`, `Path` sub-tabs from the same page.
Exit: operator has a layer hypothesis (ISP / gateway / DNS / WiFi / device) within ≤30 s.
Error path: if collector is down → top-of-page banner "Network collector not running — last sample HH:MM" with a `Retry` action; tiles render in `error` state with last-known value greyed.

### Flow B — Run an on-demand speedtest (FR-024)
Entry: operator suspects ISP throttling → `Network → Overview` → clicks `Run speedtest`.
Steps:
1. Confirmation modal: "This will consume ~80 MB of WAN traffic. Continue?" (FR-009 modal pattern, FR-012 CSRF token in form).
2. On confirm → button switches to `Running… 0/3 phases` with a progress bar; existing tiles keep streaming via SSE.
3. On finish: result row appears at the top of `Speedtest history` with down/up Mbps, idle RTT, loaded RTT, bufferbloat grade (FR-025).
Exit: result persisted, visible in history.
Error paths:
- Budget exhausted (FR-023) → modal blocked: "Daily WAN-probe budget exhausted (500 MB). Resets at 00:00 UTC."
- Network down → toast "Speedtest failed — WAN unreachable. Result not recorded."

### Flow C — Configure probe targets, thresholds, retention (FR-016, FR-017, FR-022, FR-020, FR-023)
Entry: operator wants to tune probes → `Network → Settings`.
Steps:
1. Tabs: `Targets` · `DNS resolvers` · `Alert rules` · `Retention & budget`.
2. Each target row: hostname/IP, kind (icmp/udp/dns), cadence (number stepper, 5–300 s), enabled toggle, `Remove`.
3. `Add target` opens an inline form (label, host, kind, cadence) with submit-button validation.
4. Save → toast "Saved. Probe loop applies new config within 1 cycle." Form keeps inline validation errors next to each field.
Exit: config persisted; collector reconfigures within one cadence.
Error path: invalid host → field shows `Host must resolve or be a valid IPv4/IPv6` (red `error` token), submit disabled.

### Flow D — Receive a network alert (FR-022 ↔ FR-005/FR-006 inherited)
Entry: latency rule fires → existing alerts panel + Telegram/email channels (parent product).
Steps: alert appears in parent `Alerts` panel with severity colour and message body containing target, value, threshold; muting/cooldown inherited from FR-004/FR-005.
Exit: identical to parent alert experience — no new screen.
Error path: notification channel down → already covered by parent FR-005/FR-006.

### Flow E — Inspect a LAN device (FR-027)
Entry: operator wonders which device is hogging the LAN → `Network → Devices`.
Steps:
1. Table: hostname, IP, MAC, vendor, first-seen, last-seen, status badge.
2. Sortable by `last-seen DESC` by default.
3. Click row → drawer shows extended info (OUI vendor, all observed IPs, history of appearances).
Exit: operator identifies the device.
Error path: ARP table empty (Pi just rebooted) → empty state: "No LAN devices observed yet. Devices appear as they generate ARP/mDNS traffic."

### Flow F — Verify monitor isn't hurting the Pi (FR-023, guardrail)
Entry: operator suspects Ultron is heavy → `Network → Overview` → footer card `Monitor cost`.
Steps: shows current CPU%, RAM MB, last-24h probe BW used, with a `breaker active` badge if FR-023 throttling kicked in.
Exit: operator knows the cost.
Error path: telemetry unavailable → card shows `—` and the empty-state hint "Cost telemetry not yet available — waits 5 minutes after start".

---

## Component Inventory

All components inherit FR-009 tokens. Each defines the five mandatory states.

### Screen: `Network → Overview`

| Component | States | Behavior | Nielsen heuristic |
|---|---|---|---|
| `WanStatusBadge` | default(ok green) · loading(skeleton bar) · error("collector unreachable" + Retry) · empty("no data yet — waits 30 s after start") · disabled(n/a — FR is MUST) | Reads aggregated FR-018; clickable → `Outages` tab. | H1 Visibility of system status |
| `PublicIpChip` | default(`HHHH.HHHH.HHHH.HHHH` mono) · loading(skeleton) · error("unknown — last verified HH:MM") · empty("not yet checked") · disabled(n/a) | FR-026. Tooltip: "Last change: …". | H1 Visibility of system status |
| `TargetTile` (one per target) | default(rtt + sparkline) · loading(skeleton) · error("probe failing — N consecutive timeouts" + Retry) · empty("awaiting first sample") · disabled(toggle off in settings → greyed with "Disabled") | Renders RTT (large mono), jitter (small), loss% (badge). Click → `Latency detail`. | H1, H4 Consistency, H6 Recognition |
| `ThroughputSparkline` | default(area chart) · loading(skeleton) · error · empty("no traffic recorded yet") · disabled | Reads Pi nic counters (FR-019/021 — bytes in/out). | H1 |
| `LikelyCauseChip` | default(green "Healthy") / yellow("DNS resolver slow") / red("WAN down") · loading · error · empty("collecting baseline…") · disabled(n/a) | Deterministic rule chain (no ML): (1) WAN-down → "WAN", (2) DNS p95 > X → "DNS", (3) WiFi RSSI < -75 dBm → "WiFi", (4) gateway loss > 0 → "LAN/Gateway", else "Healthy". | H10 Help users diagnose |
| `RunSpeedtestButton` | default · loading("Running… 0/3 phases" + progress) · error(toast) · empty(n/a — always present) · disabled("Budget exhausted — resets 00:00 UTC", or "Already running") | FR-024 + FR-023 gating. Shows confirmation modal. | H3 User control, H5 Error prevention |
| `MonitorCostCard` | default(CPU%, RAM MB, 24h BW, breaker badge if active) · loading · error · empty("warming up — waits 5 min") · disabled(n/a) | FR-023 visibility. Breaker active → amber `warn` badge "Throttled". | H1, H10 |
| `OutagesList` (Outages tab) | default(rows: start, end, duration, severity) · loading · error("history unavailable — DB locked" + Retry) · empty("No outages recorded — well done.") · disabled | FR-018. Sortable. | H1, H6 |
| `SpeedtestHistoryTable` | default(rows: date, down, up, idle RTT, loaded RTT, bufferbloat grade) · loading · error · empty("No speedtest run yet — click 'Run speedtest'") · disabled | FR-024 + FR-025. | H1, H7 Flexibility |

### Screen: `Network → Latency detail` (per-target drill-in)

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| `LatencyChart` (window selector 1h/24h/7d/30d) | default(line chart, p50 + p95 band) · loading(skeleton) · error("no data for this window") · empty("no samples in window") · disabled(target removed → "Target deleted, history retained") | FR-020 — ≤600 points, downsampling. | H1, H6 |
| `WindowTabs` | default · loading(disabled while fetching) · error · empty(n/a) · disabled | Selects 1h/24h/7d/30d. | H7 |
| `JitterMiniChart` | default · loading · error · empty · disabled | EWMA jitter from FR-016. | H1 |
| `LossSparkline` | default · loading · error · empty · disabled | Per-window loss%. | H1 |

### Screen: `Network → DNS`

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| `DnsResolverGrid` | default(card per (resolver, domain): last_ms, status, 1h fail rate) · loading · error · empty("No DNS probes configured — Settings → DNS resolvers") · disabled(probe disabled → greyed) | FR-017. | H1, H10 |

### Screen: `Network → Devices`

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| `DevicesTable` | default(sortable) · loading · error · empty("No LAN devices observed yet.") · disabled(discovery off in settings → "LAN discovery disabled") | FR-027. | H1, H6, H7 |
| `DeviceDetailDrawer` | default · loading · error · empty(n/a — only opens with row) · disabled(n/a) | Slides from right at desktop, full-screen at 375px. | H3, H7 |

### Screen: `Network → WiFi` (FR-028, conditional)

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| `WifiPanel` | default(RSSI gauge, SNR, bitrate, channel, retries, CRC errors) · loading · error · empty("No samples yet") · disabled(`Not applicable — Pi is on Ethernet`) | FR-028. Disabled state is the **default** when wlan is down — and is what most operators will see. | H1, H10 |

### Screen: `Network → Path` (FR-029)

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| `PathHopList` | default(numbered hop list with IPs and RTTs) · loading · error · empty("Path data appears after 2 traceroute runs (~hourly).") · disabled(disabled in settings) | FR-029. Path-changed events flagged with amber. | H1 |

### Screen: `Network → Settings`

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| `TargetsTable` | default · loading · error · empty("No targets — defaults will be used") · disabled(read-only when collector is reloading) | FR-016 config. | H3, H5, H7 |
| `AddTargetForm` | default · loading(submit) · error(inline per field) · empty(n/a) · disabled(during save) | Inline validation on host field, FR-012 CSRF. | H5, H9 Help users recognize errors |
| `AlertRulesTable` | default · loading · error · empty("No network rules — using defaults") · disabled | FR-022 — extends FR-004 alert engine. | H5, H7 |
| `RetentionAndBudgetForm` | default · loading · error(inline) · empty(n/a) · disabled | FR-020 retention + FR-023 CPU/RAM/BW budgets. | H5, H10 |

### Cross-cutting components

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| `NavLinkNetwork` | default · loading(n/a) · error(n/a) · empty(n/a) · disabled(n/a — always present once feature flag on) | Adds `Network` to parent nav. | H4 Consistency |
| `Toast` (success / warn / error) | default · loading(n/a) · error(self) · empty(n/a) · disabled(n/a) | Reuses parent component. | H1 |
| `ConfirmModal` (Stop/Restart-style) | default · loading(during action) · error(stays open with error text) · empty(n/a) · disabled(during action) | Reused from parent (FR-008 pattern) for `Run speedtest`, `Remove target`, `Reset breaker`. | H3, H5 |

---

## Nielsen Compliance

Per screen, the heuristics that drove the design and any trade-offs.

### Network → Overview
- **H1 Visibility of system status** — WAN badge, public IP chip, per-target tiles, monitor-cost card all show live state with timestamps.
- **H4 Consistency** — reuses parent FR-009 tokens, badge shapes, table rows; nav follows parent pattern.
- **H6 Recognition rather than recall** — semantic colours (green/amber/red) + textual labels; no abbreviations without tooltip.
- **H10 Help diagnose problems** — `LikelyCauseChip` translates raw metrics into a one-line layer hypothesis.
- **Trade-off accepted**: density is high (PRO-TECH archetype) — at 375 px, tiles stack vertically; we accept 2+ screen-heights of scroll because the operator typically lands here in a triage state and skims top-down.

### Network → Latency detail
- **H1, H6** — chart with p50 line + p95 band so spikes are recognisable, not buried in averages.
- **H7 Flexibility** — 4 window presets cover the realistic triage span without giving free-form date pickers (which add complexity for one-user product).
- **Trade-off**: no zoom-to-region in v1; users wanting deeper drill-in get the 30-day window, not arbitrary ranges.

### Network → DNS, Devices, WiFi, Path
- **H1, H6** — every panel surfaces last-sample timestamp.
- **H10** — empty states explain *why* no data yet (waiting time, hardware not applicable, traceroute cadence).
- **WiFi disabled-by-default** — when Pi is on Ethernet, the panel is the *normal* path; we show "Not applicable" rather than empty so operators do not chase a non-issue (corrected H9 violation: would-be confusing "no signal data" without context).

### Network → Settings
- **H5 Error prevention** — inline validation, cadence stepper bounded 5–300 s, `Remove target` confirmation.
- **H3 User control** — every save is undoable by editing again; no destructive action without confirm.
- **H9 Recognise & recover from errors** — field-level errors with the field, not page-level.
- **Trade-off**: no "Restore defaults" button in v1 to keep settings flat — operators copy values manually if they want to revert.

### Run speedtest / Confirm modal
- **H3 User control + H5 Error prevention** — confirmation cites BW cost; cancel is the default focus.
- **H1** — running state has phase counter (`0/3 phases`), not just a spinner.

### Heuristic count
8 of 10 Nielsen heuristics actively applied (H1, H3, H4, H5, H6, H7, H9, H10). H2 (match real world) and H8 (aesthetic minimalism) inherit from parent tokens without feature-specific design.

### Violations log
- 0 found violations.
- 1 corrected during design — initial draft of WiFi panel showed an empty signal gauge when Pi was Ethernet-only; corrected to the explicit "Not applicable" disabled state (H9).
- 0 accepted as trade-off.

---

## Design Tokens

This feature **inherits** parent FR-009 tokens unchanged — re-stated here so the developer has the full contract in one place. No token is invented for this feature; the only additions are *semantic role bindings* for new metrics.

### Color roles (dark-first, archetype: PRO-TECH/DASHBOARD)

| Role | Hex | Usage | Reason |
|---|---|---|---|
| `bg.canvas` | `#0B0F14` | Page background | Parent FR-009 dark-mode anchor; reduces glare during night ops. |
| `bg.surface` | `#121821` | Cards, drawers | One step elevation above canvas; visually separates tiles. |
| `bg.surfaceAlt` | `#192230` | Table-row hover, modal | Second elevation; consistent with parent. |
| `border.subtle` | `#1F2A38` | Card and table borders | Lower contrast than text — borders are structure, not signal. |
| `text.primary` | `#E5ECF3` | Body, numeric values | Contrast 13.4:1 on `bg.canvas` — well above WCAG AA. |
| `text.secondary` | `#9AA7B5` | Labels, axis text | Contrast 6.0:1 on `bg.canvas` — meets AA for body. |
| `text.muted` | `#6B7886` | Disabled, "n/a" placeholders | Contrast 4.6:1 — minimum AA. Used on disabled states only. |
| `accent.primary` | `#3B82F6` | Selected nav item, primary action | Inherits parent action token. |
| `status.ok` | `#3FB950` | Healthy badges, ok dots | Parent semantic green. |
| `status.warn` | `#D29922` | Yellow tiles, throttled badge, breaker active | Parent semantic amber. |
| `status.critical` | `#F85149` | WAN down, exceeded thresholds, errors | Parent semantic red. |
| `status.info` | `#58A6FF` | Info events (IP change, path change) | Parent semantic blue. |
| `chart.series.rtt` | `#58A6FF` | Latency line | Distinct from status colours so the chart isn't read as state. |
| `chart.series.jitter` | `#A371F7` | Jitter line | Violet — the only new data hue, picked for colour-blind separability vs. blue. |
| `chart.series.loss` | `#F85149` | Loss area | Reuses critical red — loss IS the bad state. |
| `chart.series.throughput.in` | `#3FB950` | Bytes in | Reuses ok green for inbound (asymmetric homes typically rate-limited on egress). |
| `chart.series.throughput.out` | `#D29922` | Bytes out | Amber to distinguish from in. |
| `chart.band.p95` | `rgba(88,166,255,0.18)` | p95 confidence band | 18% alpha on the rtt hue keeps the line readable. |

All foreground/background pairs verified ≥4.5:1 (body) and ≥3:1 (large text and UI components). No gaps.

### Typography

| Token | Value | Reason |
|---|---|---|
| `font.sans` | `Inter, system-ui, -apple-system, "Segoe UI", Helvetica, Arial, sans-serif` | Inherited from parent; Inter has tabular numerals for data UI. |
| `font.mono` | `"JetBrains Mono", "SF Mono", Menlo, Consolas, monospace` | PRO-TECH archetype — IPs, MACs, hex IDs, RTT values use mono so columns align. |
| `font.size.xs` | `12px` | Axis labels, chip text. |
| `font.size.sm` | `14px` | Table body, secondary labels. |
| `font.size.base` | `15px` | Body text — 1px above parent default for the dense tile values. |
| `font.size.lg` | `18px` | Tile titles, table headers. |
| `font.size.xl` | `24px` | Tile primary number (RTT, throughput). |
| `font.size.xxl` | `32px` | Page-level "Network" header on desktop. |
| `font.weight.regular` | `400` | Body. |
| `font.weight.medium` | `500` | Labels, table headers. |
| `font.weight.semibold` | `600` | Tile primary numbers, page headers. |
| `font.lineHeight.tight` | `1.2` | Numeric values. |
| `font.lineHeight.normal` | `1.5` | Body, paragraphs. |
| `font.feature.tabularNums` | `"tnum" 1, "lnum" 1` | Required on every numeric value so 7s and 1s align in tables. |

### Spacing

8-pt scale, inherited from parent:
`space.0=0`, `space.1=4px`, `space.2=8px`, `space.3=12px`, `space.4=16px`, `space.5=24px`, `space.6=32px`, `space.7=48px`, `space.8=64px`.

Reason: every density decision in this feature must land on the 8-pt grid so it lines up visually with parent screens (Dashboard, Docker, Services).

### Radius / elevation

| Token | Value | Reason |
|---|---|---|
| `radius.sm` | `4px` | Badges, chips. |
| `radius.md` | `8px` | Cards, tiles. |
| `radius.lg` | `12px` | Modals, drawers. |
| `elevation.0` | `none` | Canvas. |
| `elevation.1` | `0 1px 2px rgba(0,0,0,0.4)` | Cards. |
| `elevation.2` | `0 8px 24px rgba(0,0,0,0.5)` | Modals, drawers. |

### Motion

| Token | Value | Reason |
|---|---|---|
| `motion.duration.fast` | `120ms` | State changes (hover, badge colour). |
| `motion.duration.base` | `180ms` | Drawer open/close. |
| `motion.duration.modal` | `200ms` | Confirm modal entrance. |
| `motion.easing.standard` | `cubic-bezier(0.2, 0, 0, 1)` | Material-style; consistent with parent. |

PRO-TECH defaults forbid decorative animation. The only motion is functional feedback (≤200 ms, requirement 4 of UX standards).

### Touch targets

All interactive elements ≥44×44 px (FR-009 inherited). Number steppers in `RetentionAndBudgetForm` use stacked +/− buttons each 44×44 px on mobile, side-by-side on desktop.

---

## 5. Responsive Breakpoints

| Breakpoint | Layout |
|---|---|
| `375px` (mobile) | Tiles stack 1-col. Sub-tabs (`Overview / DNS / Devices / WiFi / Path / Settings`) collapse into a horizontal scroll-tab strip. Tables become card lists. Drawers open full-screen. |
| `768px` (tablet) | Tiles 2-col. Sub-tabs fit on one row. Tables remain tables with horizontal scroll if needed. Drawers slide from right at 480 px width. |
| `1440px` (desktop) | Tiles 4-col on Overview. Charts span full width. Drawers slide from right at 560 px. Side-by-side detail + chart on Latency drill-in. |

Mobile (375 px) behaviour is explicit per screen:
- **Overview**: WAN badge full-width, public IP chip below, target tiles 1-col, throughput sparkline full-width, likely-cause chip full-width, monitor-cost card full-width. Run-speedtest button is sticky in the bottom-bar so it remains thumb-reachable.
- **Latency detail**: window tabs scrollable; chart full-width with reduced y-axis label density.
- **DNS, Devices, WiFi, Path**: list/card layout (no horizontal scroll on tables — tables become cards).
- **Settings**: forms 1-col, save button sticky bottom.

---

## 6. Coverage check

Every UX/visual/reporting FR maps to at least one screen and one component:

| FR | Screen | Component(s) |
|---|---|---|
| FR-016 | Overview · Latency detail | TargetTile, LatencyChart, JitterMiniChart, LossSparkline |
| FR-017 | DNS | DnsResolverGrid |
| FR-018 | Overview · Outages tab | WanStatusBadge, OutagesList |
| FR-019 | Overview | Whole screen |
| FR-020 | Latency detail (and embedded charts) | LatencyChart, WindowTabs |
| FR-021 | (cross-cutting — persistence) | All historical components depend on it |
| FR-022 | Settings → Alert rules | AlertRulesTable |
| FR-023 | Overview footer · Settings → Retention & budget | MonitorCostCard, RetentionAndBudgetForm |
| FR-024 | Overview | RunSpeedtestButton, SpeedtestHistoryTable |
| FR-025 | Overview (history table) | SpeedtestHistoryTable bufferbloat column |
| FR-026 | Overview | PublicIpChip |
| FR-027 | Devices | DevicesTable, DeviceDetailDrawer |
| FR-028 | WiFi | WifiPanel |
| FR-029 | Path | PathHopList |

No FR without a surface. No surface without an FR.
