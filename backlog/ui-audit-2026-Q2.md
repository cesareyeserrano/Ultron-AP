# UI Audit — Ultron-AP — 2026 Q2 (BL-019)

Scope: layout, hierarchy, duplication, input ergonomics, responsiveness. Visual redesign and a11y are explicitly out of scope.

Reviewer pass: server-rendered Go templates under `web/templates/`. Findings cite file and line.

---

## Dashboard

### [P1] Status-tile row duplicates "Data Freshness" and "Live Telemetry"
- **Location:** `web/templates/partials/sse-metrics.html:15-43`
- **Category:** duplication
- **Issue:** The 4-card top strip exposes two cards driven by the exact same state (`.MetricsStale` / `.Metrics`): "Data Freshness" (Healthy/Delayed/Connecting + age) and "Live Telemetry" (Live/Delayed/Waiting + last update). Same colour, same age value (`MetricsAgeSec`), same conditional, restated with synonym labels. On a 4-up grid that is half the row.
- **Recommendation:** Collapse into one "Telemetry" tile that shows live/delayed/waiting + age + reconnect/logs CTAs. Reclaim the slot for something the dashboard does not currently surface (e.g. WAN/network rollup, alert rollup).
- **Feeds into:** standalone ticket

### [P2] WAN status appears in two places on dashboard load
- **Location:** `web/templates/partials/sse-metrics.html:81-91` and the WAN chip later inserted by SSE; also duplicated on `web/templates/network.html:14-25`
- **Category:** duplication
- **Issue:** A small "WAN up/down" pill is rendered at the bottom-right of the metrics partial whenever `.Network` and `.WAN` are present, while the Network page renders the same pill at the top of its own page. Same data, two surfaces, no clear primary.
- **Recommendation:** Pick one canonical home for the WAN pill (top-right of dashboard header strip is fine) and remove from the metric partial — it adds visual noise to the metric grid.
- **Feeds into:** standalone ticket

### [P2] Timeline-window chips floated right with `justify-end` look orphaned above charts
- **Location:** `web/templates/dashboard.html:10-18`
- **Category:** layout
- **Issue:** The `5m / 2h / 6h / 12h / 24h` chip row uses `justify-end` and ships two micro-meta spans (`window: 5m`, `samples: N`) at 10px size. The chip row is not wrapped in a labelled container, so it visually attaches to nothing — neither the metric grid above nor the chart grid below.
- **Recommendation:** Either anchor the chip row inside a small "Charts" header bar with a left-aligned section title and right-aligned chips, or place the chips above each individual chart card. Today they read as global controls but are positioned awkwardly.
- **Feeds into:** standalone ticket

### [P3] Section heading hierarchy is inconsistent on dashboard
- **Location:** `web/templates/dashboard.html:40, 47`; `web/templates/partials/sse-verdicts.html:3`
- **Category:** hierarchy
- **Issue:** Some sections render an `h2` ("Operational Indicators", "System Summary"), others have no heading (the metric grid, the chart grid, the timeline chip row). The metric strip is the most prominent block on the page yet has no label.
- **Recommendation:** Either label every section with the same `text-sm font-semibold text-text-muted uppercase tracking-wider` heading style, or make all of them headerless. Right now the top half of the page has no headings and the bottom half does, which looks like two pages stitched together.
- **Feeds into:** standalone ticket

### [P2] System Summary cards have uneven density
- **Location:** `web/templates/partials/sse-summary.html:1-100`
- **Category:** hierarchy
- **Issue:** Three cards (Apps / VPN / Containers). The middle card (VPN) is much taller because it always renders the peer list inline (no `<details>`), while Apps and Containers are wrapped in `<details>` (collapsed-by-default semantically, force-opened by `data-default-open` JS). On `lg:grid-cols-3` the whole row stretches to the tallest card, so Apps and Containers grow extra whitespace when VPN has many peers.
- **Recommendation:** Make all three cards use the same disclosure pattern — either all `<details>` summaries or all inline lists. Pick one and make the row flow predictably.
- **Feeds into:** standalone ticket

