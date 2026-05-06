# UX Spec — settings-revamp

Archetype: **server-rendered admin form page** (HTMX + Tailwind, dark mode, single-operator). Surface = one screen (`/settings`) split into seven accordion sections plus a sticky anchor-chip strip and the cross-page header dropdown. The revamp is a widget refactor: **no new screens, no new flows**, only better controls inside the existing screen.

---

## User Flows

### Persona — Pi Operator (admin), single user, mixed laptop / phone use

#### Flow A — Tune an alert threshold (most common)
- **Entry:** sidebar → /settings, or header dropdown shortcut.
- **Steps:**
  1. Anchor-chip strip is sticky on top → click `Alerts` chip.
  2. Page scrolls AND the Alerts accordion expands (FR-063).
  3. User scans the rule list, picks a rule (or "+ New rule").
  4. Severity is set in **one click** on the segmented control (FR-058).
  5. Threshold is set with the stepper; allowed range is visible inline ("1–100 %"), so the user never types out of range (FR-057, FR-060).
  6. Cooldown is set by clicking a chip preset (1m / 5m / 15m / 1h) or "custom" (FR-059).
  7. User clicks **Save** → form-state pill flashes `saving` → `applied` → disappears (FR-065).
- **Exit:** rule saved, pill returns to idle (DOM-removed). User can navigate via chip strip.
- **Error path:** if the user manually overrides the stepper to type 999, inline error appears next to the field with the same range string from the label ("1–100 %"). No round-trip needed.

#### Flow B — Configure backup destination + encryption key
- **Entry:** anchor chip `Backup`.
- **Steps:**
  1. Backup accordion expands; user sees three sub-headings: **Limits**, **Schedule**, **Destination** (FR-062).
  2. In Schedule, the user picks a time on a single `<input type="time">` (FR-064).
  3. In Destination, the user picks scheme (env / kms / file) on a select, types value (e.g. `ULTRON_BACKUP_KEY`), tabs away.
  4. HTMX GET probes `/api/settings/encryption-key/probe`; badge renders ✓ "env var ULTRON_BACKUP_KEY found" or ✗ "env var not set" (FR-068).
  5. User clicks **Save**.
- **Exit:** backup destination saved.
- **Error path:** ✗ badge stays visible; the user fixes the scheme or value before Save. Save itself does not re-probe — the badge is the live signal.

#### Flow C — Restart or shutdown the device
- **Entry:** anchor chip `System`.
- **Steps:**
  1. User sees Restart and Shutdown cards. Shutdown has a red-tinted border + `DESTRUCTIVE` eyebrow (FR-067). Logout is **not** in the grid.
  2. User clicks Restart → existing typed-confirmation guard fires (countdown + word match — unchanged).
  3. On confirm, action posts via existing helper IPC.
- **Exit:** device restarts.
- **Error path (Logout instead):** user opens the header dropdown (any page) → clicks Logout → existing /logout endpoint, redirected to /login.

#### Flow D — Deep-link into a section
- **Entry:** /settings#telegram (e.g. from a chat link, bookmark).
- **Steps:**
  1. Page first paints with the Telegram accordion **already expanded** — no flash of collapsed content (FR-063).
  2. User edits, saves.
