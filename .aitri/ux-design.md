# UX Design: Ultron Monitoring Stabilization

## 1. Hero Flow
1. Operator opens Dashboard.
2. Above-the-fold summary shows CPU, RAM, temperature, storage, and service health with clear severity colors.
3. Operator sees a global alert chip in header with count and severity.
4. If healthy, user exits in <10 seconds.
5. If alert exists, click alert chip -> Alerts view filtered to active critical/warning.
6. Operator reviews issue card, opens related module (Docker/Services/Logs) in one click.
7. Action outcome appears as explicit success/error state with retry path.

## 2. Component Innovation
- `Status Ribbon`: compact top ribbon with 3 zones (System, Services, Data Freshness).
- Each zone exposes one primary state and one confidence hint (e.g., `Data age: 4s`).
- Improves mental model by separating "system health" from "data quality".

## 3. State Matrix
- Dashboard:
  - Ideal: live metrics update smoothly, no layout shift.
  - Pre-emptive: skeleton placeholders and "connecting" badge.
  - Failure/Recovery: stale-data banner with last update timestamp + reconnect action.
- Alerts:
  - Ideal: grouped by severity with clear timestamps.
  - Pre-emptive: loading group placeholders.
  - Failure/Recovery: fetch error panel + retry + fallback link to logs.
- Settings:
  - Ideal: deterministic save/apply states (`idle`, `saving`, `applied`, `failed`).
  - Pre-emptive: controls disabled during in-flight action.
  - Failure/Recovery: field-level error and non-destructive retry.

## 4. Accessibility and Inclusivity Audit
- Contrast target >= WCAG AA for all status text on dark backgrounds.
- Full keyboard path for navigation, alert chip, and primary actions.
- Screen-reader labels for status icons, data freshness, and reconnect actions.
- Touch targets >= 44x44 px on mobile.
- Focus visible and persistent across HTMX updates.

## 5. Aesthetic Identity and Consistency
- Keep existing dark visual identity; increase depth using subtle elevation and border hierarchy.
- Typography: consistent heading/body scale with reduced visual noise.
- Color system: semantic tokens (`ok`, `warn`, `critical`, `muted`) reused across pages.
- Component patterns: shared card, badge, button, and empty/error states across modules.

## 6. UX Success Metrics
- Task completion (identify current system state): >= 95%.
- Time to identify critical issue: <= 8 seconds median.
- Navigation interactions to corrective context: <= 2.
- Misclick/error action rate: < 3%.
- Accessibility baseline: WCAG 2.1 AA for core dashboard flows.
