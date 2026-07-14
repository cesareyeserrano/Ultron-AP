# UX Spec — ac-coverage-gaps

Scope: FR-079 (Telegram mute window), FR-080 (daily email digest), FR-081 (per-service log drawer), FR-082 (fan-mode selector), FR-083 (OLED configuration).

This feature adds no new screen. Everything lands on two existing surfaces: `/settings` (mute control, digest fields, hardware section) and `/services` (log drawer). The design authority is the **existing product**: the settings revamp (FR-057..FR-070) already fixed the vocabulary — accordion sections, widget controls (segmented / toggle / chip-preset / stepper), a form-state pill, a status line, and a controller in `web/static/js/settings.js`. Every component below reuses that vocabulary rather than inventing one, because a second vocabulary on the same page is the regression NFR-087 exists to prevent.

---

## User Flows

Persona: **Pi Admin (sole operator, high tech level)** — the only persona in Phase 1.

### Flow A — Mute Telegram during an incident (FR-079)

- **Entry point:** admin is being paged repeatedly for an outage they are already handling. They open `/settings` (sidebar → Settings) and expand the **Telegram** section, or land directly on `#settings-telegram` via the anchor chip.
- **Steps:**
  1. The Telegram section shows a **Mute alerts** row with a chip-preset group: `1h` · `4h` · `24h`.
  2. Admin clicks `4h`.
  3. The row switches to its muted state: `Muted — 3h 59m left` plus a **Cancel** button. The form-state pill shows `applied`; the status line reads "Telegram alerts muted for 4h".
  4. Alerts keep firing and keep appearing in the Alerts panel and the header badge — only Telegram delivery stops. This is stated in the section's help text so the admin is never left wondering whether monitoring stopped.
- **Exit point:** the window expires on its own, or the admin clicks **Cancel** and the row returns to the chip-preset state.
- **Error path:** a failed save leaves the previous mute state visible and unchanged, sets the pill to `failed`, and the status line offers **Retry** — identical to every other settings form (FR-065/FR-066). If the stored mute row is unreadable, the system treats it as *not muted* (fail-open, NFR-090) so an alert is never silently swallowed; the row renders the chip-preset state.

### Flow B — Enable the daily digest (FR-080)

- **Entry point:** `/settings` → **Email** section (the digest belongs to the channel it is sent over; a separate section would orphan it).
- **Steps:**
  1. Below the SMTP fields, a **Daily digest** row: a toggle plus an hour stepper (`0–23`, rendered as `08:00`).
  2. Admin enables the toggle. The hour stepper becomes enabled (it is `disabled` while the digest is off — the control shows the dependency instead of accepting input that would do nothing).
  3. Admin sets `08`, saves. Pill → `applied`; the status line reads "Daily digest at 08:00".
- **Exit point:** the digest is enabled; at 08:00 one email arrives.
- **Error path:** an hour outside 0–23 is rejected server-side with an inline field error on the hour input (the existing `applyFieldErrorFromResponse` path, FR-066). A failed send does not surface in the UI at send time — it is recorded in `action_history` (NFR-091), visible on `/history`, which is where the admin already looks for "did the box do the thing".

### Flow C — Read a failing unit's logs in place (FR-081)

- **Entry point:** `/services`, a unit row showing `failed` (red indicator).
- **Steps:**
  1. The row carries a **Logs** button next to Start/Stop/Restart.
  2. Admin clicks it. The button enters its loading state; the drawer expands below the row showing a skeleton/"Loading logs…" line.
  3. The last 100 journalctl lines render in a monospace, scrollable panel (`max-h-96 overflow-y-auto`), oldest→newest, with the newest line in view (the drawer scrolls to the bottom on open — the reason you open logs on a failed unit is to read what just happened).
  4. Clicking **Logs** again collapses the drawer.
- **Exit point:** admin has read the tail and acts (restart the unit) or collapses the drawer.
- **Error paths:**
  - Helper unavailable → the drawer renders an explicit error state: "Could not read logs — the privileged helper is unavailable." (AC-081-004). Never an empty panel: an empty panel is indistinguishable from "this unit has no logs", which is a different fact.
  - Unit has no journal entries → an explicit empty state: "No log entries for this unit."
  - Session expired → the boosted request redirects to `/login` (AC-081-005).

### Flow D — Configure the hardware section (FR-082, FR-083)