---

## Network

### [P1] WAN banner is a flex row with body text that wraps awkwardly on tablet
- **Location:** `web/templates/network.html:14-25`
- **Category:** responsive
- **Issue:** The WAN banner uses `flex-wrap items-center justify-between` between a long descriptive paragraph and a small status pill. On tablet widths the paragraph wraps to 2-3 lines while the pill stays a single chip far to the right; the visual centroid is off and the descriptive copy reads more important than the actual state pill.
- **Recommendation:** Promote the state pill to be the dominant element (large, top), demote the descriptive paragraph to a sub-line beneath it, drop the `justify-between` flex layout. The pill is the data; the paragraph is meta.
- **Feeds into:** standalone ticket

### [P1] LAN devices table has no horizontal-scroll affordance on narrow viewports
- **Location:** `web/templates/partials/lan-devices.html:4-39`
- **Category:** responsive
- **Issue:** The table is wrapped in `overflow-x-auto` which is correct, but there is no visual cue (gradient fade, scroll hint) and the columns (IP, MAC, Vendor, Online, Last seen) collectively exceed any phone viewport. On mobile users will miss columns silently. Also no `min-w` on the table forces it to compress unreadable rather than scroll.
- **Recommendation:** Either give the table a `min-w` so overflow always triggers and add a subtle right-edge fade indicator, or render a card-list layout below `md:` instead of a table. The current behaviour silently drops information density.
- **Feeds into:** standalone ticket

### [P3] Section title `<h2 class="sr-only">Network</h2>` is hidden on every page
- **Location:** `web/templates/network.html:3`; same pattern in services/docker/alerts/history/logs/settings
- **Category:** hierarchy
- **Issue:** Every page hides its top-level heading via `sr-only` and relies on the header bar `<h1>{{.Title}}</h1>` rendered by `partials/header.html`. The header bar is only `text-lg`. So the visible page title is small (lg = 18px), while all section labels inside the page are `text-sm uppercase`. There is no clear visual `H1`.
- **Recommendation:** Either drop the `sr-only` H2 and promote it to a visible page title at the top of `<main>` (with breathing room), or beef up the header `h1` to `text-2xl` so it does the job. Today the whole app reads "uppercase eyebrow soup" with no anchor.
- **Feeds into:** standalone ticket

---

## Services

### [P3] Page is otherwise minimal — fine
- **Location:** `web/templates/services.html`
- **Category:** -
- **Issue:** Page is a thin wrapper; real content lives in `partials/services-list.html`. No issues at this level.
- **Recommendation:** -
- **Feeds into:** -

### [P1] Per-service action buttons (Start/Stop/Restart) cause row reflow on mobile
- **Location:** `web/templates/partials/services-list.html:20-90`
- **Category:** responsive
- **Issue:** Each service row uses `flex flex-wrap md:flex-nowrap`. Below `md`, the buttons drop to a second row at `w-full` with `md:justify-end`, which lifts the action cluster to full-width. With three buttons + disabled-state padding the row becomes ~140px tall per service — on a Pi with 30+ services this is a long unscannable scroll.
- **Recommendation:** On mobile collapse to a single overflow menu (kebab) per row, or hide labels and show icons-only. The current "everything full-width on mobile" pattern is the slowest possible scan order.
- **Feeds into:** standalone ticket

### [P2] Service row metadata (CPU/RAM pills, status pill, info `i`) crowds the name
- **Location:** `web/templates/partials/services-list.html:32-44`
- **Category:** hierarchy
- **Issue:** The first inline row puts: name + tooltip-i + active-state pill + (optional) CPU pill + RAM pill, all `flex-wrap`. With `serviceHasRuntime` true and a long name, the wrap order is unpredictable and the pills lose alignment between rows. There is no breathing room (no `gap-y` on wrap).
- **Recommendation:** Pin name+state-pill on the first line; move CPU/RAM/info-icon to a second meta line in the description block (where Description already lives). That gives a stable two-line card and runtime info gets its own predictable strip.
- **Feeds into:** standalone ticket

