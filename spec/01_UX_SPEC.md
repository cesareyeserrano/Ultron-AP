# UX Spec — Ultron-AP

## 1. Hero Flow

1. Operator opens Dashboard.
2. Above-the-fold summary shows CPU, RAM, temperature, storage, and service health with clear severity colors.
3. A global alert chip in the header displays count and highest severity.
4. If healthy, operator exits in < 10 seconds.
5. If alert exists: click alert chip → Alerts view filtered to active critical/warning.
6. Operator reviews issue card, opens related module (Docker / Services / Logs) in one click.
7. Action outcome appears as explicit success/error state with retry path.

## 2. Component Inventory

| Component | Purpose |
|---|---|
| Status Ribbon | Compact top ribbon with 3 zones: System, Services, Data Freshness |
| Alert Chip | Header badge showing active alert count and severity |
| Metric Card | CPU / RAM / Disk / Temp — value + historical sparkline |
| Service Row | Docker or Systemd row with status badge and action buttons |
| Log Drawer | Last 100 lines for any container or service, on-demand |
| Settings Form | Grouped settings with `idle / saving / applied / failed` states |
| Danger Zone | Typed confirmation + countdown cancel window for shutdown/restart |

## 3. State Matrix

### Dashboard
- **Ideal:** Live metrics update smoothly; no layout shift.
- **Degraded:** Skeleton placeholders and "connecting" badge while SSE reconnects.
- **Failure/Recovery:** Stale-data banner with last update timestamp + manual reconnect action.

### Alerts
- **Ideal:** Grouped by severity with clear timestamps and dismiss actions.
- **Degraded:** Loading group placeholders.
- **Failure/Recovery:** Fetch error panel + retry + fallback link to logs.

### Settings
- **Ideal:** Deterministic save states (`idle → saving → applied`).
- **Degraded:** Controls disabled during in-flight action.
- **Failure/Recovery:** Field-level error message with non-destructive retry.

### Service Controls
- **Ideal:** Start/Stop/Restart completes with explicit success badge.
- **Degraded:** Action disabled if service state unknown.
- **Failure/Recovery:** Error inline with retry and audit trail entry.

## 4. Accessibility Requirements

- Contrast ≥ WCAG AA for all status text on dark backgrounds.
- Full keyboard path for navigation, alert chip, and all primary actions.
- Screen-reader labels on status icons, data-freshness indicator, and reconnect actions.
- Touch targets ≥ 44 × 44 px on mobile.
- Focus visible and persistent across HTMX partial updates.

## 5. Aesthetic Identity

- Dark visual identity; depth via subtle elevation and border hierarchy.
- Typography: consistent heading/body scale with reduced visual noise.
- Semantic color tokens: `ok` (green) / `warn` (yellow) / `critical` (red) / `muted` (gray).
- Shared card, badge, button, and empty/error patterns across all modules.
- Temperature thresholds: green < 60 °C, yellow 60–75 °C, red > 75 °C.

## 6. UX Success Metrics

| Metric | Target |
|---|---|
| Task completion — identify current system state | ≥ 95% |
| Time to identify critical issue | ≤ 8 s median |
| Navigation interactions to corrective context | ≤ 2 |
| Misclick / error action rate | < 3% |
| Accessibility baseline | WCAG 2.1 AA for core dashboard flows |