- **Entry point:** `/settings` → new **Hardware** section (anchor chip `Hardware`, positioned between Performance and Backup: it is a device-level concern, like performance, and unlike the notification channels above it).
- **Steps:**
  1. **Fan mode** — a segmented control with exactly four options: `Auto` · `Quiet` · `Performance` · `Off`.
  2. **OLED display** — a toggle (`enabled`), plus a segmented control for the metric: `CPU` · `RAM` · `Temp` · `IP`. The metric control is `disabled` while the OLED toggle is off.
  3. Admin saves. Pill → `applied`.
- **Exit point:** values persist and re-render on the next page load.
- **Error path:** an out-of-range mode/metric is rejected with HTTP 400 and an inline field error; the stored value is unchanged (AC-082-003, AC-083-003).
- **Honesty requirement (owner constraint):** the section carries a visible note — *"Ultron stores these settings; it does not drive the fan or OLED panel yet."* The admin must not believe the fan is being controlled when it is not. This is Nielsen #1 (visibility of system status) applied to a deliberate scope boundary, and it is the UX consequence of the owner's decision that hardware control stay out (it previously cost the Pi significant CPU and memory).

---

## Component Inventory

### Screen: `/settings` — Telegram section (FR-079)

| Component | States | Behavior | Nielsen heuristics applied |
|---|---|---|---|
| Mute chip-preset (`1h` / `4h` / `24h`) | default · hover · focus-visible · active(selected) · disabled(while saving) | `data-widget="chip-preset"` — sets a hidden `mute_hours` input; submits with the Telegram form. Rendered only when no window is open. | #6 Recognition over recall (durations are shown, not typed); #3 User control (a mute is a reversible choice) |
| Mute status row (`Muted — 3h 59m left`) | muted · (absent when not muted) | Server-rendered from `expires_at`; the remaining time is computed at render. Replaces the chip group while a window is open. | #1 Visibility of system status |
| Cancel mute button | default · hover · focus-visible · loading · disabled | `hx-post` to clear the window; swaps the row back to the chip-preset state. | #3 User control and freedom (an obvious exit from the muted state) |
| Mute help text | static | "Alerts keep firing and keep appearing here — only Telegram delivery pauses." | #1 Visibility; prevents the mute being misread as "monitoring off" |
| Form-state pill | idle(absent) · saving · applied · failed | Existing `[data-form-state-host="telegram"]` — no new mechanism. | #1 Visibility of system status |

### Screen: `/settings` — Email section (FR-080)

| Component | States | Behavior | Nielsen heuristics applied |
|---|---|---|---|
| Digest toggle | on · off · focus-visible · disabled(saving) | `data-widget="toggle"`, field `digest_enabled`. Gates the hour stepper. | #4 Consistency (same toggle as every other on/off in Settings) |
| Digest hour stepper (`0–23`) | default · focus-visible · at-min · at-max · disabled(digest off) · error | `data-widget="stepper"`, field `digest_hour`, `min=0 max=23`, rendered as `HH:00`. Disabled while the digest is off — the control expresses the dependency. | #5 Error prevention (bounds enforced in the control, not just server-side); #8 Minimalist design |
| Inline field error | absent · present | Existing `[data-field-error]` path — server 400 maps to the hour input. | #9 Help users recognize and recover from errors |

### Screen: `/services` — service row (FR-081)

| Component | States | Behavior | Nielsen heuristics applied |
|---|---|---|---|
| **Logs** button | default · hover · focus-visible · loading · expanded(pressed) · disabled(during fetch) | `hx-get="/api/services/{name}/logs"`, `hx-target` the row's drawer, `hx-swap="innerHTML"`. `aria-expanded` reflects the drawer. ≥44×44px touch target (AC-009-004 applies here too). | #1 Visibility (loading state); #4 Consistency (same control shape as Start/Stop/Restart) |
| Log drawer | collapsed(default) · loading · loaded · empty · error | Collapsed on page load — **no fetch happens until the admin opens it** (AC-081-005). Monospace, `max-h-96 overflow-y-auto`, scrolled to the newest line on open. Content is HTML-escaped and passes logfilter redaction. | #1 Visibility; #8 Minimalist design (100 lines, not a firehose) |
| Drawer loading state | "Loading logs…" | htmx request in flight (`hx-indicator`). | #1 Visibility of system status |
| Drawer empty state | "No log entries for this unit." | Distinct from the error state — an empty journal is a fact, not a failure. | #9 Recognize/diagnose |
| Drawer error state | "Could not read logs — the privileged helper is unavailable." | Explicit message, never an empty panel. | #9 Help users recognize, diagnose, and recover from errors |

### Screen: `/settings` — Hardware section (FR-082, FR-083)

