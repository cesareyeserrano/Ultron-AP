# 01_UX_SPEC.md — telegram-message-ux

**Feature:** telegram-message-ux
**Archetype:** Notification surface — short-lived, glanceable, mobile-first content rendered inside two third-party clients (Telegram chat, Email client). The product's own UI footprint is tiny: a single button in settings (`Test Telegram`) plus the toast/inline feedback it produces. The bulk of the "screens" defined here are message templates, not web pages.

**Why this matters for the design:** the user's first (and often only) viewport is the **Telegram lock-screen preview** — typically the first ~80 chars of the subject line on a 375px-wide phone. Every design decision rolls up to one rule: *the operator must be able to triage from the lock screen*.

---

## User Flows

### Persona — Raspberry Pi Operator (mobile triage)
Single persona. Tech level intermediate. On a phone, away from the dashboard.

### Flow 1 — Resource alert fires (CPU / RAM / Disk / Temperature)
- **Entry:** alert engine evaluates a rule and emits a fire event.
- **Steps:**
  1. Telegram push notification arrives on the operator's phone.
  2. Lock-screen preview shows the subject line (≤80 chars): `🔴 CPU usage critical on ultron`.
  3. Operator unlocks → reads the chat row: subject, threshold-aware metric line with elapsed-since-breach, trend hint, probable-cause line, deep-link footer.
  4. Operator decides: act now (tap "Open dashboard"), wait, or ignore. If acting → exits to web dashboard.
- **Exit:** either tap on the deep link (browser opens `/alerts`) or close Telegram (no further interaction).
- **Error path:** Telegram delivery fails → no message arrives. The operator sees nothing here; surfacing this failure is the alert-engine's responsibility (existing behavior, out of scope).

### Flow 2 — Systemd service alert fires
- Same entry/exit as Flow 1, but the message body includes unit name, ActiveState, ActiveEnterTimestamp, and a journal tail block.
- **Branch — journal unavailable:** the journal block is replaced with `journal unavailable: <reason>`. Operator still gets the subject + state line + footer.

### Flow 3 — Docker container alert fires
- Same entry/exit as Flow 1, but the message body includes container name, image, state, exit code (when exited), and a docker-logs tail.
- **Branch — daemon unreachable:** the log block is replaced with `docker logs unavailable: <reason>`.

### Flow 4 — Alert resolves
- **Entry:** alert engine emits a resolve event for a previously-fired rule.
- **Steps:**
  1. Telegram push arrives. Lock-screen preview: `✓ CPU usage RESOLVED on ultron — active for 4m 12s`.
  2. Operator reads the row, confirms recovery, no action needed.
- **Exit:** close Telegram.
- **Error path:** none — resolves are best-effort; if delivery fails the next fire (if any) will surface the situation.

### Flow 5 — Storm of same-rule fires within 60s
- **Entry:** rule R1 fires; 45s later it fires again; 30s later again.
- **Steps:**
  1. Fire #1 → new chat row posted; message_id cached.
  2. Fire #2 (t=45s) → existing row is updated via `editMessageText`; subject becomes `… (2 fires)`. **No new push notification** on most Telegram clients (edit only).
  3. Fire #3 (t=75s, > 60s window) → new chat row posted; cache replaced.
- **Exit:** the operator sees one "live" row that updates, plus one new row when the window rolls over. Lock-screen is not re-buzzed for edits.
- **Error path:** `editMessageText` returns `400 message is not modified` → swallow, no user-visible failure. Other 4xx → log + send a fresh row instead.