### [P2] Group header chip strip lacks a dominant element
- **Location:** `web/templates/partials/services-list.html:6-15`
- **Category:** hierarchy
- **Issue:** Group `<summary>` shows: group label pill + "N active" + "M failed" + "K total" + chevron. All text-xs, all text-text-muted (except the failed count). Reads as five competing chips with the only visual hint of priority being the danger color when a service is failed. The group itself (e.g. "Network") is the same size as the metadata.
- **Recommendation:** Promote the group label to `text-sm font-semibold` and shrink the count meta to a single trailing meta line (e.g. `42 services · 41 active · 1 failed`). Today group headers and counts visually compete.
- **Feeds into:** standalone ticket

---

## Docker

### [P2] Container card stat line cramps `CPU`/`MEM` together with a hard `&nbsp;`
- **Location:** `web/templates/partials/docker-list.html:28-30`
- **Category:** hierarchy
- **Issue:** `CPU 1.2%   MEM 12.4%` is rendered as a single `<p>` with manual non-breaking-space separator. On narrow viewports both metrics wrap onto the same line awkwardly and on wide screens they collide with the container name above.
- **Recommendation:** Two pill chips (`CPU 1.2%`, `MEM 12.4%`) like services rows. Aligns visual vocabulary across pages and survives wrapping.
- **Feeds into:** standalone ticket

### [P1] Same control-button reflow problem as services
- **Location:** `web/templates/partials/docker-list.html:34-78`
- **Category:** responsive
- **Issue:** Same issue as services: full-width Start/Stop/Restart cluster on mobile, mixed disabled+enabled states, no overflow menu. Compound with services means two pages have the same flaw.
- **Recommendation:** Same fix: kebab menu on mobile, or icon-only buttons. Tackle both (services + docker) together.
- **Feeds into:** linked to services finding above

### [P3] Docker page top banner only renders pill when unavailable
- **Location:** `web/templates/docker.html:4-9`
- **Category:** layout
- **Issue:** When Docker is available the top row shows only "Container runtime status and controls." with empty space to the right. The banner exists only to host the unavailability pill, which is dead space 99% of the time.
- **Recommendation:** Drop the empty banner; surface the unavailability state inline (full-width error banner) when it occurs. Same applies to services.html:4-9.
- **Feeds into:** standalone ticket

---

## Alerts

### [P1] Severity filter chips use four different colour treatments
- **Location:** `web/templates/alerts.html:31-48`
- **Category:** hierarchy
- **Issue:** Active "All" → `bg-accent text-base`; active "Critical" → `bg-danger text-white`; active "Warning" → `bg-yellow-400 text-base`; active "Info" → `bg-accent text-base` (same as All — collision). Idle states are uniform. So All-active and Info-active look identical, and Critical-active uses a different text colour token (`text-white` vs `text-base`) than the others.
- **Recommendation:** Pick one active-state treatment (e.g. `bg-card border-accent text-text` ring) and apply uniformly. Severity meaning belongs on the alert card, not on the filter chip — the filter is just a tab.
- **Feeds into:** standalone ticket

### [P2] Unack count shown twice (sidebar badge + page pill)
- **Location:** `web/templates/partials/sidebar.html:54-55` (badge) and `web/templates/alerts.html:8-10` (pill)
- **Category:** duplication
- **Issue:** The sidebar exposes a global `alert-badge` count and the alerts page itself shows `{{.Content.UnackCount}} unacknowledged` again at the top right. On the alerts page both numbers are visible simultaneously.
- **Recommendation:** Hide the sidebar badge while on `/alerts` (the JS already mutes the dot via `data-active-page` — extend to the numeric badge), or remove the in-page pill.
- **Feeds into:** standalone ticket

### [P3] Loading spinner SVG embedded inside the "Clear Alerts" button label
- **Location:** `web/templates/alerts.html:21-25`
- **Category:** layout
- **Issue:** The htmx-indicator spinner SVG sits inline before the "Clear Alerts" text. With `htmx-indicator` hidden by default the button has a phantom 14px gap on the left.
- **Recommendation:** Use a positioned overlay spinner pattern, or use `hidden` + class swap so the gap collapses when idle.
- **Feeds into:** standalone ticket