| Component | States | Behavior | Nielsen heuristics applied |
|---|---|---|---|
| Section accordion | collapsed(default) · expanded | Built client-side by `settings.js` exactly like the other six sections — no new controller, no page-level inline `<script>` (NFR-087, CSS7). | #4 Consistency and standards |
| Anchor chip `Hardware` | idle · active | Added to the existing chip strip; scrolls to and expands the section (`anchor-chip.js`). | #7 Flexibility (direct navigation to a section) |
| Fan-mode segmented (`Auto`/`Quiet`/`Performance`/`Off`) | default · active · focus-visible · disabled(saving) · error | `data-widget="segmented"`, field `fan_mode`, hidden input carries the value. Exactly four options — no free text, so an invalid mode cannot be typed. | #5 Error prevention; #6 Recognition over recall |
| OLED toggle | on · off · focus-visible | `data-widget="toggle"`, field `oled_enabled`. Gates the metric control. | #4 Consistency |
| OLED metric segmented (`CPU`/`RAM`/`Temp`/`IP`) | default · active · focus-visible · disabled(OLED off) · error | `data-widget="segmented"`, field `oled_metric`. | #5 Error prevention |
| Scope note | static | "Ultron stores these settings; it does not drive the fan or OLED panel yet." | #1 Visibility of system status — the admin must not believe hardware is being driven |
| Form-state pill + status line | idle · saving · applied · failed(+Retry) | Existing `[data-form-state-host="hardware"]` + `#hardware-status`. | #1 Visibility; #9 Error recovery |

---

## Nielsen Compliance

### `/settings`

| Heuristic | How the design satisfies it | Trade-off |
|---|---|---|
| 1. Visibility of system status | Mute shows the remaining time, not just "muted". The hardware section states plainly that it drives nothing. Every form keeps the existing `saving → applied/failed` pill. | The hardware scope note is friction the admin must read — accepted: a silent non-functional control is worse than an honest one. |
| 2. Match with the real world | "Mute for 4h", "Daily digest at 08:00", "Fan: Quiet" — the admin's language, not `expires_at` / `digest_hour` / enum values. | — |
| 3. User control and freedom | A mute is cancellable at any time; the digest is a toggle; nothing here is destructive or irreversible. | — |
| 4. Consistency and standards | Every new control is an existing widget (segmented / toggle / chip-preset / stepper) inside the existing accordion, saving through the existing form pipeline. Zero new interaction patterns. | Constrains the design to the established vocabulary — this is the point (NFR-087). |
| 5. Error prevention | Bounded controls: the hour stepper cannot leave 0–23; fan mode and OLED metric are closed sets, so an invalid value cannot be typed. Dependent controls are disabled rather than accepting no-op input. | Client bounds are a convenience, not the guarantee — the server still validates (AC-082-003, AC-083-003). |
| 6. Recognition over recall | Durations, hours and modes are visible choices; the admin never types an enum or remembers a valid value. | — |
| 7. Flexibility and efficiency | The `Hardware` anchor chip jumps straight to the section; mute is two clicks from the sidebar. | — |
| 8. Aesthetic and minimalist design | The digest is two controls (toggle + hour), not a cron builder. The hardware section is four controls. | Deliberately no advanced options — the no-go zone forbids them. |
| 9. Recognize, diagnose, recover | Server validation errors land as inline field errors next to the offending input; a failed save keeps the previous state and offers Retry. | — |
| 10. Help and documentation | The mute help text and the hardware scope note carry the two facts the admin cannot infer from the controls. | — |

### `/services`

| Heuristic | How the design satisfies it | Trade-off |
|---|---|---|
| 1. Visibility of system status | The Logs button has a loading state; the drawer distinguishes loading / loaded / empty / error. | — |
| 2. Match with the real world | "Logs" and the raw journal tail — what the admin would have run `journalctl -u X -n 100` to see. | — |
| 3. User control and freedom | The drawer opens and closes on demand; it never opens itself. | — |
| 4. Consistency | The Logs button sits with Start/Stop/Restart, same shape, same swap semantics, same CSRF/auth path. | — |
| 5. Error prevention | The unit name is not user-typed — it comes from the rendered row and is re-validated against the helper's allow-list (AC-081-002, NFR-088). | — |
| 8. Aesthetic and minimalist design | 100 lines in a bounded, scrollable panel — not a live tail that would keep the Pi busy. | The admin cannot follow logs live; the no-go zone excludes it (and it protects the Pi). |
| 9. Recognize, diagnose, recover | Error and empty states are different messages, so "helper down" is never mistaken for "no logs". | — |

