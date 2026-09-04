# UX / Design Spec — NUT_UPS_Pi_On_Dashboard

**Archetype: PRO-TECH/DASHBOARD** — reason: this is a feature added to the existing Ultron-AP monitoring dashboard (dark-first, high-density, monospace for numeric data, muted accent, minimal chrome). **The existing Ultron-AP design system is the source of truth** and is transcribed below from `web/css/input.css` and the existing tile/verdict partials (`web/templates/partials/sse-verdicts.html`, `dashboard.html`). No new design language is invented — the UPS surfaces reuse the established `.metric-tile`, `.verdict-card`, severity-border and `.timeline-chip` patterns so the UPS card is scannable next to the CPU/RAM/Network tiles.

**Constraint check (from `01_REQUIREMENTS.json` — not re-asked):**
- Design system / branding → existing Ultron-AP dark graphite tokens (`web/css/input.css`). **USED.**
- Accessibility → WCAG 2.1 AA (inherited from parent FR-009). **USED.**
- Device / viewport → responsive web, mobile-first at 375px (FR-017 AC "renders without horizontal scroll at 375px"). **USED.**
- Performance → live update over the existing SSE channel; no new polling on the browser side. **USED.**

## Scope of this spec
Three surfaces, all inside the existing authenticated dashboard shell (sidebar + main). No new page/route is required for the core card; history reuses the existing charts section pattern.
- **S1 — UPS card** (FR-017): a `metric-tile` on the dashboard grid, live over SSE.
- **S2 — Safe-shutdown config block** (FR-023): a read-only sub-section inside/under the UPS card.
- **S3 — UPS history charts** (FR-019): 24 h / 7 d charts reusing the existing charts area + `timeline-chip` window switcher.

Non-visual FRs (FR-016 client, FR-018 estimator, FR-020 event log, FR-021 alerts, FR-022 insights, FR-024 config) surface through existing UIs (Telegram messages, the existing Alerts panel, the existing Insights section, the `.env` file) and define no new screen here — noted so nothing is silently dropped.

---

## User Flows

Persona: **Raspberry Pi Operator** (tech: mid) — wants to read power state at a glance and be told the moment mains fails, from the same dashboard.

### Flow F1 — Glance at power state (S1, happy path)
- **Entry:** operator opens the dashboard (already authenticated).
- **Steps:** dashboard renders → UPS card appears in the metric grid → shows large status ("En red"), load %, input V, battery V + estimated % (with "estimado" tag), beeper state → SSE pushes update the card in place every cycle.
- **Exit:** operator has the power state without any interaction.
- **Error path:** if `internal/ups` reports `unreachable`, the card renders the **"Sin datos"** state (muted, dashed border) with the reason "UPS sin comunicación" and the relative time since last good sample — never zeros or a blank card.

### Flow F2 — A power event happens (S1, live transition)
- **Entry:** operator is on the dashboard (or returns to it) during a mains event.
- **Steps:** UPS goes `OL → OB` → next SSE push swaps the card to **"En batería"** with the warning border; on `LB` the card escalates to the **critical** border ("Batería baja"). The matching alert also arrives via the existing Alerts panel + Telegram (FR-021), outside this card.
- **Exit:** on `OB → OL` the card returns to "En red"; the outage is recorded (FR-020).
- **Error path:** if communication is lost during the event, card falls to "Sin datos" and the "UPS sin comunicación" verdict is shown.

### Flow F3 — Check when/how NUT will shut down (S2, read-only)
- **Entry:** operator looks at the UPS card's shutdown-config block.
- **Steps:** block shows `ups.delay.shutdown`, `ups.delay.start`, and the low-battery cutoff (21.0 V) — each with a **"gestionado por NUT"** label.
- **Exit:** operator understands the safe-shutdown parameters. **There is no edit control** — this is display-only by design (RS-2/NFR-018).
- **Empty/error path:** any shutdown variable the UPS does not publish shows **"no disponible"**, not blank or 0.

### Flow F4 — Review outage/voltage history (S3)
- **Entry:** operator scrolls to the UPS history charts.
- **Steps:** charts for input.voltage / battery.voltage / ups.load render for the selected window; `timeline-chip` switches 24 h / 7 d.
- **Exit:** operator sees trends and outage count.
- **Empty path:** with no history yet, charts show "Recopilando datos…" (matching the existing "Collecting data..." empty state), not an error.

---

## Component Inventory