---

## History

### [P2] "Clear History" button has no count or last-action context
- **Location:** `web/templates/history.html:4-20`
- **Category:** hierarchy
- **Issue:** Header strip is "Audit trail for service and container operations." + Clear History button. There's no counter (how many records about to be cleared), no date range. The button is consequential and yet has less context than the alerts equivalent.
- **Recommendation:** Add a meta line: "X entries · oldest YYYY-MM-DD". Then "Clear History" reads as a destructive action with explicit scope.
- **Feeds into:** standalone ticket

### [P2] Source filter ("All / Docker / Systemd") is one row of pills, identical to alerts severity filter — should be a typed select
- **Location:** `web/templates/history.html:22-36`
- **Category:** pickers
- **Issue:** With three options, pills are fine. But the same pattern is used on alerts (4 options) and could plausibly grow to 5+ (network, backup, system). Inconsistent: dashboard timeline uses chips, alerts severity uses chips, history source uses chips. Settings would also benefit. There is no shared component.
- **Recommendation:** Standardise on one filter component pattern. For ≤4 stable options keep chips; for ≥5 or growing, switch to a select. Today the same chip pattern is reimplemented per page.
- **Feeds into:** linked to BL-020 settings-revamp

### [P3] Pagination controls are minimal and lopsided
- **Location:** `web/templates/history.html:80-89`
- **Category:** hierarchy
- **Issue:** "← Newer" and "Older →" with `flex justify-between`. When `Page == 0` an empty `<span>` holds the left slot and "Older →" sits alone on the right. There is no page indicator (e.g. "page 3 of N") and no jump-to-first.
- **Recommendation:** Add a centred page indicator. Keep the same flex frame but with three slots (prev / position / next).
- **Feeds into:** standalone ticket

### [P2] History row layout collides with alert row layout despite carrying nearly identical data
- **Location:** `web/templates/history.html:39-77` vs `web/templates/partials/alerts-list.html:6-55`
- **Category:** layout
- **Issue:** Both pages render lists of "icon + meta strip + body + timestamp" but with different markup, different status indicators (success/error icons vs severity dots), different timestamp positioning. Maintaining two divergent renderers for nearly-isomorphic data.
- **Recommendation:** Extract a shared `event-row` partial used by alerts, history, and recent WAN events. Currently `network.html:32-44` is yet a third variant of the same idea.
- **Feeds into:** standalone ticket

---

## Logs

### [P1] "Source" select is the only input, but the page does not pre-fill from URL or remember last choice
- **Location:** `web/templates/logs.html:10-17`
- **Category:** pickers
- **Issue:** Select uses options injected server-side. There's no time-range picker (logs are always "last 100 lines" implicit — see `docker-detail.html:20`), no severity filter, no follow/tail toggle. For a log viewer this is sparse.
- **Recommendation:** Add: lines-to-fetch numeric input (with sensible defaults: 50 / 100 / 500 / 1000), follow toggle, severity quick-filter. Keep the source picker.
- **Feeds into:** standalone ticket

### [P2] Mac-window-traffic-light decoration is purely cosmetic and steals header space
- **Location:** `web/templates/logs.html:31-38`
- **Category:** hierarchy
- **Issue:** The three coloured dots (red/yellow/green) at the top of the log pane mimic a macOS window chrome. They are non-interactive. The status text ("Idle / Fetching... / Success") sits to their right at 10px. The status is the load-bearing element; the dots are decoration that takes prime real estate.
- **Recommendation:** Drop the dots; promote the status to `text-xs` left-aligned with a small spinner/check icon for state. Use the regained space for a copy-to-clipboard button and a fetch-time stamp.
- **Feeds into:** standalone ticket

