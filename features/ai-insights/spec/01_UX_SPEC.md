# UX / Design Spec — AI Insights (feature: ai-insights)

**Archetype: PRO-TECH/DASHBOARD** — reason: Ultron-AP is a self-hosted system/network **monitoring** panel (devtools/data-ops), already dark-first (FR-009), htmx + SSE, Tailwind, monospace for data. This feature adds an AI layer *inside* that existing surface, so it inherits the established dark theme rather than introducing a new visual language. Explicit visual FRs (FR-018, FR-022, FR-024) refine behavior but do not override the archetype.

**Scope of this spec:** the screens/components this feature ADDS or MODIFIES. Everything else in the panel is unchanged (regression boundary). Tokens in §4 are the **existing** Ultron-AP theme — the developer reuses them, does not re-pick.

**Target medium:** responsive web (htmx-rendered Go templates). Breakpoints: **375px (mobile) · 768px (tablet) · 1440px (desktop)**. Accessibility target: **WCAG 2.1 AA** (inherited project standard).

---

## User Flows

Persona: **Raspberry Pi Operator (single admin, mid tech)**. All flows require an authenticated session (FR-007); unauthenticated access redirects to `/login` (unchanged).

### Flow A — Configure the AI provider (FR-018, FR-025, FR-017)
- **Entry:** Settings → new **"AI Assistant"** section (collapsible card, consistent with existing settings cards).
- **Steps:**
  1. Operator toggles **Enable AI** on.
  2. Fields reveal (progressive disclosure): **Endpoint URL**, **Model name**, **API key** (masked input), **"Send explanation to Telegram on alert"** toggle (default off).
  3. Operator clicks **Test connection** → inline status chip shows `Testing…` then `OK — model: <name>` or a failure reason.
  4. Operator clicks **Save** → inline `Saved` confirmation; key field clears to a masked placeholder (`••••••••`).
- **Exit:** config persisted; AI features become available across the panel.
- **Error paths:**
  - Enable on + endpoint or key empty → **inline field error** ("Endpoint URL is required when AI is enabled"), Save blocked (H5 error prevention).
  - Test connection fails → status chip `Failed: <one-line reason>` (e.g. "401 — check API key", "host unreachable"), nothing persisted (FR-025).
  - Endpoint not `https://` → inline warning; save allowed only with an explicit "use insecure endpoint" confirm (NFR-005).

### Flow B — Explain on demand (FR-016, FR-022, FR-024, FR-019)
- **Entry:** Dashboard / insights — each fired insight card and the system-state header gains an **"Explain with AI"** button.
- **Steps:**
  1. Operator clicks **Explain with AI**.
  2. An **explanation panel** (slide-over on desktop ≥768px, full-width sheet on 375px) opens in **loading** state within ≤200 ms.
  3. On success: renders **Probable cause**, **Suggested remediation**, and a **Cited signals** list (FR-024).
  4. Operator reads; can **Retry**, **Copy**, or **Close** (escape action = Close / Esc key / backdrop tap).
- **Exit:** panel closed; no system state changed (read-only, FR-016).
- **Error paths:**
  - AI not configured → button is **hidden or disabled** with tooltip "Configure AI in Settings" linking to Flow A (FR-019). Direct endpoint hit returns 4xx, never 500.
  - Provider error/timeout → **error state**: one-line reason + **Retry** button (FR-021, FR-022).
  - Telemetry insufficient → **empty state**: "Not enough recent data to explain this yet" + hint to wait for more samples.

### Flow C — Receive AI note in Telegram (FR-026)
- **Entry:** a threshold alert fires while **Telegram-push toggle** is on and AI is configured.
- **Steps:** existing rule-based alert is delivered first (unchanged); a **follow-up message** arrives titled `🤖 AI analysis` with cause + remediation (plain text, secrets redacted).
- **Exit:** operator reads on phone; no panel needed.
- **Error path:** AI generation fails/times out → **only** the normal alert is delivered; failure is logged; no duplicate, no dropped alert (FR-026 AC-026-1f). This is a backend/notify surface, not an interactive screen — included for content/format definition.

#### Telegram message format — category emoji convention
The Telegram **alert header** is prefixed with a **category emoji + severity emoji** so the operator can tell at a glance *what subsystem* the alert relates to, not only how severe it is. The rule-based alert text is otherwise **unchanged** — the category emoji is an additive prefix, it does not replace the message (no_go_zone respected). The AI follow-up and the web Explanation panel (S2) reuse the same category emoji for cross-surface consistency (H4).

Severity (existing): 🔴 critical · 🟡 warning. Category → emoji map (by alert metric/domain):