### Flow 6 — Operator clicks "Test Telegram" in settings
- **Entry:** operator is on the existing settings page (settings-revamp owns the chrome; this feature owns the button's outcome).
- **Steps:**
  1. Operator taps `Test Telegram`.
  2. Button enters `sending` state immediately (≤100ms feedback per UX standards).
  3. Within ≤3s a synthetic CPU fire arrives in Telegram, prefixed `TEST — `.
  4. Inline result region next to the button updates: success → `Sent. Check Telegram.`; failure → `Failed: <short reason>` with a `Retry` link.
- **Exit:** operator confirms message landed on phone, returns to other settings.
- **Error path (no chat ID configured):** button is **disabled** with helper text `Configure Bot Token & Chat ID first`.
- **Error path (Telegram API error):** inline error message includes the API's `description` field, truncated to 120 chars.

### Flow 7 — Email parity (background flow, no UI)
Each fire/resolve also produces an email when SMTP is configured. Same logical content blocks as Telegram, rendered in HTML. No interactive flow here — just a passive surface. Subject equals Telegram subject minus the leading severity emoji.

---

## Component Inventory

The "screens" here are message templates. Each template is composed of message blocks (components) that can be present, absent, or in a fallback state. Below, each block enumerates all 5 states.

### Surface A — Telegram message (fire / resolve)

| Component | Default | Loading | Error | Empty | Disabled | Behavior | Nielsen |
|---|---|---|---|---|---|---|---|
| **Subject line** | `<emoji> <Friendly Metric> <severity> on <host>` ≤80 chars | n/a (synchronous render) | falls back to `<emoji> Alert on <host>` if friendly-label lookup panics | n/a — always present | n/a | One line, escaped, never wraps in a 375px lock-screen preview | Visibility of system status; Match between system and real world |
| **Storm counter `(N fires)`** | absent on first fire | n/a | n/a | n/a — only appears when N≥2 | n/a | Appended to subject after `editMessageText`; updates in place | Visibility of system status |
| **Metric line** (`CPU 92% (threshold > 80%) for 1m 20s`) | full form with operator+threshold+elapsed | n/a | `<value> (threshold n/a)` when threshold missing | n/a — required for resource fires | n/a | One line; numeric formatting respects metric precision | Help users recognize, diagnose, and recover |
| **Timestamp line** (`2026-05-06T11:14:02Z (local: 2026-05-06 13:14:02 CEST)`) | rendered when first_fired_at unavailable | n/a | n/a | absent when `for <duration>` is rendered instead | n/a | Mutually exclusive with elapsed form | Match between system and real world |
| **Trend line** (`trend: 78% → 92% (Δ +14%)`) | rendered when ring buffer has prior sample | n/a | absent on render error | absent when no sample exists in 4m30s–5m30s window | n/a | Resource alerts only; never `n/a` | Recognition rather than recall |
| **Surface block — systemd** | `unit · state · active since · journal tail (≤600 chars)` | n/a | `journal unavailable: <reason>` | `no recent journal entries` | n/a | Tail truncated with `… (truncated)` | Help users recognize, diagnose, recover |
| **Surface block — docker** | `container · image · state · exit code? · log tail (≤600 chars)` | n/a | `docker logs unavailable: <reason>` | `no recent log lines` | n/a | Exit code shown only when state=exited | Help users recognize, diagnose, recover |
| **Probable-cause line** (`top: ffmpeg (78%)` / `cause: OOM-killed (137)` / `last error: …`) | rendered when derivable | n/a | absent on derivation timeout (>200ms) | absent when no source data | absent on resolves | Single line, prefix per surface (`top:` / `cause:` / `last error:`) | Help users diagnose; Recognition rather than recall |
| **Deep-link footer** (`[Open dashboard](https://…/alerts)`) | always present, last non-empty line | n/a | falls back to `http://<host>:<port>/alerts` if `ULTRON_PUBLIC_URL` unset | n/a — required | n/a | Tap target is the link; Telegram handles navigation | User control and freedom |
| **Severity glyph** (🔴 / 🟡 / 🔵 / ✓) | maps to severity | n/a | falls back to `🔴` if severity unknown | n/a | n/a | Always paired with severity word in subject | Visibility of system status; Recognition |

**Mobile (375px) behavior:** the entire message must be readable without horizontal scroll inside Telegram on a 375px viewport. Telegram wraps long lines; the renderer never inserts non-breaking constructions that would overflow. Tested at 375px in the visual spec preview (Phase 4 step 3).

### Surface B — Email message (fire / resolve)

Mirrors Surface A logically. HTML5, semantic tags (`<h2>` subject, `<p>` metric line, `<pre>` journal/log block, `<a>` footer link). Plain-text alternative is generated from the same model, lines separated by `\n`.

| Component | Default | Loading | Error | Empty | Disabled | Behavior | Nielsen |
|---|---|---|---|---|---|---|---|
| **HTML subject** (no leading emoji) | `CPU usage critical on ultron` | n/a | render error → minimal fallback `Alert on <host>` | n/a | n/a | Email header `Subject:` field | Match between system and real world |
| **HTML body sections** | one `<section>` per logical block, in the same order as Telegram | n/a | falls back to plain-text if HTML render panics | per-block fallbacks identical to Surface A | n/a | Inline styles only — no external CSS | Consistency and standards |
| **Plain-text alternative** | identical lines to Telegram body, no Markdown escaping | n/a | always generated as fallback | n/a | n/a | Required by RFC 2046 multipart/alternative | Error prevention (mail clients without HTML) |
| **Footer link** | `<a href="…/alerts">Open dashboard</a>` | n/a | `http://<host>:<port>/alerts` fallback | n/a | n/a | Same URL contract as Surface A | User control and freedom |

### Surface C — "Test Telegram" button (settings page)

Owned visually by settings-revamp; this feature owns the button's behavior contract.

| Component | Default | Loading | Error | Empty | Disabled | Behavior | Nielsen |
|---|---|---|---|---|---|---|---|
| **Button** | label `Test Telegram` | label `Sending…` + spinner; disabled while in flight | label `Test Telegram` (re-enabled); inline error appears | n/a — button is always present | label `Test Telegram` greyed out when bot token or chat ID missing; helper text below: `Configure Bot Token & Chat ID first` | Tap target ≥44×44 px; activates within ≤100ms of click | Visibility of system status; Error prevention |
| **Inline result region** | empty (no message) | empty during send | `Failed: <reason>` in error color + `Retry` link | empty before any send | empty | Persists until the next click; cleared on Retry | Help users recognize and recover from errors |

---

## Nielsen Compliance

Per surface, applicable heuristics from Nielsen's 10:

### Surface A — Telegram message
| # | Heuristic | How it's satisfied | Trade-off |
|---|---|---|---|
| 1 | Visibility of system status | Severity glyph + word in subject; storm counter updates in place; resolve uses distinct ✓ | none |
| 2 | Match between system and real world | Friendly metric labels (`CPU usage` not `cpu_usage`); local-time alongside UTC | none |
| 3 | User control and freedom | Deep-link footer lets the operator escape to the dashboard at any time | Telegram has no in-message dismiss; that's a platform constraint |
| 4 | Consistency and standards | Same block order across all surfaces (resource/systemd/docker); MarkdownV2 throughout | none |
| 5 | Error prevention | Markdown escaper (FR-025) prevents broken renders; allow-list validation (NFR-008) prevents shell-injection in journal/docker subprocesses | none |
| 6 | Recognition rather than recall | Threshold shown beside value; trend shown beside current; cause shown when derivable — operator never has to remember thresholds | Probable-cause is a hint, not authoritative — accepted, called out in no_go_zone |
| 7 | Flexibility and efficiency of use | Storm protection groups rapid fires so power-users aren't spammed | Edited messages don't re-buzz the lock screen — accepted (that's the point) |
| 8 | Aesthetic and minimalist design | Each block omitted entirely when not derivable; no `n/a` clutter | none |
| 9 | Help users recognize, diagnose, recover | Surface-specific block (journal tail / docker log tail) + probable-cause line directly point at the issue | Diagnosis depth is intentionally shallow — root cause analysis is in no_go_zone |
| 10 | Help and documentation | Deep-link footer is the help — `/alerts` carries full context | No in-message help text; would inflate body |

### Surface B — Email message
Same heuristics as Surface A. Additional: HTML5 + plain-text alternative satisfies **5 (error prevention)** for mail clients that strip HTML.

### Surface C — "Test Telegram" button
| # | Heuristic | How it's satisfied |
|---|---|---|
| 1 | Visibility of system status | Sending → spinner; success/failure shown inline within ≤3s |
| 5 | Error prevention | Disabled state when prerequisites missing; helper text explains why |
| 9 | Help users recognize, diagnose, recover | Failure includes the API description (truncated 120 chars) and a Retry link |

**Violations found and corrected:** zero — design was authored against the heuristics, not retrofitted.
**Trade-offs accepted:**
- (Surface A, heuristic 7) Storm-edited messages don't re-buzz; that's the desired UX, not a bug.
- (Surface A, heuristic 6) Probable-cause is heuristic, not diagnostic. Called out in no_go_zone item #10.

---

## Design Tokens

Even though Telegram's theme is out of our control, the tokens we DO control are: severity glyphs (Telegram), and the full color/type/spacing system for HTML email + the Test Telegram button feedback. These tokens are derived from the existing project palette (FR-009 dark-mode UI, semantic tokens already in use) so notification surfaces remain consistent with the dashboard.

### Severity glyphs — Telegram (the only "color" we control there)
| Token | Value | Reason |
|---|---|---|
| `severity.critical.glyph` | 🔴 | Established convention in IDEA.md and FR-018 ("red circle"); matches dashboard `critical` semantic token |
| `severity.warning.glyph` | 🟡 | Yellow circle; matches dashboard `warn` token (FR-009) |
| `severity.info.glyph` | 🔵 | Blue circle; matches dashboard `info` (low-importance) tier |
| `severity.resolved.glyph` | ✓ | Plain check; pairs with green CSS color in HTML email; the only non-circle glyph, deliberately distinct from fires |

### Color tokens — HTML email + button feedback
All values chosen for ≥4.5:1 contrast against `surface.0` (body text) or ≥3:1 (large/UI per WCAG 2.1 AA). Hex values are aligned with the existing dashboard's dark-mode tokens (FR-009) so a quote-forwarded email visually matches the dashboard.

| Token | Hex | Role | Reason / Contrast |
|---|---|---|---|
| `surface.0` | `#0E1116` | Email body background | Matches dashboard background; reduces glare on phone in dark mode |
| `surface.1` | `#161A22` | Card / section background inside body | Subtle elevation; 1.18:1 vs surface.0 (decorative; not load-bearing) |
| `border.subtle` | `#2A2F3A` | Section separators | 3.1:1 vs surface.0; meets ≥3:1 for UI separators |
| `text.primary` | `#E6E9EF` | Body text, subject | 13.9:1 vs surface.0 — well over 4.5:1 |
| `text.secondary` | `#A6ACBA` | Timestamps, journal tail | 7.4:1 vs surface.0 |
| `severity.critical` | `#F25C5C` | Critical subject + glyph mirror | 4.6:1 vs surface.0 — passes 4.5:1 floor |
| `severity.warning` | `#E6B341` | Warning subject + glyph mirror | 9.1:1 vs surface.0 |
| `severity.info` | `#5AA6F0` | Info subject + glyph mirror | 6.0:1 vs surface.0 |
| `severity.resolved` | `#3FB570` | Resolve checkmark + duration line | 6.1:1 vs surface.0 |
| `accent.link` | `#7CA8F2` | "Open dashboard" link in email | 6.5:1 vs surface.0; distinguishable from severity.info but related family |
| `feedback.success` | `#3FB570` | Test-button success message | Same as severity.resolved — single semantic |
| `feedback.error` | `#F25C5C` | Test-button error message | Same as severity.critical |

### Type scale
Reason: emails must render on legacy mail clients. Use **system font stacks**, not webfonts. No external font fetches (also a privacy + offline-readability win).

| Token | Stack / size | Reason |
|---|---|---|
| `font.system` | `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif` | Native rendering on every platform; zero load |
| `font.mono` | `ui-monospace, "SF Mono", Menlo, Consolas, monospace` | For journal/log/exit-code blocks; preserves alignment |
| `text.subject` | 18px / 600 weight / 1.3 line-height | Mobile-readable headline; reads as `<h2>` semantically |
| `text.body` | 15px / 400 / 1.5 | Above the 14px mobile-readability floor |
| `text.meta` | 13px / 400 / 1.4 | Timestamps, trend, "active for 4m 12s" |
| `text.code` | 13px / 400 / 1.5 / `font.mono` | Journal & log tails; exit-code line |

### Spacing scale (px, 4px base)
Reason: 4px base aligns with dashboard tokens; consistent vertical rhythm makes the email scan-readable on a phone.

| Token | Value | Use |
|---|---|---|
| `space.1` | 4 | Inline gaps within a line (e.g. between glyph and word) |
| `space.2` | 8 | Tight stacks (subject ↔ metric line) |
| `space.3` | 12 | Default block separator |
| `space.4` | 16 | Section padding inside surface.1 cards |
| `space.6` | 24 | Padding between body and footer |
| `space.8` | 32 | Outer body padding (phone) |

### Tap targets
| Token | Value | Use |
|---|---|---|
| `tap.min` | 44×44 px | "Open dashboard" link in email; `Test Telegram` button; `Retry` link — meets WCAG 2.5.5 |

### Length budgets (functional tokens)
| Token | Value | Source |
|---|---|---|
| `length.subject.max` | 80 chars | Lock-screen preview width on 375px |
| `length.body.max` | 4096 chars | Telegram API hard cap (FR-028) |
| `length.surface_block.max` | 600 chars | FR-020 / FR-021 |
| `length.surface_block.minimal` | 100 chars | FR-028 step 4 truncation |
| `length.error_description.max` | 120 chars | Test-button inline error truncation |

### Responsive breakpoints (HTML email + button feedback)
| Breakpoint | Behavior |
|---|---|
| `375px` (phone, primary target) | All blocks stack vertically; `space.8` outer padding; subject line wraps at 2 visual lines max |
| `768px` (tablet) | Same vertical stack; max-width 600px centered; outer padding `space.8` |
| `1440px` (desktop mail client) | Max-width 600px centered; mail clients add their own chrome |

Telegram's mobile/desktop rendering is governed by the Telegram client — not by us; we ship the same MarkdownV2 body to all viewports.

---

## Coverage check

Every UX FR (FR-017, FR-018, FR-019, FR-023, FR-026, FR-027) → has a corresponding component in §2.
Every component → has all 5 states defined (default/loading/error/empty/disabled), with `n/a` declared explicitly where a state cannot occur (e.g. server-rendered messages have no `loading`).
Every error state → tells the operator what happened (`journal unavailable: <reason>`), why (the `<reason>` is the underlying error), and what they can do next (deep-link footer always present → operator can open the dashboard for full context; Test-button failures show `Retry`).
Mobile 375px behavior → explicit per surface in §1 / §4.