### [P3] `min-h-[500px]` is hardcoded — unhandled on tall viewports
- **Location:** `web/templates/logs.html:30`
- **Category:** responsive
- **Issue:** Log pane has fixed `min-h-[500px]`, no `flex-1` to stretch to viewport. On a 1440-tall screen, half the page is wasted whitespace; on a 600-tall mobile viewport, the log pane is taller than the viewport and you scroll past everything else.
- **Recommendation:** Make the log pane fill remaining vertical space on this page (`flex-1` inside the main scroll container) or use `max-h: calc(100vh - …)`.
- **Feeds into:** standalone ticket

---

## Settings

> See also `backlog/settings-page-ui-improvements/backlog.md` (existing seed). Findings overlap where noted.

### [P0] All numeric "interval / threshold / time" inputs are raw `<input type="number">`
- **Location:** `web/templates/settings.html:71-72, 83-84, 188-215, 246-261, 274-280`
- **Category:** pickers
- **Issue:** Eleven separate numeric inputs across Alert Rules, Performance, Backup, and Schedule. None use a typed picker, none surface units inline as part of an input group (units are floating spans), schedule hour/minute is two adjacent number inputs (no clock picker), schedule cadence is a select but the actual time anchor is two free-text numerics. This is the highest-value pickers fix in the entire app.
- **Recommendation:** Replace with: a stepper component for "interval N units" (number + unit dropdown — sec/min/hr); a `<input type="time">` for schedule hour/minute (single picker, server splits); pre-set chip rows for common cooldowns (1m / 5m / 15m / 1h) with a "custom" escape hatch. Threshold for alert rules also benefits from a slider+number combined widget.
- **Feeds into:** BL-020 settings-revamp

### [P1] Settings page enforces a single accordion globally via JS, but anchor-chip nav at top still scrolls to closed sections
- **Location:** `web/templates/settings.html:9-19, 417-475`
- **Category:** layout
- **Issue:** The chip strip at the top (`#settings-alerts`, `#settings-telegram`, …) is hash anchors. The accordion JS auto-closes every section except the one being opened (lines 469-473). If the user clicks a chip while another section is expanded, the page scrolls but the target accordion remains collapsed unless the click handler also fires — and it does not (chips are anchor `<a>` tags).
- **Recommendation:** Wire the chip-strip clicks to also expand the matching accordion, or convert the chip strip into a tab control rather than anchor links. Today the chips look like nav but only do half the job.
- **Feeds into:** BL-020 settings-revamp

### [P1] Numeric ranges are encoded only as `min`/`max` HTML attributes — no visible hint
- **Location:** `web/templates/settings.html:191, 198, 205, 212, 248, 252, 256, 260, 275, 279`
- **Category:** pickers
- **Issue:** SSE interval `min=2 max=60`, Disk `min=1 max=1440`, Docker `min=5 max=300`, Schedule hour `min=0 max=23`, etc. None of these ranges are displayed in the UI — the user only learns by trial and error (or by reading the inline error mapping in JS at lines 538-541). A typed picker or a labelled "(2-60 sec)" suffix would prevent the round-trip failure.
- **Recommendation:** Render the allowed range inline next to the label: `Dashboard refresh (2-60 sec)`. Or use a stepper with hard min/max bounds visible.
- **Feeds into:** BL-020 settings-revamp

### [P1] Severity is a `<select>` instead of a typed segmented control
- **Location:** `web/templates/settings.html:75-80`
- **Category:** pickers
- **Issue:** Severity has 3 fixed values (critical / warning / info) and is the most semantically loaded field on the alert-rules form. A select hides options behind a click and shows no colour preview. Same applies to the email/telegram "Enabled" checkboxes which could be toggle switches given they gate side-effecting integrations.
- **Recommendation:** Segmented 3-button picker for severity with the actual severity colour applied to each segment (matches the alert card colour). Convert "Enabled" checkboxes to clearly-labelled toggle switches.
- **Feeds into:** BL-020 settings-revamp