| Category | Emoji | Category | Emoji |
|---|---|---|---|
| CPU temperature | 🌡️ | Network latency / packet loss | 🌐 |
| CPU load / usage | 🧮 | WiFi / signal | 📶 |
| RAM / memory | 🧠 | Gateway / DNS | 🛰️ |
| Disk | 💾 | Docker container | 🐳 |
| Power / UPS (Pironman) | 🔌 | Systemd service | ⚙️ |
| (fallback / uncategorized) | 🔔 | | |

Examples:
```
🌡️🔴 High CPU temperature        cpu_temp 78°C > 75°C
🐳🟡 Container "nginx" unhealthy
💾🔴 Disk almost full            disk 94% > 90%
🌐🔴 Gateway latency 180ms
⚙️🟡 ssh.service inactive
```
AI follow-up reuses the category: `🤖 AI analysis · 🌡️ thermal`. **Touch-point note:** this lightly enhances the existing FR-005 Telegram alert presentation (adds a category-emoji prefix); the rule-based body and delivery path are not otherwise changed.

---

## Component Inventory

### Screen S1 — Settings › AI Assistant section
| Component | States (default / loading / error / empty / disabled) | Behavior | Nielsen |
|---|---|---|---|
| Enable AI toggle | default=off; loading=n/a; error=n/a; empty=n/a; disabled=while Save in flight | Reveals/hides config fields (progressive disclosure) | H8, H7 |
| Endpoint URL field | default=empty w/ label+placeholder `https://…`; loading=n/a; error=inline red text + border `--color-danger`; empty=required hint; disabled=when AI off | Inline validation on blur (H5) | H5, H6, H9 |
| Model name field | default=empty w/ label; error=inline; empty=hint "e.g. qwen2.5:14b"; disabled=when AI off | Free text, persisted | H6 |
| API key field (masked) | default=`••••••••` if set, empty if not; loading=n/a; error=inline; empty=required hint; disabled=when AI off | Never re-renders the real key (FR-017); typing replaces | H5, secrets |
| Telegram-push toggle | default=off; disabled=when AI off OR Telegram not configured (tooltip explains) | Opt-in additive push (FR-026) | H6, H9 |
| Test connection button + status chip | default=`Test connection`; loading=`Testing…` spinner; error=`Failed: <reason>` red chip; empty=n/a; disabled=when required fields empty | Calls provider, persists nothing (FR-025) | H1, H9 |
| Save button + confirmation | default=`Save`; loading=`Saving…`; error=toast `Couldn't save: <reason>`; disabled=when validation fails | Persists config; inline `Saved` (H1) | H1, H5 |

### Screen S2 — AI Explanation panel (slide-over / mobile sheet)
| Component | States | Behavior | Nielsen |
|---|---|---|---|
| Explanation container | default=closed; loading=skeleton + spinner ≤200ms; error=error block; empty=empty block; disabled=n/a | Opens from trigger; focus-trapped; Esc/backdrop closes (H3) | H1, H3, H8 |
| Probable cause block | default=rendered text; loading=skeleton lines; error=hidden; empty="insufficient data" message; disabled=n/a | Plain language (H2) | H2 |
| Remediation block | default=rendered steps; loading=skeleton; error=hidden; empty=hidden; disabled=n/a | Advisory only, never an action button | H2, H8 |
| Cited signals list | default=chips of named signals; loading=hidden; error=hidden; empty=label `unverified` if no citation (FR-024); disabled=n/a | Lets operator sanity-check grounding | H1, H10 |
| Retry / Copy / Close controls | default=all enabled; loading=Retry hidden, Close enabled; error=Retry primary; empty=Retry; disabled=Copy disabled until content exists | Retry re-requests; Close is escape action | H3, H7 |

### Screen S3 — "Explain with AI" trigger (on dashboard insight card & system header)
| Component | States | Behavior | Nielsen |
|---|---|---|---|
| Explain button | default=`✨ Explain with AI`; loading=spinner inline while panel opens; error=n/a (errors surface in S2); empty=n/a; disabled/hidden=when AI unconfigured, tooltip → Settings (FR-019) | One-click primary action per insight (H7) | H6, H7, H9 |

---

## Nielsen Compliance

**S1 — Settings › AI Assistant**
- H1 Visibility: Test/Save show `Testing…/Saving…` then result chip within ≤1s.
- H5 Error prevention: required fields validated on blur before Save; Save disabled on invalid config.
- H6 Recognition: labels above every field (no placeholder-only); model field hint shows an example value.
- H9 Error recovery: failures state cause + fix ("401 — check API key"), never bare "Error".
- Trade-off accepted: insecure (non-https) endpoint is *allowed* behind an explicit confirm rather than hard-blocked — the operator may run a trusted LAN/Tailscale endpoint; surfaced as a warning (documented deviation from strict H5).