### S1 — UPS card (`metric-tile`, live via `sse-swap`)
| Component | States (default / loading / error / empty / disabled) | Behavior | Nielsen |
|---|---|---|---|
| **Status headline** (`ups.status` → Spanish) | default: large label + severity border; loading: "Conectando…"; error: "Sin datos" (muted, dashed border); empty: same as error (no sample yet); disabled: card absent when `ULTRON_UPS_ENABLED` off | Text + border color driven by state map (below). Updates in place on SSE push, no reload | H1, H2, H4 |
| **Severity border** | reuses `.metric-tile` / `.metric-warning` / `.metric-critical` | OL/OL CHRG → default border; OB/RB → warning; LB/OFF/ALARM → critical; BYPASS → warning; unreachable → muted dashed | H1, H4 |
| **Load** (`ups.load` %) | default: "2 %"; error/empty: "—"; others n/a | mono numeral + unit | H2, H6 |
| **Input voltage** (`input.voltage` V) | default value; error/empty: "—" | mono numeral + " V" | H2, H6 |
| **Battery voltage + estimated %** | default: "24.2 V · 50 % estimado"; error/empty: "—"; the "estimado" tag is always present when a % shows | small chip labelled **estimado** next to the %; thin bar fills 0–100% | H2 (honest wording), H6 |
| **Beeper state** (`ups.beeper.status`) | default: "activado"/"silenciado"/"deshabilitado"; error/empty: "—" | text row | H2, H6 |
| **Last-updated caption** | default: relative time ("hace 4 s"); error: "sin datos hace 3 min" | `text-[10px] text-text-muted`, like verdict `RelativeTime` | H1 |

### S2 — Safe-shutdown config block (read-only, inside/under the UPS card)
| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Section heading** "Apagado seguro" + "gestionado por NUT" tag | default | static label; tag signals ownership so no one looks for an edit control | H2, H8 |
| **Delay rows** (`ups.delay.shutdown`, `ups.delay.start`) | default: value + unit (s); empty/error: **"no disponible"**; disabled: n/a | read-only text rows; no inputs, no buttons | H5, H6 |
| **Low-battery cutoff row** ("punto de apagado") | default: "21.0 V"; sourced from configured bound (FR-024), never from a privileged file read | read-only text row | H2 |
| **(explicitly no control)** | — | The block contains zero elements that could issue SET/INSTCMD/shutdown/load.off. This absence is a design requirement, not an omission (NFR-018) | H5 |

### S3 — UPS history charts (reuse existing charts area)
| Component | States | Behavior | Nielsen |
|---|---|---|---|
| **Chart tiles** (input V / battery V / load) | default: line chart ≥1 point; loading: "Recopilando datos…"; error: "Sin datos"; empty: "Recopilando datos…"; disabled: hidden when module off | reuse `.metric-tile` chart pattern from `dashboard.html` | H1, H4 |
| **Window switcher** (24 h / 7 d) | active / idle | reuse `.timeline-chip` / `-active` / `-idle` | H4, H7 |

All text originating from NUT (status, beeper, delays) is **HTML-escaped** before rendering — server-side template escaping, never `innerHTML` (NFR-019; the toast XSS regression must not return).

---

## Nielsen Compliance

**S1 — UPS card**
- H1 Visibility: state + relative-time caption always show whether data is live; SSE push < 1 s. Trade-off: none.
- H2 Match real world: Spanish operator language ("En red", "En batería", "Batería baja"), and the battery % is always tagged **estimado** so it is never mistaken for a real gauge (honest per hardware constraint).
- H4 Consistency: identical `metric-tile` + severity-border grammar as the CPU/RAM/Temp/Network tiles; the operator already reads these colors.
- H8 Minimalist: the card shows only the live essentials; delays/history live in their own blocks.

**S2 — Shutdown config**
- H5 Error prevention: the panel exposes **no** shutdown control at all, so a misclick cannot power the Pi down — the strongest form of prevention. Trade-off accepted: the operator must use NUT directly to change these (explicitly out of scope, RS-2).
- H2 Match real world: "gestionado por NUT" states who owns the setting.
- H6 Recognition: "no disponible" is explicit rather than a silent blank.

**S3 — History**
- H1 Visibility + H4 Consistency: same chart tiles and `timeline-chip` switcher as the main dashboard history.
- H9 Recover from errors: empty state guides ("Recopilando datos…") instead of an error.

No heuristic is violated by a design decision. One deliberate trade-off (S2 has no controls) is accepted and is a security requirement, not a usability gap.

---

## Design Tokens

**Source of truth: transcribed verbatim from `web/css/input.css` (`@theme`). A token that differs from that file is wrong.** The UPS surfaces add no new base tokens; they only map the existing semantic colors to UPS states.

