## Feature
Typed pickers, visible validation ranges, and structural cleanup for the settings page — addresses BL-020 and the 9 BL-020-tagged findings from the 2026 Q2 UI audit (BL-019).

## Problem / Why
The settings page is the weakest UI surface in Ultron-AP. Eleven numeric fields across Alert Rules, Performance, Backup, and Schedule are raw `<input type="number">` with no unit-aware widget. Allowed ranges are encoded only in HTML `min`/`max` attributes — users only learn bounds via round-trip server errors. Severity is a `<select>` instead of a colour-coded segmented control. Backup config crams 12+ inputs into one undivided section. The top anchor-chip strip scrolls but does not expand the target accordion. Section number badges (`01`…`07`) imply a wizard flow that doesn't exist. Settings is a navigation surface masquerading as a form wall.

## Target Users
The single Pi operator (admin) configuring alert thresholds, notification channels, backup schedule, and performance intervals. Same persona as today — no new user type.

## New Behavior
- The system must replace all numeric interval/threshold/timeout inputs with a stepper + unit-dropdown widget where the unit (sec/min/hr) is part of the same control.
- The system must render a single `<input type="time">` for backup schedule (replacing the two adjacent hour/minute number inputs).
- The system must offer chip-preset rows (1m / 5m / 15m / 1h) with a "custom" escape hatch for alert-rule cooldowns.
- The system must render the allowed range inline next to every numeric field label (e.g. "Dashboard refresh (2–60 sec)").
- The system must replace the severity `<select>` with a 3-button segmented control where each segment carries the actual severity colour (critical / warning / info).
- The system must convert the email and Telegram "Enabled" checkboxes into toggle switches.
- The system must wire the top anchor-chip strip to expand the target accordion section on click (not just scroll).
- The system must subdivide the Backup section into three labelled sub-groups: Limits, Schedule, Destination.
- The system must hide the per-section form-state pill when state is `idle`, and only render it during `saving` / `applied` / `failed` transitions.
- The system must remove the `01`–`07` section number badges (they imply a non-existent wizard flow).
- The system must move Logout out of the System Controls grid into the header dropdown (Logout is benign; Restart/Shutdown are destructive — they should not share visual weight).
- The system must apply a stronger destructive treatment (red-tinted card border) to Shutdown only, not Restart.
- The system must drop the inner `max-w-4xl` form constraint so forms span the same width as the `max-w-5xl` shell.
- The system must replace Spanish strings on the settings header and sidebar tooltip with English.
- The system must offer a two-field composite picker (scheme `env` / `kms` / `file` + value) for the backup encryption key reference, with live "resolved at runtime" feedback.

## Success Criteria
- Given a user opens settings, when they view any numeric input, then the allowed range is visible inline next to the label without trial-and-error.
- Given a user clicks an anchor chip at the top of settings, when the page scrolls to the target section, then the matching accordion expands automatically.
- Given a user opens the alert-rule form, when they pick a severity, then they see a 3-segment colour-coded control — no dropdown click required.
- Given a user views the System Controls section, when they look for Logout, then it is no longer present (moved to header).
- Given a user submits a numeric value out of range, when the server validates, then the inline error references the same range shown next to the label (no surprise round-trip).
- Given a user opens the Backup section, when they scan it, then they see three sub-headings (Limits / Schedule / Destination) instead of an undivided 14-field grid.
- Given the form is `idle`, when the user views any section header, then no form-state pill is rendered (zero clutter at rest).
- All 9 BL-020-tagged audit findings resolved; no regressions on FR-007 (auth), FR-009 (dark-mode contrast), FR-012 (CSRF).

## Out of Scope
- Visual redesign / token theme overhaul (audit explicitly excludes a11y + visual redesign).
- Changes to non-settings pages (services, docker, alerts, history, network, dashboard, logs) — those audit findings are separate tickets.
- Telegram message content / formatting — separate follow-up feature.
- Insights-engine rule threshold exposure as first-class settings (mentioned in BL-020; deferred until insights-engine field telemetry warrants it).
- Search/filter for settings (BL-020 mentions; deferred — current page fits one viewport with chip nav, search not yet justified).
- Settings versioning / change history.
- Migrating existing stored settings values (schema unchanged — only input widgets change).