**S2 — AI Explanation panel**
- H1: loading indicator ≤200ms (FR-022).
- H2: cause/remediation in the operator's language, not raw model jargon.
- H3: Esc, backdrop tap, and explicit Close all dismiss; no destructive action exists (read-only) so no confirm needed.
- H8: only cause + remediation + citations shown — no decorative chrome.
- H9: error state gives one-line reason + Retry.
- H10: `unverified` label teaches the operator when grounding is weak (FR-024).

**S3 — Trigger**
- H6/H7: a single recognizable action per insight; primary, one click.
- H9: when unconfigured, disabled state explains why and links to the fix (no dead end).

**Heuristics applied: 9/10** (H4 consistency inherited from existing patterns; H10 partial — contextual hints, no separate onboarding, acceptable for a single-admin tool). **Violations found: 1 · corrected: 0 · accepted trade-off: 1** (non-https endpoint confirm, above).

---

## Design Tokens

**Source of truth:** the existing Ultron-AP dark theme (`web/css/input.css`). This feature **reuses** these tokens — it does not introduce new colors. Reason: visual consistency with the rest of the panel (FR-009) and PRO-TECH/DASHBOARD archetype (dark-first, muted, monospace for data).

### Color roles (existing CSS custom properties)
| Role | Token | Value | Reason |
|---|---|---|---|
| Background (base) | `--color-base` | `#0b0c0f` | App page background — deep neutral, dashboard archetype |
| Surface | `--color-surface` | `#121418` | Panels/slide-over background, one step above base |
| Card | `--color-card` | `#1a1d23` | Settings cards, explanation blocks |
| Border | `--color-border` | `#2a2f37` | Field/card separators, low-emphasis |
| Text primary | `--color-text` | `#e5e7eb` | Body/heading text — ≥12:1 on base (AA pass) |
| Text secondary | `--color-text-muted` | `#9ca3af` | Labels, citations, hints — ~5.9:1 on base (AA pass) |
| Primary / accent | `--color-accent` | `#c2c7d0` | Neutral accent for controls/icons |
| Success accent | `green-400/500` (Tailwind `#4ade80 / #22c55e`) | existing "OK/active" accent (used 20×) — Test-connection success chip, healthy states |
| Error text | `--color-error-text` | `#ff6b6b` | Inline field errors, error state heading |
| Error surface | `--color-error-bg` | `#4a1525` | Error block background |
| Danger action | `--color-danger` / hover `--color-danger-hover` | `#e34b6a` / `#c93f5c` | (not used here — no destructive AI action; listed for consistency) |

**AI-specific accent:** the "Explain with AI" affordance uses the existing **success-green** accent (not a new color) to read as an additive, safe (read-only) action — reason: avoid implying a destructive/control action, stay within the palette.

### Type scale
- **Font families:** `font-sans` (UI text — inherited Tailwind sans stack) and **`font-mono`** for data/signal chips, model names, endpoint URLs, and cited signals (`rule_id=17`, `cpu_temp`) — reason: monospace disambiguates identifiers/values, archetype default.
- **Scale (inherited Tailwind):** `text-xs` (12px) labels/citations · `text-sm` (14px) body/fields · `text-base` (16px) explanation body · `text-lg` (18px) section/panel titles. Weights: 400 body, 500 labels, 600 headings.

### Spacing scale
- Inherited Tailwind 4px base: `p-3`/`gap-3` (12px) inside fields, `p-4` (16px) card padding, `gap-2` (8px) chip rows, `mt-6` (24px) between settings groups — reason: matches existing settings card rhythm.

### Responsive behavior (per screen)
- **S1 Settings:** single-column stacked fields at **375px** (no horizontal scroll — FR-018 AC); two-column label/field grid at **768px+**; max-width card centered at **1440px**.
- **S2 Explanation panel:** **375px** = full-width bottom sheet, content scrolls vertically, sticky Close; **768px** = right slide-over 420px wide; **1440px** = slide-over 480px, dashboard stays visible behind.
- **S3 Trigger:** **375px** = full-width button under the insight; **768px+** = inline icon-button aligned right on the insight card. Tap target ≥44×44px at all sizes.

### Contrast confirmation
All text roles ≥4.5:1 on their backgrounds: text-primary `#e5e7eb` and text-muted `#9ca3af` pass AA on `--color-base`/`--color-card`; error-text `#ff6b6b` on `--color-error-bg` `#4a1525` passes AA for body. **Confirmed — no gaps.**
