# UX/UI Specification — network-alerts

Archetype: **PRO-TECH/DASHBOARD** — reason: Ultron-AP is an authenticated Raspberry Pi monitoring/admin panel for system and network operations. The feature extends an existing dense settings and alerting surface, so it inherits the parent dark-first, high-density, semantic status-token UI with monospace treatment for numeric/network values and minimal chrome.

Scope: one existing screen, `/settings -> Alert rules`, plus the existing alert rules table partial. No new `/network` alert-management page, no new notification screen, and no new widget script family.

## User Flows

Persona: **Raspberry Pi Operator** (intermediate, single-admin). Goal: configure network alerts that page Telegram/email for real incidents without flapping.

### Flow A — Create a sustained latency alert

- **Entry point:** authenticated operator opens `/settings`, expands `Alert rules`, clicks the existing add-rule affordance.
- **Steps:**
  1. The form renders with `metric` first. Options appear in this exact order: `cpu`, `ram`, `disk`, `temp`, `latency`, `loss`, `dns_failure_rate`, `wan_outage`, `public_ip_change`.
  2. Operator selects `latency`.
  3. The form reveals `target`, `operator`, `threshold`, `sustained_duration`, `severity`, and `cooldown`.
  4. `target` is a server-rendered select populated from the FR-016 configured target list, such as `gateway`, `8.8.8.8`, `1.1.1.1`.
  5. Operator chooses `target=gateway`, `operator=>`, `threshold=100`, `sustained_duration=120`, `severity=warning`, and `cooldown=15m`.
  6. Operator submits. The submit button enters loading within 100 ms; the existing `/api/alerts/rules` endpoint returns the existing rules-table partial with the new row.
- **Exit point:** the new latency rule appears in the alert rules table with metric, target, threshold, sustained duration, severity, cooldown, enabled state, and row actions.
- **Error path:** if the target is missing or no longer in the configured target list, the target field shows `Choose a configured network target.` Submit remains blocked client-side where possible; server-side validation returns HTTP 400 and the form renders the same message beside `target`.

### Flow B — Create a sustained packet-loss alert

- **Entry point:** `/settings -> Alert rules`, add-rule form.
- **Steps:**
  1. Operator selects `metric=loss`.
  2. The target select appears and is required.
  3. Operator sets threshold as percentage, for example `5`, and sustained duration, for example `60`.
  4. Operator saves.
- **Exit point:** rules table row shows `loss`, selected target, threshold with `%`, sustained duration, severity, and cooldown.
- **Error path:** values outside 0-100 show `Packet loss threshold must be between 0 and 100%.` next to the threshold field. The row is not created until corrected.

### Flow C — Create a DNS failure-rate alert

- **Entry point:** `/settings -> Alert rules`, add-rule form.
- **Steps:**
  1. Operator selects `metric=dns_failure_rate`.
  2. The form hides `target`; DNS rules cover all configured resolvers.
  3. The form keeps `operator`, `threshold`, `sustained_duration`, `severity`, and `cooldown` visible.
  4. Operator saves a rule such as `threshold=20`, `sustained_duration=120`, `severity=warning`.
- **Exit point:** row appears as `dns_failure_rate`, target rendered as `-`, threshold rendered as `%`, sustained duration shown.
- **Error path:** if DNS probes are disabled or fewer than two DNS samples are available at evaluation time, the UI does not block rule creation. The rule row remains enabled; runtime skip reason is handled by observability logs, not a form error.

### Flow D — Create a WAN outage alert

- **Entry point:** `/settings -> Alert rules`, add-rule form.
- **Steps:**
  1. Operator selects `metric=wan_outage`.
  2. The form hides `target`, `operator`, `threshold`, and `sustained_duration`.
  3. Severity is visible but locked to `critical`; cooldown remains editable.
  4. Operator saves.
- **Exit point:** row appears as `wan_outage`, severity `critical`, no threshold, no target, cooldown visible.
- **Error path:** if the operator attempts to submit hidden threshold/operator fields via a stale client or crafted request, the server ignores irrelevant hidden fields and validates the rule shape. Invalid severity returns `WAN outage alerts use critical severity.`

### Flow E — Create a public-IP change alert

- **Entry point:** `/settings -> Alert rules`, add-rule form.
- **Steps:**
  1. Operator selects `metric=public_ip_change`.
  2. The form hides `target`, `operator`, `threshold`, and `sustained_duration`.
  3. Severity is locked to `info`; cooldown defaults to 60 minutes and remains editable.
  4. Operator saves.
