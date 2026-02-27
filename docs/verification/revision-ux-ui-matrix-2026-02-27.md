# UX/UI Audit Matrix — revision-ux-ui

Date: 2026-02-27  
Environment target: Raspberry Pi (`http://192.168.1.29:8080`)  
Scope: `dashboard`, `docker`, `services`, `alerts`, `history`, `logs`, `settings`  
Visual direction constraint: keep original dark graphite style (black/gray), no new features.

## Severity Scale
- P1: Blocks task completion or causes broken rendering.
- P2: High friction, misalignment, or readability issue that slows operation.
- P3: Minor polish/consistency issue.

## Module Matrix

| Module | Finding | Severity | Status |
|---|---|---:|---|
| Dashboard | Previous render break from invalid template comparison (`Temperature`) | P1 | Fixed |
| Dashboard | KPI cards inconsistent across breakpoints | P2 | Fixed |
| Dashboard | Timeline controls lacked clear grouping | P3 | Fixed |
| Docker | Action buttons could crowd/wrap poorly on mobile widths | P2 | Fixed |
| Docker | Header hierarchy inconsistent vs other modules | P3 | Fixed |
| Services | Service action row crowded on mobile | P2 | Fixed |
| Services | Group rows had uneven visual alignment in narrow layout | P3 | Fixed |
| Alerts | Message text truncation hid critical alert context | P2 | Fixed |
| Alerts | Filter/action bar inconsistent wrapping in small widths | P3 | Fixed |
| History | Long `target` and error text truncated diagnostics | P2 | Fixed |
| History | Filter controls not grouped consistently with other modules | P3 | Fixed |
| Logs | Source selector + action button vertical misalignment | P2 | Fixed |
| Logs | Top heading density inconsistent with module pattern | P3 | Fixed |
| Settings | Section navigation band lacked shared panel structure | P3 | Fixed |
| Global (Header) | Duplicate/over-verbose user label caused crowding on smaller widths | P3 | Fixed (kept test-compatible text visibility rules) |
| Global (Sidebar) | Brand block oversized for dense operations context | P3 | Fixed |

## Regression/Residual Risks
- P2 risk: very long service names + many badges may still wrap into 3+ lines on extra-small screens.
- P2 risk: high-density tables (`alert-rules`) still rely on horizontal scroll for narrow widths.
- P3 risk: module subtitle copy may need final wording pass for operator language consistency.

## Raspberry Validation Checklist (Manual)

Use a hard refresh first (`Ctrl/Cmd + Shift + R`).

### Global
- [ ] Sidebar toggles correctly on desktop and mobile.
- [ ] Active nav state is visible for each module.
- [ ] Header does not overlap controls at mobile width.
- [ ] No broken layout flashes when switching routes with HTMX.

### Dashboard (`/`)
- [ ] Page loads without template/render errors.
- [ ] KPI cards display in stable responsive grid.
- [ ] Timeline chips remain clickable and readable in one or multiple wrapped rows.
- [ ] Charts + operational summary render without overlap.

### Docker (`/docker`)
- [ ] Container rows keep action buttons visible on mobile.
- [ ] Start/Stop/Restart controls are readable and not clipped.
- [ ] Long names/images do not break row structure.

### Services (`/services`)
- [ ] Group headers and service rows keep alignment.
- [ ] Runtime badges (CPU/RAM) wrap gracefully.
- [ ] Action buttons remain reachable without overlap.

### Alerts (`/alerts`)
- [ ] Long alert messages are fully readable (no hard truncation).
- [ ] Severity filter chips wrap cleanly.
- [ ] Acknowledge button remains visible with long text content.

### History (`/history`)
- [ ] Long target/details strings remain readable.
- [ ] Pagination controls remain visible and aligned on mobile.

### Logs (`/logs`)
- [ ] Source selector and fetch button align properly.
- [ ] Log output panel fills available area and scrolls correctly.
- [ ] Error responses remain readable in output pane.

### Settings (`/settings`)
- [ ] Top section navigation chips wrap without overlap.
- [ ] Forms keep label/input alignment across breakpoints.
- [ ] Action buttons are visible and separated from inputs.

## Production Log Sanity Checks (Pi)

Run on Pi:

```bash
journalctl -u ultron-ap --since "10 minutes ago" --no-pager
```

Expected:
- No `Failed to execute template ...`
- No recurring `sse: render error ...`