### [P1] Encryption key reference is a free-text input with placeholder copy as the only docs
- **Location:** `web/templates/settings.html:294-297`
- **Category:** pickers
- **Issue:** Field accepts `env:ULTRON_BACKUP_KEY` or `kms://...`. There is no scheme-picker, no validation hint, no "Show resolved key fingerprint" feedback. A typo silently breaks backups.
- **Recommendation:** Two-field composite: scheme select (`env` / `kms` / `file`) + value input. Display "Resolved at runtime: ✓ key found" or "✗ env var not set" as live verification (re-using the test-button pattern already on telegram/email).
- **Feeds into:** standalone ticket

### [P1] Backup config fits 14 fields into a single section with three different `grid-cols-2` rows
- **Location:** `web/templates/settings.html:235-298`
- **Category:** hierarchy
- **Issue:** Section contains: 2 enable/encrypt checkboxes, 4 numerics (interval/retention/timeout/maxsize), 3 schedule fields (mode/hour/minute), 3 destination fields (mode/local-path/key-ref). That's 12+ inputs in three different `grid-cols-2` blocks separated only by white margin. No sub-headings ("Schedule", "Destination", "Limits"). The visual reads as a wall of inputs.
- **Recommendation:** Sub-divide with three labelled sub-groups inside the section: Limits / Schedule / Destination. Use the same uppercase-eyebrow heading style as the section heading itself.
- **Feeds into:** BL-020 settings-revamp

### [P2] "System Controls" mixes destructive actions with a Logout button in one grid
- **Location:** `web/templates/settings.html:328-360`
- **Category:** hierarchy
- **Issue:** Logout, Restart device, Shutdown device share the same `grid-cols-2` and similar visual weight. Logout is benign, Shutdown is catastrophic; they should not be peers. Restart and Shutdown both gate through the same danger guard, but Logout's "Run" button looks identical to Restart's "Run" button.
- **Recommendation:** Move Logout out of System Controls (it belongs in the header dropdown or a separate "Session" row). Keep Restart/Shutdown together with stronger visual separation: red-tinted card border on Shutdown only, neutral on Restart.
- **Feeds into:** BL-020 settings-revamp

### [P2] "Clear alerts" and "Clear history" appear here in addition to their pages
- **Location:** `web/templates/settings.html:392-409` (settings) vs `web/templates/alerts.html:11-27` (alerts page) and `web/templates/history.html:7-19` (history page)
- **Category:** duplication
- **Issue:** Both clear actions are reachable in three places: the settings danger zone, the page-level toolbar on alerts, and the page-level toolbar on history. Inconsistent — some destructive actions are page-local (clear alerts on /alerts), others are settings-global (clear history on /settings).
- **Recommendation:** Decide one canonical home. Either keep them on their page only (drop from settings), or centralise in settings only (drop from page toolbars). Mixed locations means muscle-memory will fail at least 30% of the time.
- **Feeds into:** standalone ticket

### [P2] Section number badges (`01`, `02`, `03`...) add ordering signal that is not respected by anchors
- **Location:** `web/templates/settings.html:25, 98, 131, 180, 228, 309, 330`
- **Category:** layout
- **Issue:** Each section header carries a numbered pill. The order is hard-coded but the anchor chips at the top are also static. Nothing dynamic. The numbers imply a workflow ("step 1 → 2 → ...") but settings is a navigation surface, not a wizard.
- **Recommendation:** Drop the numbers, or make them genuine progressive disclosure (only render once the previous section is configured). Today they are decoration that implies a workflow that does not exist.
- **Feeds into:** BL-020 settings-revamp

### [P2] Settings page header includes Spanish copy
- **Location:** `web/templates/settings.html:7`
- **Category:** hierarchy
- **Issue:** "Ajusta alertas, notificaciones, rendimiento y mantenimiento con enfoque seguro para servidor." — Spanish copy on an otherwise English UI. Sidebar tooltip on `web/templates/partials/sidebar.html:113` is also Spanish ("Expandir/contraer barra lateral"). Mixed locale makes the app feel half-finished.
- **Recommendation:** English-only or i18n. Pick one — probably English since every other label is English.
- **Feeds into:** standalone ticket