- **Exit point:** row appears as `public_ip_change`, severity `info`, cooldown visible.
- **Error path:** invalid severity returns `Public IP change alerts use info severity.` The form preserves the selected metric and cooldown value so the user can retry.

### Flow F — Add sustained duration to a host rule

- **Entry point:** `/settings -> Alert rules`, existing host metric rule form or edit row.
- **Steps:**
  1. Operator selects or edits `cpu`, `ram`, `disk`, or `temp`.
  2. `sustained_duration` stepper is visible with `data-min=0`, `data-max=3600`, default `0`.
  3. Operator sets `300` seconds for a known flapping rule.
  4. Operator saves.
- **Exit point:** existing host alert semantics remain unchanged if value is `0`; if value is greater than `0`, row displays the sustained duration.
- **Error path:** values below 0 or above 3600 show `Sustained duration must be 0-3600 seconds.` beside the stepper.

### Flow G — View, disable, or delete a network alert rule

- **Entry point:** `/settings -> Alert rules`, existing alert rules table.
- **Steps:**
  1. Operator scans rows. Network rows are mixed with host rows in the same table.
  2. Target-scoped rows show the target inline. Transition rows show `-` for threshold and sustained duration.
  3. Operator toggles enabled state or deletes a row using existing row actions.
- **Exit point:** table partial updates in place.
- **Error path:** failed toggle/delete shows an inline table-level error: `Could not update alert rule. Retry.` The previous row state remains visible.

### Responsive Behavior

- **375px:** form fields stack in a single column. Metric select is full width. Target, threshold, and sustained-duration fields appear directly below metric when relevant. Table becomes compact rows with primary row content first: metric, target, severity, enabled. Secondary values wrap below; actions are icon buttons with accessible labels.
- **768px:** two-column form grid. Metric and target share the first row when target is visible. Transition-rule forms use a shorter two-column row with severity and cooldown.
- **1440px:** existing settings shell width is used. Alert rules table uses full columns: metric, target, operator, threshold, sustained duration, severity, cooldown, enabled, actions. No nested cards.

## Component Inventory

### Screen: `/settings -> Alert rules`

| Component | States (default/loading/error/empty/disabled) | Behavior | Nielsen heuristics applied |
|---|---|---|---|
| `MetricSelect` | default: current metric selected with all 9 options in required order; loading: disabled during form submit; error: inline message `Choose a supported alert metric.`; empty: defaults to `cpu`; disabled: greyed when editing is blocked | Controls progressive disclosure for rule shape. `latency`/`loss` show target. `wan_outage`/`public_ip_change` hide threshold-style fields. | H1, H4, H5, H6 |
| `TargetSelect` | default: visible and required for `latency`/`loss`, populated server-side; loading: disabled during submit; error: inline invalid-target message; empty: if no targets are configured, visible disabled with `No configured network targets`; disabled: hidden for non-target metrics or greyed when no targets exist | Never fetches client-side. Uses `name="target"`. Unknown values are rejected by the server whitelist. | H1, H5, H6, H9 |
| `OperatorSelect` | default: visible for threshold-style rules; loading: disabled during submit; error: inline invalid-operator message; empty: defaults to `>`; disabled: hidden for transition rules | Uses existing host-rule operator behavior. | H4, H5, H6 |
| `ThresholdInput` | default: visible for `cpu`, `ram`, `disk`, `temp`, `latency`, `loss`, `dns_failure_rate`; loading: disabled during submit; error: range/unit-specific inline message; empty: empty value blocks submit; disabled: hidden for transition rules | Unit hint changes by metric: `%` for `cpu`/`ram`/`disk`/`loss`/`dns_failure_rate`, `C` for `temp`, `ms` for `latency`. | H2, H5, H9 |
| `SustainedDurationStepper` | default: visible for threshold-style rules, value default `0`, `data-min=0`, `data-max=3600`; loading: buttons/input disabled; error: inline `0-3600 seconds`; empty: defaults to `0`; disabled: hidden for transition rules | Reuses FR-057 stepper widget. `0` means fire immediately. Values greater than `0` mean all samples in the trailing window must breach. | H1, H5, H6, H10 |
| `SeveritySegmentedControl` | default: visible for all rules; loading: disabled during submit; error: inline invalid severity message; empty: defaults by metric; disabled: locked for `wan_outage=critical` and `public_ip_change=info` | Uses existing severity colors. Locked state remains visible so the operator understands the rule severity. | H1, H4, H5, H6 |
| `CooldownControl` | default: visible for all metrics, existing chip-preset/custom pattern; loading: disabled during submit; error: inline range message; empty: default 15m except `public_ip_change=60m`; disabled: greyed during submit | Per-rule cooldown remains editable for every metric. | H1, H5, H7 |
| `RuleSubmitButton` | default: enabled when visible fields validate; loading: label `Saving...`; error: re-enabled after failed save; empty: disabled until required fields filled; disabled: disabled when no valid target exists for target-scoped metrics | Posts to existing `/api/alerts/rules`; response swaps the same table partial. | H1, H5, H7, H9 |
| `FormFeedback` | default: not rendered; loading: `Saving rule...`; error: specific server error; empty: not rendered; disabled: not rendered | Appears within 100 ms of submit and clears after success or next edit. | H1, H9 |
| `AlertRulesTable` | default: rows for host and network rules; loading: skeleton rows during htmx refresh; error: table-level retry message; empty: `No alert rules configured.` plus add-rule affordance; disabled: row actions disabled during active request | Same partial used today. Network-specific cells render `-` for not-applicable fields. | H1, H4, H6, H7 |
| `RuleRowActions` | default: toggle/delete buttons; loading: clicked action disabled and row shows pending state; error: row-level message; empty: n/a; disabled: buttons disabled when request in flight | Existing action semantics. Delete keeps existing confirmation. | H1, H3, H4, H5 |