- **Exit:** as Flow A.
- **Error path:** if the hash references an unknown section (e.g. /settings#removed), the page loads with no accordion expanded and the chip strip remains; no JS error.

---

## Component Inventory

Each component has 5 states: default · loading · error · empty · disabled. "N/A — single instance" is used where a state cannot meaningfully exist (e.g. `empty` on a stepper).

### Section 1: Alerts (AlertConfig form)

| Component | States | Behavior | Nielsen heuristics |
|---|---|---|---|
| **Stepper (numeric, optional unit-dropdown)** | default = current value rendered + range hint visible · loading = save-in-flight, − / + buttons disabled · error = inline message in `text-error-text` below field, same range string as label · empty = N/A (always has stored value or default 0) · disabled = grey −/+ buttons, no hover | Click +/− steps within bounds. Direct keyboard input still allowed; out-of-range typing triggers blur-time inline error. Unit dropdown (sec/min/hr) part of the control where applicable. | #1 visibility (range visible), #5 prevention (bounds), #9 recovery (inline error not toast) |
| **Severity segmented control (3-button)** | default = one segment active in semantic colour (critical=danger, warning=yellow, info=accent), others neutral · loading = N/A (selection is local, save-on-Save) · error = N/A · empty = N/A · disabled = all three rendered in muted with reduced opacity | One-click selection. Keyboard: ←/→ cycle, Space/Enter selects. Mobile <sm: row of 3 dot+label icons; semantics preserved. | #4 consistency (colour matches alert card), #6 recognition (no dropdown to discover) |
| **Cooldown chip-preset row + custom field** | default = a preset highlighted if stored value matches a preset, else no chip highlighted and `custom` field is the visible source · loading = N/A · error = inherited from stepper inside `custom` · empty = N/A · disabled = chips faded, custom field disabled | Click chip → set value & highlight only that chip. Click `custom` or type → clear all preset highlights. | #6 recognition, #7 flexibility (custom escape hatch) |

### Section 2: Performance + Section 3: SSE / Polling intervals

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Stepper with unit dropdown** (Dashboard refresh, Disk poll, Docker poll, Service poll) | as above | as above; unit dropdown is sec/min/hr per the field. | #1 visibility, #5 prevention |

### Section 4: Notifications (Telegram, Email)

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Toggle switch** ("Enabled") | default = visible on/off track + thumb in semantic colour (accent on, muted off) · loading = mid-flight on save (handled at form level) · error = N/A (the toggle never errors locally) · empty = N/A · disabled = faded, not interactive | Click toggles; Space toggles when focused; touch ≥44 px. | #6 recognition (state is the visual), #4 consistency (replaces ambiguous checkbox) |
| **Test button** (existing Telegram/Email "Test") | unchanged states/behaviour from today | unchanged | n/a |

### Section 5: Backup (sub-divided)

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Sub-section heading** (Limits / Schedule / Destination) | default = uppercase eyebrow + section title pair, ≥16 px breathing room above | Static. | #4 consistency (matches section heading style) |
| **Time picker** (`<input type="time">` for backup schedule) | default = HH:MM rendered from stored hour+minute · loading = blocked during save · error = native browser invalid-time message · empty = N/A (always has default) · disabled = greyed | Native picker. Submit sends `time=HH:MM`. Server still accepts legacy `hour=N&minute=M` for one release. | #2 match real world (clock not 2 numbers), #5 prevention |
| **Encryption-key composite picker** (scheme select + value + ✓/✗ badge) | default = scheme + value rendered, badge idle · loading = badge shows spinner during HTMX probe · error = badge ✗ + one-line reason · empty = scheme=env, value="" → badge muted "enter a value" · disabled = field greyed | On blur of value: HTMX GET → /api/settings/encryption-key/probe → updates badge. Probe never returns key bytes. | #1 visibility (live status), #5 prevention (typo caught before save), #10 help-and-docs (reason line) |

### Section 6: System Controls

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Restart card** | default = neutral border, action button | as today | #4 consistency |
| **Shutdown card** | default = **red-tinted border** + "DESTRUCTIVE" eyebrow + action button · all other states inherit existing typed-confirmation modal | as today | #5 prevention (visual weight matches consequence) |
| **Logout** (REMOVED from this section) | n/a — moved to header dropdown | n/a | #4 consistency (Logout is benign, lives with session controls) |

### Cross-page: Header dropdown

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Header user-menu dropdown** | default = avatar/glyph button visible, menu hidden · open = panel with Logout (and future items) · disabled = N/A · error = N/A · empty = N/A | Click avatar → menu opens; Logout → POST /logout. Reachable from every authenticated page. | #4 consistency (single Logout location), #7 flexibility (1-click from anywhere) |

### Cross-section: Anchor-chip strip

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Anchor chips** (sticky top of /settings) | default = chip per section, current section chip highlighted via `IntersectionObserver` · loading = N/A · error = N/A · empty = N/A · disabled = N/A | Click chip → smooth-scroll to section AND expand its accordion AND `history.replaceState` updates `#hash`. Page-load with hash auto-expands matching section before paint. | #1 visibility (highlight current), #3 user-control (replaceState — Back doesn't trap), #4 consistency |

### Cross-section: Form-state pill

| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Form-state pill** | default = **NOT in DOM** (idle) · loading = `saving` (rendered within 100 ms of submit) · success = `applied` (1.5–4 s) · error = `failed` (persists until next user interaction with section) · disabled = N/A | Rendered only during transitions. Reuses existing semantic colour tokens. | #1 visibility-when-relevant, anti-clutter |

### Section 7 — Section number badges

**REMOVED.** No component.

---

## Nielsen Compliance — `/settings`

| # | Heuristic | How the design satisfies it | Trade-off |
|---|---|---|---|
| 1 | **Visibility of system status** | Range hint is visible BEFORE the user types (stepper label). Form-state pill appears within 100 ms of submit. Encryption-key probe gives live ✓/✗ on blur. | Pill DOM-removed at idle (zero clutter) — accepted; transient state is shown only when state ≠ idle. |
| 2 | **Match between system and real world** | Backup schedule uses a clock picker, not 2 numeric fields. Severity uses red/yellow/blue colour tokens that match the alert cards. Cooldown chip presets use natural English labels (1m / 5m / 15m / 1h). | None. |
| 3 | **User control and freedom** | Anchor-chip clicks update hash via `replaceState` so Back button does not trap the user in chip-click history. Cooldown "custom" escape hatch always available. | We do NOT wire chip clicks to `pushState` — accepted: chip nav is in-page navigation, not page transitions. |
| 4 | **Consistency and standards** | New widgets reuse existing tokens (no new colour primitives). Sub-section headings use the same uppercase-eyebrow style as section headings. Logout sits in the header dropdown across every page. | None. |
| 5 | **Error prevention** | Stepper bounds the input. Visible range hint matches server validation string (single source of truth, FR-060). Shutdown gets red border + eyebrow so the destructive action is unmistakable. Encryption-key probe catches typos before save. | Native browser time picker has known quirks on Safari iOS — accepted; legacy `hour=N&minute=M` remains accepted for one release as a fallback (FR-064). |
| 6 | **Recognition rather than recall** | Severity is a 3-segment colour control, not a dropdown — the user sees the choices and the colours simultaneously. Range hints make valid values discoverable. | None. |
| 7 | **Flexibility and efficiency of use** | Cooldown presets accelerate the common case; "custom" handles the rare. Keyboard nav on segmented control + stepper + toggle works without mouse. | None. |
| 8 | **Aesthetic and minimalist design** | Form-state pill removed when idle. Section number badges removed (`01`–`07`) — no fake wizard. Forms span the shell width (no asymmetric right margin). | We remove visual cues some users might have relied on for "step ordering" — accepted: settings is navigation, not a wizard. |
| 9 | **Help users recognise, diagnose, recover from errors** | Inline validation error reads the exact same range string as the label. Encryption-key probe ✗ message names the cause ("env var not set"). | None. |
| 10 | **Help and documentation** | Encryption-key probe doubles as documentation (the reason text explains scheme expectations). Range hints are documentation in the label. | We do NOT add a separate help link — accepted; the /help page (FR-048+) is reachable via sidebar. |

**Heuristics applied: 10/10. Violations found: 0 — all addressed at design time. Trade-offs accepted: 4 (listed above).**

---

## Design Tokens

**Source: parent project's existing `web/css/input.css` `@theme` block.** No new tokens are introduced. The settings revamp is a widget refactor; tokens are reused verbatim.

### Colour roles

| Role | Token | Hex | Reason |
|---|---|---|---|
| Background (page) | `--color-base` | `#0b0c0f` | Existing dark-graphite base; reused. |
| Surface (panel) | `--color-surface` | `#121418` | Section panel background. |
| Card (alert/system control card) | `--color-card` | `#1a1d23` | Reused for stepper/segmented-control inactive segments. |
| Text primary | `--color-text` | `#e5e7eb` | Body text + label. WCAG AA on `--color-base`: contrast ≈ 14.6:1. |
| Text secondary / muted | `--color-text-muted` | `#9ca3af` | Range hints, eyebrows. Contrast on base ≈ 6.6:1 — passes AA. |
| Accent (active chip / toggle on) | `--color-accent` | `#c2c7d0` | Existing accent. Severity=info also uses this. |
| Critical / danger / Shutdown border | `--color-danger` | `#e34b6a` | Severity=critical segment + Shutdown card border. |
| Error message text | `--color-error-text` | `#ff6b6b` | Inline error below stepper. |
| Error background | `--color-error-bg` | `#4a1525` | Error pill / probe ✗ badge. |
| Border (inactive segment, stepper outline) | `--color-border` | `#2a2f37` | Reused. |
| Warning (severity=warning segment) | `rgba(250, 204, 21, …)` (existing yellow used in `metric-warning` / `alert-card-warning`) | `#facc15` | Already used elsewhere — no new token. |
| Success / applied (probe ✓, pill applied) | derived from `--color-accent` 20% bg | n/a | Reuses existing `bg-accent/20` pattern. |

**Contrast verification (against `--color-base` `#0b0c0f`):**
- `--color-text` `#e5e7eb` → 14.6:1 ✅ (≥4.5:1 body)
- `--color-text-muted` `#9ca3af` → 6.6:1 ✅
- `--color-accent` `#c2c7d0` → 11.0:1 ✅
- `--color-danger` `#e34b6a` → 4.5:1 ✅ (passes AA at exactly threshold for body — AA-large for sure)
- `--color-error-text` `#ff6b6b` → 6.4:1 ✅

**No contrast gaps. All roles ≥4.5:1.**

### Type scale

| Role | Family | Size / Weight | Reason |
|---|---|---|---|
| Body | `Space Grotesk` (existing `--font-sans`) | 14px / 400 | Existing baseline. |
| Section title | `Space Grotesk` | 16px / 600 | Existing pattern. |
| Eyebrow / sub-section heading | `Space Grotesk` | 12px / 600 / `uppercase` / `tracking-wider` | Existing eyebrow pattern (matches "Operational Indicators" elsewhere). |
| Range hint | `Space Grotesk` | 12px / 400 / `text-text-muted` | Subordinate to label. |
| Stepper value cell | `Space Grotesk` | 14px / 500 / tabular-nums | Numeric stability across digits. |
| Probe reason text | `Space Grotesk` | 12px / 400 / `text-text-muted` (✓) or `text-error-text` (✗) | Matches inline-error pattern. |

### Spacing scale

Reused from Tailwind's default scale (4 px steps). Specific commitments:
- Stepper button hit-area ≥ `min-w-[44px] min-h-[44px]` (NFR-030).
- Sub-section heading top margin: `mt-6` (24 px) — visible breathing room above each Limits / Schedule / Destination heading.
- Section header right-padding `pr-3` to make room for transient form-state pill.
- Anchor-chip strip vertical padding `py-2` (8 px), horizontal `px-3` per chip (12 px), gap `gap-2` (8 px).
- Segmented control: each segment `min-w-[88px]` desktop, `min-w-[64px]` mobile; full row `gap-0` (segments share borders).

### Component contract

The Phase-4 implementer must ship:
- Stepper widget — `web/static/js/widgets/stepper.js`, vanilla JS, no deps.
- Segmented control — `web/static/js/widgets/segmented.js`.
- Toggle switch — `web/static/js/widgets/toggle.js`.
- Chip-preset row — `web/static/js/widgets/chip-preset.js`.
- Encryption-key probe wiring — HTMX `hx-trigger="blur"` on the value input, `hx-get="/api/settings/encryption-key/probe"`, `hx-target="#enc-key-badge"`, `hx-swap="outerHTML"`.
- Header dropdown — `web/templates/partials/header.html` gains a button + a small panel; CSS reuses existing `panel-soft`.
- All four widget JS files are loaded with the existing `?v=…` cache-bust scheme. No bundler.

---

## Responsive Breakpoints

| Width | Behaviour |
|---|---|
| **375px (mobile)** | Anchor-chip strip horizontally scrollable; sub-section headings still visible; stepper buttons remain ≥44 px; segmented control collapses to 3 dot+label icons; Backup sub-headings stack normally; header dropdown opens full-width below header. |
| **768px (tablet)** | Forms span shell width; sub-section grids `grid-cols-2`; segmented control row layout; header dropdown anchors to top-right of header. |
| **1024px+ (laptop / desktop)** | Forms reach `max-w-5xl` (no inner `max-w-4xl`, FR-070); chip strip not scrollable; Backup `Limits` / `Schedule` / `Destination` use `grid-cols-3` for short-field sub-groups, `grid-cols-2` for the Destination sub-group. |

**Mobile-first verified:** every interactive element is reachable with a thumb at 375px (right-thumb zone covers stepper +/− and segmented-control segments). The encryption-key probe badge sits below the value input, not to the right — never compressed.

---

## Empty states

`/settings` does not have a meaningful "empty state" (config is always populated with defaults at first run). The only place a sub-empty state applies is the **Alert Rules list inside the Alerts section**: when there are zero rules, the existing pattern renders "No rules yet — add your first rule to start receiving notifications" with a primary "+ New rule" button. The revamp does **not** change this — explicit out of scope.

---

## Out of scope (for the UX phase)

- New screen designs, new colour primitives, new font.
- Mobile gesture shortcuts beyond the existing native scroll.
- Dark-mode → light-mode toggle (Ultron-AP is dark-only by design).
- Settings search.
- Settings versioning / change history UI.