### [P3] "Form-state pill" idle/saving/applied/failed is a 5th visual axis competing with section borders
- **Location:** `web/templates/settings.html:28, 101, 134, 183, 231` and JS `497-511`
- **Category:** hierarchy
- **Issue:** Each section header carries: number pill + section title + form-state pill (right-aligned). Three chip-shaped tokens of differing semantics. The form-state pill barely changes (most users see it `idle` permanently) so it adds clutter at rest while being invisible during the moments it matters.
- **Recommendation:** Show the state pill only when state ≠ idle; reuse the same status target for all transitions.
- **Feeds into:** BL-020 settings-revamp

### [P3] `max-w-4xl` inside `max-w-5xl` shell wastes ~80px on the right of forms
- **Location:** `web/templates/settings.html:2, 44, 103, 136, 185, 233, 312, 334`
- **Category:** layout
- **Issue:** `#settings-shell` is `max-w-5xl mx-auto` (~1024px). Every form inside is then `max-w-4xl` (~896px), so forms are 128px narrower than their container. The padding looks intentional from the right but unintentional from the left (forms left-align inside the shell).
- **Recommendation:** Drop the inner `max-w-4xl` or apply `mx-auto` to forms so the asymmetry isn't visible.
- **Feeds into:** BL-020 settings-revamp

---

## Login

### [P3] Login page is the only screen that hard-codes pixel sizes
- **Location:** `web/templates/login.html:14, 12`
- **Category:** layout
- **Issue:** `style="width:100px;height:100px"` on the logo image and `max-w-[360px]` on the card. The header uses inline `style` for centring (`partials/header.html:17`) too. Inline styles bypass design tokens.
- **Recommendation:** Move to Tailwind classes (`w-24 h-24`). Cosmetic but a footgun for theming.
- **Feeds into:** standalone ticket

---

## Cross-page patterns

### [P1] Container width is inconsistent across pages
- **Location:** dashboard/network/services/docker/alerts/history/logs use `<main>`-default full width (`web/templates/base.html:24` → `flex-1 p-4 md:p-6`); settings uses `max-w-5xl mx-auto` (`web/templates/settings.html:2`); login is its own thing.
- **Category:** layout
- **Issue:** Settings is the only content page that constrains its width. On a 1920px monitor, dashboard sprawls edge-to-edge while settings stops at ~1024px, and the user has to recalibrate spatial expectation when navigating between them.
- **Recommendation:** Pick one max-width policy. Either constrain all pages with `max-w-7xl mx-auto` for consistency, or remove the constraint from settings. Mixed is the worst option.
- **Feeds into:** standalone ticket

### [P1] Three different list-row patterns for nearly identical data shapes
- **Location:** `web/templates/partials/alerts-list.html:6-55` (alerts), `web/templates/history.html:39-77` (history), `web/templates/network.html:32-44` (WAN events)
- **Category:** layout
- **Issue:** All three render "icon + chip + label + timestamp + optional details". All three use slightly different markup, padding, gap sizes, and timestamp placement. Maintenance burden is 3× and the pages don't feel related.
- **Recommendation:** Extract a single `event-row` partial. Even if the data sources differ, the visual contract should be one component.
- **Feeds into:** standalone ticket

### [P1] Filter-chip strip is reimplemented per page
- **Location:** `web/templates/dashboard.html:10-15` (timeline), `web/templates/alerts.html:32-47` (severity), `web/templates/history.html:24-35` (source), `web/templates/settings.html:11-18` (anchor nav)
- **Category:** layout
- **Issue:** Four separate chip strips, four slightly different active-state treatments (active = bg-accent in some, bg-danger in alerts, etc.). The settings anchor strip looks identical to the others but does navigation, not filtering — semantic mismatch.
- **Recommendation:** Standardise a `chip-strip` partial with two flavours (filter vs nav) and a single active-state token. Apply uniformly. Settings-specific anchor behaviour should look distinguishable from filter chips.
- **Feeds into:** standalone ticket