### Alert Rules Table Column Contract

| Column | Host threshold rules | Latency/loss | DNS failure rate | WAN outage | Public IP change |
|---|---|---|---|---|---|
| Metric | `cpu`/`ram`/`disk`/`temp` | `latency`/`loss` | `dns_failure_rate` | `wan_outage` | `public_ip_change` |
| Target | `-` | configured target | `-` | `-` | `-` |
| Operator | selected operator | selected operator | selected operator | `-` | `-` |
| Threshold | value + unit | value + unit | value + `%` | `-` | `-` |
| Sustained | seconds, default `0` | seconds | seconds | `-` | `-` |
| Severity | editable | editable | editable | locked `critical` | locked `info` |
| Cooldown | editable | editable | editable | editable | editable, default 60m |

## Nielsen Compliance

### `/settings -> Alert rules`

- **H1 Visibility of system status:** form feedback appears during save; row-level pending states show which action is running; locked severities remain visible instead of disappearing.
- **H2 Match between system and real world:** threshold units use operator language: milliseconds for latency, percent for loss/DNS failure rate, seconds for sustained duration.
- **H3 User control and freedom:** delete keeps the existing confirmation; failed saves preserve entered values; changing metric immediately reconfigures visible fields.
- **H4 Consistency and standards:** all controls reuse existing settings widgets and the existing alert rules table partial. Network rules live alongside host rules.
- **H5 Error prevention:** metric choice hides irrelevant fields; target values are selected from a whitelist; locked severities prevent invalid transition-rule shapes.
- **H6 Recognition rather than recall:** metric options are visible in a select; units and range hints are rendered beside fields; not-applicable table cells use `-`.
- **H7 Flexibility and efficiency of use:** cooldown presets remain available; keyboard users can tab through the form in the same order as visual layout.
- **H8 Aesthetic and minimalist design:** the form progressively discloses only fields relevant to the selected metric; no separate network alerts page duplicates the same job.
- **H9 Help users recognize, diagnose, and recover from errors:** validation messages are field-specific and explain the fix. Server errors are rendered in the form/table context.
- **H10 Help and documentation:** `sustained_duration=0` is described inline as `0 = fire immediately`; nonzero values are described as `require continuous breach`.

Trade-offs:
- DNS probe availability is not validated at creation time. This avoids blocking future-valid rules when probes are temporarily disabled; runtime skip reasons are logged by the engine.
- Transition rules hide threshold fields instead of showing disabled controls. The locked severity and cooldown remain visible, preserving enough context without visual noise.

Nielsen compliance: **10/10 heuristics applied**. Violations found: **0**. Corrected during design: **1** — transition rules originally showed disabled threshold/operator fields, changed to hide them to reduce false affordances. Accepted trade-offs: **2**.

## Design Tokens

The feature inherits the parent Ultron-AP dark mode token system and the settings-revamp widget language. No new aesthetic primitives are introduced. Tokens below are the implementation contract for this feature.

### Color Roles

