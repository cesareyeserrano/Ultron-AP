# UX/UI Feedback Log - 2026-02-27

Feature: `ux-ui-upgrade`
Source: User feedback (manual session notes)
Scope: Documentation only (no functional changes applied)

## Summary

This document captures UI/UX and behavior feedback reported for the current web app experience. Items are grouped by area and include expected outcomes to guide implementation.

## Feedback Items

### Sidebar / Branding

FB-UI-01
- Issue: Brand label shows `Ultron` instead of uppercase.
- Requested change: Display `ULTRON`.
- Expected result: Sidebar logo text is fully uppercase.

FB-UI-02
- Issue: Branding subtitle has typo and inconsistent wording: `Rasberry PI Dashboard`.
- Requested change: Replace with `Raspberry Dashboard`.
- Expected result: Correct spelling and consistent subtitle.

FB-UI-03
- Issue: Raspberry icon appears in subtitle line and should be removed.
- Requested change: Remove Raspberry icon from subtitle area.
- Expected result: Subtitle is text-only.

FB-UI-04
- Issue: Sidebar toggle is misplaced and responsive behavior is inconsistent when collapsing.
- Requested change: Fix toggle placement and collapsed/expanded responsive behavior.
- Expected result: Toggle is accessible and state transitions are stable on desktop/tablet/mobile.

FB-UI-05
- Issue: Logo size is too small.
- Requested change: Set Ultron icon to `100px x 100px`.
- Expected result: Sidebar logo renders at `100x100` without breaking layout.

### Header / User Identity

FB-UI-06
- Issue: Current header user label shows `admin`.
- Requested change: Show `Usuario Administrador: César Augusto`.
- Expected result: Header displays requested user identity text.

### Alerts / History Actions

FB-BUG-01
- Issue: `Clear Alerts` action is reported as not working.
- Requested change: Fix clear alerts flow.
- Expected result: Alerts are cleared correctly and UI updates without errors.

FB-BUG-02
- Issue: `Clear History` action is reported as not working.
- Requested change: Fix clear history flow.
- Expected result: History records are cleared correctly and UI updates without errors.

FB-BUG-03
- Issue: Multiple confirm popups appear when triggering clear history action.
- Requested change: Ensure only one confirmation prompt per action.
- Expected result: Single confirmation dialog and single request submission.

FB-UX-01
- Issue: Need unread notification bubble for alerts.
- Requested change: Show notification badge when there are unread/unacknowledged alerts.
- Expected result: Badge updates in real-time and hides when count is zero.

### Services View

FB-UI-07
- Issue: Service group cards have mixed colors that reduce consistency.
- Requested change: Use blue styling for all groups by default; only show red when there is an error/failure.
- Expected result: Consistent blue visual system with red reserved for error state.

FB-UI-08
- Issue: Info (`i`) tooltips are empty or not useful.
- Requested change: Add minimum meaningful description for each service (what it is / what it does).
- Expected result: Tooltip always shows actionable description text.

### Logs

FB-ENH-01
- Issue: Logs page has limited sources.
- Requested change: Add sources such as memory, CPU, Pironman, Home Assistant, and keep list extensible as new services are added.
- Expected result: Logs source list can grow dynamically with available services and relevant system sources.

### Settings

FB-UI-09
- Issue: Settings screen needs better usability and optimization.
- Requested change: Improve settings UI layout and interaction efficiency.
- Expected result: Faster navigation, clearer grouping, reduced visual clutter.

### Home / Dashboard

FB-UI-10
- Issue: Home accordion sections are collapsed by default.
- Requested change: Keep accordions expanded by default.
- Expected result: Main dashboard sections visible on first load.

FB-ENH-02
- Issue: Timeline charts lack timeframe controls.
- Requested change: Add range options: `week`, `24h`, `12h`, `60 min`, and current minimum interval.
- Expected result: User can switch time windows and chart data updates accordingly.

## Suggested Implementation Phases

Phase 1 (Critical UX/Bugs)
- FB-BUG-01, FB-BUG-02, FB-BUG-03, FB-UI-04

Phase 2 (Branding and Visual Consistency)
- FB-UI-01, FB-UI-02, FB-UI-03, FB-UI-05, FB-UI-06, FB-UI-07

Phase 3 (Productivity Enhancements)
- FB-UX-01, FB-UI-08, FB-ENH-01, FB-UI-09, FB-UI-10, FB-ENH-02

## Notes

- This entry is documentation-only.
- No runtime behavior was modified as part of this documentation step.
