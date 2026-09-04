## Feature
Build the six behaviors the approved requirements already promise but the product never implemented: a Telegram mute window, a daily email digest, a per-service log drawer, a hardware section in Settings (fan mode + OLED display), and a backup list with encrypted download.

## Problem / Why
`aitri verify-complete` gates on acceptance-criteria coverage. An audit of the main pipeline (BL-033) found 41 acceptance criteria with no test. 35 turned out to be real behaviors that were merely untested (now covered) — but 6 traced to behavior the requirements demand and the code never grew:

- **AC-005-005** — the admin can pick a mute window (1h / 4h / 24h); no Telegram message is sent while it is open. Nothing in `internal/notify` implements mute/silence; the only suppression is the 60s storm coalescing window (FR-024), which is a different mechanism.
- **AC-006-004** — with the daily digest enabled, one email summarising the last 24h of alerts is sent at the digest hour. `grep -ri digest internal/` returns nothing.
- **AC-010-002** — a service row opens a log drawer showing the last 100 lines from journalctl. The substance exists (`handleFetchSystemLogs` → `privileged.SystemLogs(ctx, source, 100)` → `journalctl -u <unit> -n 100`), but only as the page-level `/logs` dropdown; service rows have no drawer.
- **AC-013-002 / AC-013-003** — the Settings page shows a hardware section with a fan-mode selector and an OLED display configuration. No hardware section exists at all.
- **AC-015-004** — Settings lists prior backups and clicking download delivers the encrypted file. The encryption primitive exists (`encryptFileAESGCM`, ULTRONENC2, exercised by `backup_crypto_test.go`) and the automated backup flow uses it, but the only download route (`GET /api/settings/backup`) makes a FRESH plaintext SQLite snapshot and serves it as `application/x-sqlite3`. Prior backups are neither listed nor downloadable.

[ASSUMPTION] The user's intent is that these six ship as originally specified, not that the requirements were aspirational — confirmed on 2026-07-13 when they chose "implement all six via the feature pipeline" over amending the requirements.

## Target Users
The single admin operating the Raspberry Pi panel. No new user types.

## New Behavior
- The system must let the admin open a mute window of 1h, 4h or 24h, and must suppress Telegram sends for its duration, resuming automatically when it expires.
- The system must let the admin enable a daily digest with a digest hour, and must send exactly one email at that hour summarising the previous 24h of alerts.
- The system must let the admin open a log drawer on any systemd service row, showing the last 100 journalctl lines for that unit, fetched through the privileged helper.
- The system must render a hardware section in Settings containing a fan-mode selector and an OLED display configuration, persisting both.
- The system must list prior backups in Settings, and must deliver the stored encrypted backup file (not a fresh plaintext snapshot) when the admin clicks download.

## Success Criteria
- GIVEN a 1h mute window is open, WHEN an alert fires, THEN no Telegram message is sent; WHEN the window expires, THEN sends resume. (AC-005-005)
- GIVEN the daily digest is enabled, WHEN the digest hour is reached, THEN exactly one email summarising the last 24h of alerts is sent. (AC-006-004)
- GIVEN a systemd service row, WHEN the admin opens its log drawer, THEN the last 100 journalctl lines for that unit are shown. (AC-010-002)
- GIVEN the Settings page, WHEN the hardware section renders, THEN a fan-mode selector and an OLED display configuration are present and persist. (AC-013-002, AC-013-003)
- GIVEN Settings lists prior backups, WHEN the admin clicks download, THEN the encrypted backup file is delivered to the browser. (AC-015-004)
- `aitri verify-complete` passes with zero untested acceptance criteria.

## Touch Points
MODIFIES:
- FR-005 (Telegram) — `internal/notify/dispatcher.go`, `internal/notify/telegram.go`: consult the mute window before sending.
- FR-006 (Email) — `internal/notify/email.go` + a new digest scheduler.
- FR-010 (Logs) — `internal/server/handlers_system.go` (`handleFetchSystemLogs` already fetches 100 lines), `web/templates/partials/services-list.html`.
- FR-013 (Hardware) — `web/templates/partials/` (new `settings-hardware.html`), `internal/server/templates.go`, `internal/server/handlers_settings.go`.
- FR-015 (Backup) — `internal/server/handlers_performance.go` (`handleSettingsBackup` currently serves a plaintext snapshot), `web/templates/partials/settings-maintenance.html`.
- `internal/database/` — new persisted config for mute window, digest hour, fan mode, OLED.
- `scripts/aitri-test.sh` + `spec/03_TEST_CASES.json` — new TC emit lines traced by `ac_id`.

PURELY ADDS: backup listing endpoint, digest scheduler, log-drawer endpoint.

## Must Not Break (Regression Boundary)
- Telegram alerts (FR-005) keep sending immediately when NO mute window is open; the FR-024 60s storm coalescing keeps working independently of mute.
- Per-event email alerts (FR-006) keep sending on every fire; the digest is additive, not a replacement.
- The `/logs` page (FR-010) keeps working as a page-level source dropdown.
- Every existing Settings section (alerts, telegram, email, performance, backup, maintenance, system controls) keeps saving; the accordion, form-state pills, widgets and CSRF gating (FR-057..FR-070) keep working, including after hx-boost swaps.
- The privileged helper (FR-011) keeps validating every unit name against its allow-list; no new host action bypasses the Unix socket.
- Backups stay encrypted at rest (AC-015-003) and the automated schedule keeps running; the existing `GET /api/settings/backup` snapshot path must not start leaking an unencrypted file to a new caller.
- All 80 currently-passing test cases keep passing.

## Out of Scope
- Physically driving the Pironman fan or writing to the OLED panel (the ACs require the Settings controls and their persistence; actuating the hardware needs the Pironman daemon on the Pi and is deferred — declare it as technical debt).
- Per-rule or per-source mute (the AC specifies a global 1h/4h/24h window only).
- Digest scheduling granularity beyond a single daily hour (no weekly/custom cron).
- Backup retention/pruning changes — the existing retention job stays as-is.
- Restoring a backup from the listing (download only).