| Role | Hex | Usage | Reason |
|---|---|---|---|
| Background | `#0b0c0f` | Page background | Existing dark graphite parent base; appropriate for a monitoring panel used at night. |
| Surface | `#121418` | Settings panels, form surface | Existing elevated surface; separates forms from background without decorative cards. |
| Primary | `#c2c7d0` | Primary action text, active controls | Existing muted primary/accent treatment; avoids noisy colors in dense ops UI. |
| Accent | `#58a6ff` | Info severity, links, focus accent where parent uses blue | Blue maps to informational alert events such as public-IP change. |
| Error | `#ff6b6b` | Inline validation text, failed feedback | Existing high-contrast error text on dark background. |
| Text primary | `#e5e7eb` | Labels, row values, body text | Contrast above AA on the page background. |
| Text secondary | `#9ca3af` | Hints, units, secondary table cells | Still readable while subordinate to primary values. |
| Border | `#2a2f37` | Inputs, table rows, segmented controls | Existing structure token, lower contrast than content. |
| Status critical | `#e34b6a` | Critical severity, WAN outage locked severity | Matches parent critical alert language. |
| Status warning | `#facc15` | Warning severity | Matches parent warning alert language. |
| Status info | `#58a6ff` | Info severity, public IP change | Differentiates non-urgent informational events. |
| Disabled | `#6b7280` | Disabled field text/icons | Signals unavailable controls while staying legible. |

Contrast confirmation:
- `#e5e7eb` on `#0b0c0f`: passes WCAG AA for body text.
- `#9ca3af` on `#0b0c0f`: passes WCAG AA for body text.
- `#ff6b6b` on `#0b0c0f`: passes WCAG AA for body text.
- `#c2c7d0` on `#0b0c0f`: passes WCAG AA for body text.
- `#e34b6a` on `#0b0c0f`: meets AA for alert labels and UI components.

All roles meet or exceed 4.5:1 for body-size text where used as text. Warning yellow is used with dark text only when placed on filled yellow backgrounds; as foreground text it must be paired with existing parent contrast-safe treatment.

### Type Scale

| Role | Family | Size / Weight | Reason |
|---|---|---|---|
| Body and labels | `Space Grotesk`, system sans fallback | 14px / 400 | Inherits parent settings UI; compact but readable. |
| Form section title | `Space Grotesk`, system sans fallback | 16px / 600 | Matches existing settings section hierarchy. |
| Field hint and validation support text | `Space Grotesk`, system sans fallback | 12px / 400 | Keeps units/range hints close to inputs without competing with labels. |
| Numeric values | `Space Grotesk` with `font-variant-numeric: tabular-nums` where available | 14px / 500 | Stable width for thresholds, cooldowns, and durations. |
| Table row metric/target values | `Space Grotesk`, system sans fallback | 14px / 500 | Keeps rows scannable at desktop and mobile. |
| Code-like network target values | system monospace fallback only when target is IP literal | 13px / 500 | IP addresses align and remain distinguishable from labels. |

### Spacing Scale

All spacing uses the existing Tailwind 4px scale.

- Form field gap: `16px` desktop/tablet, `12px` mobile.
- Control minimum touch target: `44px` height and width for buttons, row actions, and stepper controls.
- Input padding: `12px` horizontal, `10px` vertical.
- Inline validation margin-top: `4px`.
- Form feedback margin-top: `12px`.
- Table row padding: `12px` desktop/tablet, `10px` mobile compact rows.
- Mobile stacked row gap: `6px` between secondary values.
- No card-in-card layout; the alert rules form and table remain direct children of the existing settings section.

### Responsive Breakpoints

| Width | Behavior |
|---|---|
| `375px` | Single-column form; full-width selects and inputs; table rows stack into compact summaries; actions are icon buttons with accessible labels; no text overlaps inside buttons. |
| `768px` | Two-column form grid; table shows more columns but may wrap secondary values; target and threshold fields align on the same row where possible. |
| `1440px` | Existing settings max-width; full alert rules table columns visible; no horizontal scroll unless user-created target strings exceed normal IP/hostname length, in which case target cell truncates with title tooltip. |

### State Tokens and Interaction Timing

- Loading feedback appears within 100 ms for submit/toggle/delete.
- Successful save feedback persists for 1.5-4 seconds, then clears.
- Error feedback persists until the next user edit or retry.
- Disabled controls use reduced opacity plus `aria-disabled`/`disabled`, never color alone.
- Focus states use the existing parent focus ring; all interactive controls must be keyboard reachable.