### Color roles (existing Ultron-AP dark graphite palette)
| Role | Token | Hex | Reason |
|---|---|---|---|
| Background (base) | `--color-base` | `#0b0c0f` | app background, existing |
| Surface | `--color-surface` | `#121418` | tile/panel surface (`.metric-tile`), existing |
| Card | `--color-card` | `#1a1d23` | nested card / verdict bg, existing |
| Text primary | `--color-text` | `#e5e7eb` | body text; ≈15:1 on base — AA/AAA |
| Text secondary | `--color-text-muted` | `#9ca3af` | captions/labels; ≈7:1 on surface — AA |
| Accent | `--color-accent` | `#c2c7d0` | neutral/info highlight, focus ring, links |
| Error / critical | `--color-danger` | `#e34b6a` | LB, OFF, ALARM — reuses `.metric-critical` border |
| Border | `--color-border` | `#2a2f37` | default tile border |
| Warning (utility) | Tailwind `yellow-400` | `#facc15` | OB, RB, BYPASS — reuses `.metric-warning` / `.alert-card-warning` (already in the codebase) |

### UPS status → visual state map (new mapping, existing colors only)
| `ups.status` | Label (es) | Border class | Accent |
|---|---|---|---|
| OL | En red | `.metric-tile` (default) | text-primary |
| OL CHRG | Cargando | `.metric-tile` (default) | accent |
| OB | En batería | `.metric-warning` | `#facc15` |
| RB | Reemplazar batería | `.metric-warning` | `#facc15` |
| BYPASS | Bypass | `.metric-warning` | `#facc15` |
| LB | Batería baja | `.metric-critical` | `#e34b6a` |
| OFF | Apagado | `.metric-critical` | `#e34b6a` |
| ALARM | Alarma | `.metric-critical` | `#e34b6a` |
| (unreachable) | Sin datos | muted + dashed border (`--color-border`, `border-dashed`, `text-text-muted`) | muted |

### Typography (existing)
- Sans: `--font-sans` = "Space Grotesk", "Manrope", "Avenir Next", "Segoe UI", sans-serif — labels and status headline.
- Mono: `--font-mono` = "JetBrains Mono", "Fira Code", monospace — all numeric values (V, %, delays), matching the other tiles' numerals.
- Scale (from existing utilities in use): status headline `text-2xl`/`text-3xl` semibold; values `text-sm` mono; labels `text-[11px] uppercase tracking-wider text-text-muted`; captions `text-[10px] text-text-muted`.

### Spacing / shape (existing)
- Tile padding `p-4` (`.metric-tile`), nested rows `p-3` (`.alert-card`/`.verdict-card`), radius `rounded-lg`, grid gap `gap-3`, section spacing `space-y-2`/`space-y-5`.
- Shadow: `--shadow-panel` (existing). Hover lift `translateY(-2px)` reused from `.metric-tile:hover`.
- Focus: existing global `:focus-visible` 2px accent ring — applies to the timeline chips and Learn-more links.

### Responsive behavior (every screen)
- **375px (mobile):** UPS card spans full width (grid `grid-cols-1`); status headline stays large; value rows stack vertically; no horizontal scroll (FR-017 AC). Shutdown-config rows stack. Charts stack one per row.
- **768px (tablet):** grid `md:grid-cols-2`; UPS card sits beside another tile.
- **1440px (desktop):** grid `lg:grid-cols-3`; UPS card is one cell in the existing metric grid; charts row shows all three.
- Motion honors the existing `prefers-reduced-motion` block (transitions neutralized).

---

## Accessibility summary
- Body text `#e5e7eb` on `#0b0c0f`/`#121418`: ≈15:1 (AAA). Muted `#9ca3af` on surface: ≈7:1 (AA).
- Status is never conveyed by color alone — every state has a **text label** (En red / En batería / Batería baja / Sin datos) plus the border color (WCAG 1.4.1 use-of-color).
- Severity badges/borders mirror the existing Alerts/Verdicts components already shipped at AA.
- Keyboard: only interactive elements are the timeline chips and the "Learn more" link — both covered by the global focus ring; the card and shutdown block are non-interactive by design.

## Notes for downstream phases
- Non-visual FRs (FR-016/018/020/021/022/024) render through existing UIs (Telegram, Alerts panel, Insights, `.env`) — no new screen.
- The developer implements these tokens/patterns by reusing `web/css/input.css` classes; **no new colors, fonts, or spacing values** are introduced.
- **Local validation without prod (NFR-022):** a mock/dummy UPS data mode (reusing the simulated upsd) renders this exact card locally so every state above (En red / En batería / Batería baja / Reemplazar batería / Sin datos, plus the shutdown block) can be validated in a browser on the dev machine — no physical UPS, no deploy to the Pi. This spec's states are what the mock must be able to drive.