**Cross-cutting a11y (inherits the parent's FR-069/FR-070 and AC-009-004):** every new interactive control is ≥44×44px; focus-visible rings come from the existing `@layer` rules; the drawer toggle carries `aria-expanded`; the segmented groups are `role="radiogroup"` with `aria-checked` (matching the existing severity segmented control); the drawer respects `prefers-reduced-motion` via the existing rule.

---

## Design Tokens

**Authority: the existing product.** Ultron-AP already ships a dark-mode token set in `web/css/input.css`, and the parent's AC-009-001/005 (now enforced by a WCAG contrast test computed from these exact tokens) pins them. This feature introduces **no new token** — inventing one would break the contrast test and NFR-087. The table below transcribes the tokens the new components use and states why each is the right role.

### Color roles

| Role | Token | Value | Reason |
|---|---|---|---|
| Background | `--color-base` | `#0b0c0f` | Page background. The drawer and hardware section sit on it. |
| Surface | `--color-surface` | `#121418` | Section panels; the log drawer's own background, so the tail reads as a distinct plane from the row. |
| Card | `--color-card` | `#1a1d23` | Raised rows inside a section (mute status row, hardware controls' hover state). |
| Primary text | `--color-text` | `#e5e7eb` | Log lines and control labels. Contrast on `--color-base` is 14.6:1 — WCAG AA body (≥4.5:1) with margin. |
| Secondary text | `--color-text-muted` | `#9ca3af` | Help text, the hardware scope note, timestamps in the digest row. 7.3:1 on `--color-base` — still AA body, so the "honesty" note is genuinely readable, not decorative grey. |
| Accent | `--color-accent` | `#c2c7d0` | Active state of the segmented/chip controls (`data-[active=true]`), consistent with the severity control. |
| Error | `--color-danger` | `#e34b6a` | The drawer's error state, inline field errors, the `failed` pill. |
| Error surface | `--color-error-bg` / `--color-error-text` | `#4a1525` / `#ff6b6b` | The drawer's error banner. 5.6:1 — AA body on its own background. |
| Border | `--color-border` | `#2a2f37` | Control outlines, the drawer's top rule separating it from its row. |
| Success | Tailwind `green-400` (`bg-green-400/20 text-green-400`) | — | The `applied` pill and the muted-state row use the same green the product already uses for "active/ok". |
| Warning | Tailwind `yellow-400` | — | The `saving` pill; also the muted-state accent, because a mute is a *deliberately degraded* delivery state — the same semantic the product gives `warning`. |

### Type scale

| Element | Class | Reason |
|---|---|---|
| Section heading | `text-sm md:text-[1rem] font-semibold` | Matches the six existing settings sections exactly. |
| Control label | `text-[11px] text-text-muted` | Existing settings label size. |
| Help / scope note | `text-xs text-text-muted` | Existing help-text size. |
| Log lines | `text-xs font-mono` | Monospace is required for a journal tail: column alignment carries meaning (timestamps, PIDs). This is the same `font-mono` the product already uses for metric values. |
| Status line | `text-xs` | Existing `#…-status` size. |

Font family: the existing `font-sans` stack for chrome and `font-mono` for log content — no new font is loaded (the CSP forbids external fonts, and a webfont on a Pi is a needless payload).

### Spacing scale

The existing Tailwind scale, as used by the settings partials: `gap-1`/`gap-2` inside a control group, `space-y-3` between rows in a section, `p-3`/`p-4` for section padding, `mt-1` between a label and its control. The log drawer uses `p-3` with `max-h-96 overflow-y-auto`, so a 100-line tail scrolls inside the drawer and the page body never scrolls horizontally.

**Touch targets:** `min-h-[44px] min-w-[44px]` on every new interactive control — the parent AC-009-004 now has a test that fails on any sub-44px marker in the settings markup, and the Logs button inherits the same floor.

---

## Regression boundary (UX)

The following must look and behave exactly as they do today after this feature:

- The six existing settings sections, their accordion behavior, their form-state pills, and their widgets — including after an hx-boost body swap and a browser back/forward restore (the controller and widgets re-init on `htmx:afterSwap` / `htmx:historyRestore`).
- The `/services` Start/Stop/Restart controls and their result fragment.
- The `/logs` page-level source dropdown (FR-010) — unchanged; the drawer is additive.
- No page-level inline `<script>` returns to `settings.html` or `services.html`: the drawer is pure htmx attributes, and the hardware section binds through the existing `settings.js` (CSS7).
