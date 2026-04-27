# UX Spec — Ultron-AP

## User Flows

### Flow 1 — Hero: at-a-glance health check
1. Operator opens Dashboard.
2. Above-the-fold summary shows CPU, RAM, Temperature, Storage, and service health with clear severity colours.
3. A global alert chip in the header displays count and highest severity.
4. If healthy, operator exits in < 10 s.
5. If alert exists: click alert chip → Alerts view filtered to active critical/warning.
6. Operator reviews issue card, opens related module (Docker / Services / Logs) in one click.
7. Action outcome appears as explicit success/error state with retry path.

### Flow 2 — Service control with confirmation
1. Operator browses Docker or Services.
2. Clicks Stop/Restart on a row.
3. Confirmation modal appears with explicit action label and target name.
4. On confirm: action dispatches asynchronously; row shows in-flight indicator.
5. On completion: success/error state inline + audit-trail entry persisted.

### Flow 3 — Alert triage from Telegram
1. Telegram message arrives with severity, metric/service, current value vs threshold.
2. Operator opens panel via LAN/VPN link.
3. Alerts view shows the same alert with timestamp and context.
4. Operator clicks the related module; reviews logs (≤ 100 lines) inline.
5. Operator restarts service or mutes the alert (1h / 4h / 24h) as appropriate.

### Flow 4 — First-run authentication
1. Operator visits panel; redirect to /login.
2. Enters credentials configured via env var.
3. Successful login sets HttpOnly session cookie (Secure if behind HTTPS proxy).
4. Repeated failures (5 attempts/IP within 15 min) lock the IP out.

## Component Inventory

| Component | Purpose |
|---|---|
| Status Ribbon | Compact top ribbon with 3 zones: System, Services, Data Freshness |
| Alert Chip | Header badge showing active alert count and severity |
| Metric Card | CPU / RAM / Disk / Temp — value + historical sparkline |
| Service Row | Docker or Systemd row with status badge and action buttons |
| Log Drawer | Last 100 lines for any container or service, on-demand |
| Settings Form | Grouped settings with `idle / saving / applied / failed` states |
| Danger Zone | Typed confirmation + countdown cancel window for shutdown/restart |
| Login Form | Username + password with brute-force feedback ("locked for X minutes") |
| Confirmation Modal | Used for Stop/Restart on services and containers |
| Empty State | Standardised placeholder when a list is empty |
| Error State | Standardised inline error card with retry action |

### Component states
Each interactive component must support: `idle`, `loading`, `success`, `error`, `disabled`. SSE-driven updates are partial-row replacements (HTMX swap), preserving focus.

## Nielsen Compliance

This panel is reviewed against Nielsen's 10 usability heuristics:

1. **Visibility of system status** — SSE pushes every 5 s; "last updated" timestamp on stale data; in-flight badges on actions.
2. **Match with real world** — labels use ops vocabulary (Docker, Systemd, CPU%, MB, °C); no jargon from internal code paths.
3. **User control and freedom** — every destructive action is reversible via confirmation modal with cancel; mute can be undone.
4. **Consistency and standards** — semantic colours `ok/warn/critical/muted` are reused everywhere; same row pattern across Docker and Services.
5. **Error prevention** — Stop/Restart require confirmation; CSRF tokens block silent state changes.
6. **Recognition rather than recall** — alerts list shows current value vs configured threshold inline.
7. **Flexibility and efficiency of use** — keyboard navigation across header, alert chip, and primary actions.
8. **Aesthetic and minimalist design** — single dark theme, no decorative chrome; 12-column grid with consistent spacing.
9. **Help users recognise, diagnose, and recover from errors** — error states include the failing condition and a one-click retry; Telegram test button reports the failure mode (DNS / 401 / chat not found).
10. **Help and documentation** — settings fields show short helper text; settings page has a 'Test' button per integration to validate without external docs.

### Accessibility budget
- Contrast ≥ WCAG 2.1 AA on all status text on dark backgrounds (≥ 4.5:1 for body, ≥ 3:1 for large).
- Touch targets ≥ 44×44 px on mobile (≥ 375 px viewport).
- Full keyboard path for navigation, alert chip, and primary actions.
- Screen-reader labels on status icons, data-freshness indicator, and reconnect actions.
- Focus visible and persistent across HTMX partial updates.

## Design Tokens

### Colour
| Token | Hex | Usage |
|---|---|---|
| `--bg` | `#0e1116` | Page background |
| `--surface` | `#161b22` | Card surface |
| `--surface-2` | `#1f2630` | Elevated surface (modal, drawer) |
| `--border` | `#30363d` | Border, divider |
| `--text` | `#e6edf3` | Body text |
| `--text-muted` | `#9da7b3` | Secondary text |
| `--ok` | `#2ea043` | green — running, active, healthy |
| `--warn` | `#d29922` | yellow — warning severity, in-flight |
| `--critical` | `#f85149` | red — failed, exited-error, critical |
| `--muted` | `#7d8590` | grey — stopped, inactive |

Contrast: every text token vs `--bg` is ≥ 4.5:1 (verified WCAG 2.1 AA). Status badges reuse the same tokens — no parallel palette.

### Typography
- Family: system-ui stack (Inter / SF Pro / Segoe UI).
- Scale: 12 / 14 / 16 / 20 / 24 / 32 px.
- Line-height: 1.4 body, 1.2 headings.

### Spacing & layout
- Spacing scale: 4 / 8 / 12 / 16 / 24 / 32 / 48 px.
- Grid: 12-column responsive; collapses to single column at 768 px breakpoint.
- Touch target: ≥ 44×44 px on viewports ≤ 768 px.
- Card radius: 8 px.

### Motion
- Default transition: 150 ms ease-out.
- No animations exceed 200 ms (metric-update micro-animations only).
- `prefers-reduced-motion` honoured: animations are reduced to opacity transitions.

### Icons
- Size: 16 px inline / 20 px in headers.
- Stroke: 1.5 px.
- Provided by an embedded SVG sprite — no external icon CDN.