### [P2] Empty-state pattern duplicated five times with cosmetic divergence
- **Location:** `services-list.html:94-100`, `docker-list.html:83-89`, `alerts-list.html:57-64`, `history.html:91-98`, plus inline empty messages on dashboard/network
- **Category:** duplication
- **Issue:** Each empty state has its own SVG, its own padding (`py-12`), its own copy. The container/services/history empty states look ~identical but ship three SVGs.
- **Recommendation:** Single `empty-state` partial with `{icon, title, hint}` parameters. Trim the surface area.
- **Feeds into:** standalone ticket

### [P2] Confirmation prompts use raw browser `hx-confirm` for some destructive actions but a typed-confirmation guard for others
- **Location:** `services-list.html:67, 82` and `docker-list.html:56, 71` (browser confirm) vs `settings.html:344-389` (typed-word guard)
- **Category:** layout
- **Issue:** Restart device → typed confirmation + countdown. Restart container → native browser dialog ("Restart container foo?"). The asymmetry is jarring: container restart is arguably more frequent and just as service-affecting, but uses the cheaper guard. Browser-native dialogs also break visual brand and lose CSP context.
- **Recommendation:** Move all destructive confirmations to the in-app guard component (typed word optional for non-system actions, but consistent overlay component). At minimum, replace `hx-confirm` with a custom modal so visual style is consistent.
- **Feeds into:** standalone ticket

### [P2] Sidebar collapse toggle ships only on `md:` and up
- **Location:** `web/templates/partials/sidebar.html:112-121`
- **Category:** responsive
- **Issue:** The collapse-to-rail toggle is wrapped in `hidden md:flex`. On mobile the sidebar is overlay-only (controlled by hamburger). On tablet (768-1024) the toggle is shown but the sidebar is also `md:sidebar-expanded` by default — unclear when the rail mode is the right choice. Behaviour across breakpoints is inconsistent.
- **Recommendation:** Decide deliberately: rail mode is for tablet/laptop space-saving. Document the breakpoint contract: `<md` overlay, `md..lg` rail-default, `≥lg` expanded-default. Today it's all expanded-default which makes the toggle feel vestigial.
- **Feeds into:** standalone ticket

### [P3] Header (`partials/header.html`) is sparse — uptime is the only right-side widget
- **Location:** `web/templates/partials/header.html:20-30`
- **Category:** hierarchy
- **Issue:** Right side of the header has only `uptime` (hidden below `sm:`). No user menu, no theme toggle, no logout shortcut, no quick-action affordance. A 56px header used by ~1 widget.
- **Recommendation:** Either shrink the header to ~40px, or fill the right side with a user-menu (logout + settings shortcut), making logout reachable without the settings page.
- **Feeds into:** standalone ticket

### [P3] Asset versioning hardcoded per template
- **Location:** `web/templates/base.html:7-12`, `partials/sidebar.html:9`, `partials/header.html:17`, `login.html:7-9, 14`
- **Category:** layout
- **Issue:** Cache-bust query strings (`?v=20260226h`, `?v=20260227ux3`, `?v=20260304settings1`) are hand-edited per file. Not a layout bug but a maintenance footgun that has produced inconsistent versions in the same template.
- **Recommendation:** Centralise the version constant in the template data (e.g. `.AssetsVersion`) and reference once.
- **Feeds into:** standalone ticket

---

## Summary table

| Severity | Count |
|---|---|
| P0 | 1 |
| P1 | 13 |
| P2 | 14 |
| P3 | 11 |
| **Total** | **39** |

| Category | Count |
|---|---|
| layout | 13 |
| hierarchy | 11 |
| duplication | 6 |
| pickers | 6 |
| responsive | 5 |
| (info-only / "page is fine") | 1 |

Notes:
- 9 findings are tagged "Feeds into: BL-020 settings-revamp" — these align with the existing `settings-page-ui-improvements` seed and should be co-scheduled.
- 2 findings (services row reflow + docker row reflow) are the same bug rendered on two pages; close together.
- 3 findings (alerts list, history list, WAN-events list) share the same root cause and should be solved by extracting a single event-row partial.
