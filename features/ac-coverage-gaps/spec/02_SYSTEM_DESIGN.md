# System Design — ac-coverage-gaps

Realises FR-079 (Telegram mute window), FR-080 (daily email digest), FR-081 (per-service log drawer), FR-082 (fan mode), FR-083 (OLED configuration).

## Executive Summary

No new technology enters the stack. Every FR here is a *gap in an existing subsystem*, so the design is deliberately conservative: extend what exists, add nothing that would need its own operational story on a Raspberry Pi.

| Decision | Choice | Justification |
|---|---|---|
| Mute state | New singleton SQLite table `NotificationMute` (`id=1`) | The mute must survive a restart (AC-079-004) and must be readable from the notify dispatcher without a config round-trip. It cannot live inside the `NotificationConfig.config` JSON: that blob is **encrypted** (BG-044 / `ULTRON_SECRET_KEY`), and a mute window is not a secret — putting a plaintext-relevant, frequently-read expiry behind the secret box would mean decrypting on every alert and would fail closed (silently muting) if the key were missing. A dedicated table lets the read fail *open*. |
| Mute enforcement point | `Dispatcher.send()`, filtering the Telegram notifier out of the fan-out for that event | This is the single choke point every fire passes through (`Dispatch` → queue → `run` → `send`). Enforcing here — rather than inside `TelegramSender.Notify` — keeps the notifier a dumb transport, leaves the FR-024 storm cache untouched (NFR-085), and guarantees mute cannot accidentally suppress the email channel (no-go zone). |
| Digest config | Two new keys in the existing `NotificationConfig("email").config` JSON: `digest_enabled`, `digest_hour` | The digest *is* an email-channel setting; it is saved by the same form, encrypted by the same box, and read by the same `GetNotificationConfig("email")` the email notifier already calls. A separate table would split one channel's configuration across two stores. |
| Digest de-duplication | New singleton table `DigestState` (`id=1`, `last_sent_date TEXT` as `YYYY-MM-DD`) | "At most one per calendar day" (AC-080-002) needs durable state; a date string compared against the local date is the smallest thing that survives restarts and hour-boundary ticks. |
| Digest scheduler | One goroutine with a `time.Ticker`, reusing the pattern of the existing backup scheduler and retention job | The Pi already runs several such loops; a cron library would be a dependency for one job. Tick interval: 1 minute — cheap (a date compare and, at the digest hour, one SELECT), and it bounds "sent at the digest hour" to ≤60s of skew. |
| Log drawer | New route `GET /api/services/{name}/logs` → `privileged.SystemLogs(ctx, "service:"+name, 100)` | The 100-line fetch, the helper's Unix socket, the allow-list validation and the logfilter redaction **all already exist** (`handleFetchSystemLogs` → helper `journalctl -u <unit> -n 100`). The drawer is a second, row-scoped caller of the same pipeline — no new privileged surface (NFR-088). |
| Hardware config | New singleton table `HardwareConfig` (`id=1`) | Same shape as `BackupConfig` (`id INTEGER PRIMARY KEY CHECK (id = 1)`), which is the established pattern for "one row of panel settings". One read on settings render, one write on save — the owner's hard constraint that this cost the Pi nothing (FR-082 AC, no-go zone). |
| Frontend | Existing htmx + `settings.js` + widget vocabulary | The hardware section is a seventh accordion section built by the same client controller; the drawer is pure htmx attributes. No page-level inline `<script>` returns to any template (CSS7 / NFR-087). |

## System Architecture

```
                    ┌──────────────────────────────────────────────────────────┐
                    │                    ultron-ap (panel)                      │
                    │                                                          │
  browser           │  ┌────────────┐   ┌──────────────┐   ┌────────────────┐  │
  ───────hx-get────▶│  │ handlers_  │   │ handlers_    │   │ handlers_      │  │
  /services         │  │ services   │   │ settings     │   │ notifications  │  │
   [Logs] btn       │  │            │   │              │   │                │  │
                    │  │ NEW:       │   │ NEW:         │   │ NEW:           │  │
                    │  │ handle     │   │ handleHard   │   │ handleMute /   │  │
                    │  │ ServiceLogs│   │ wareSave     │   │ handleMuteClear│  │
                    │  └─────┬──────┘   └──────┬───────┘   └───────┬────────┘  │
                    │        │                 │                   │           │
                    │        │                 ▼                   ▼           │
                    │        │        ┌────────────────────────────────────┐   │
                    │        │        │           internal/database        │   │
                    │        │        │  NEW: HardwareConfig  (id=1)       │   │
                    │        │        │  NEW: NotificationMute(id=1)       │   │
                    │        │        │  NEW: DigestState     (id=1)       │   │
                    │        │        │  existing: NotificationConfig,      │   │
                    │        │        │            Alert, ActionLog         │   │
                    │        │        └───────▲──────────────▲─────────────┘   │
                    │        │                │              │                 │
                    │        │       ┌────────┴─────┐  ┌─────┴──────────────┐  │
                    │        │       │ notify.      │  │ NEW: notify.       │  │
                    │        │       │ Dispatcher   │  │ DigestScheduler    │  │
                    │        │       │              │  │ (1-min ticker)     │  │
                    │        │       │ send():      │  │  reads Alerts 24h  │  │
                    │        │       │  NEW mute    │  │  → EmailNotifier   │  │
                    │        │       │  filter ─────┼──┤  → ActionLog       │  │
                    │        │       └──────┬───────┘  └────────────────────┘  │
                    │        │              │ (telegram dropped while muted)   │
                    │        │              ▼                                  │
                    │        │       Telegram / Email notifiers (unchanged)    │
                    │        │                                                 │
                    │        ▼ privileged.SystemLogs(ctx, "service:<n>", 100)  │
                    └────────┼─────────────────────────────────────────────────┘
                             │ Unix socket (existing, allow-listed)
                    ┌────────▼─────────┐
                    │  ultron-helper   │  journalctl -u <unit> -n 100
                    │  (root)          │  ← unit name validated by serviceNameRe
                    └──────────────────┘
```

Components introduced: **3 tables**, **1 goroutine** (digest scheduler), **3 handlers**, **1 template partial** (`settings-hardware.html`) + a drawer block inside `services-list.html`. Everything else is reuse.

## Data Model

### Preservation contract — MUST NOT change

- `NotificationConfig` — schema untouched. The `config` JSON for channel `email` gains two OPTIONAL keys (`digest_enabled`, `digest_hour`); a config written before this feature (without them) must still load, with the digest treated as disabled. The `telegram` config is not touched at all.
- `Alert`, `AlertConfig` — untouched. Mute suppresses *delivery*, never persistence (AC-079-002): the engine keeps calling `CreateAlert`, and the alert-count SSE event keeps counting.
- `ActionLog` — schema untouched; the digest and mute write new *rows* using the existing `LogAction(userID, source, action, target, result, details)` signature.
- `BackupConfig`, `User`, `Session`, `NetSample`, `NetEvent`, `lan_devices`, `rules`, `rule_state`, `brute_force_attempts` — untouched.
- The `CREATE TABLE IF NOT EXISTS` migration block is append-only: new tables are appended, so an existing `ultron.db` upgrades in place with no data migration and no downtime.

### Delta

```sql
-- FR-079. Singleton. Absent row (or a corrupt/NULL expiry) == not muted (fail-open, NFR-090).
CREATE TABLE IF NOT EXISTS NotificationMute (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    expires_at DATETIME NOT NULL,          -- UTC; the window is open while expires_at > now()
    hours      INTEGER  NOT NULL,          -- 1 | 4 | 24 — what the admin picked, for the UI label
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- FR-080. Singleton. last_sent_date is the LOCAL calendar date (YYYY-MM-DD) of the last digest.
CREATE TABLE IF NOT EXISTS DigestState (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    last_sent_date TEXT NOT NULL DEFAULT '',
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- FR-082 / FR-083. Singleton, mirrors BackupConfig's shape.
CREATE TABLE IF NOT EXISTS HardwareConfig (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    fan_mode     TEXT    NOT NULL DEFAULT 'auto',   -- auto | quiet | performance | off
    oled_enabled INTEGER NOT NULL DEFAULT 0,        -- 0 | 1
    oled_metric  TEXT    NOT NULL DEFAULT 'cpu',    -- cpu | ram | temp | ip
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Field constraints (enforced in Go, not only by the schema, so the HTTP layer can return 400 with a field-mapped message):

| Field | Constraint | On violation |
|---|---|---|
| `NotificationMute.hours` | ∈ {1, 4, 24} | 400; nothing persisted |
| `NotificationMute.expires_at` | `now + hours` (±60s tolerance asserted in tests) | — |
| `DigestState.last_sent_date` | `YYYY-MM-DD` or `""` | unparseable ⇒ treated as "never sent" (digest may send once; never suppresses) |
| `HardwareConfig.fan_mode` | ∈ {auto, quiet, performance, off} | 400; stored value unchanged (AC-082-003) |
| `HardwareConfig.oled_metric` | ∈ {cpu, ram, temp, ip} | 400; stored value unchanged (AC-083-003) |
| `NotificationConfig(email).digest_hour` | integer 0–23 | 400 with an inline field error on the hour input |

Defaults are chosen so that an upgraded database behaves exactly as it does today: no mute row (⇒ not muted), `digest_enabled` absent (⇒ disabled), `HardwareConfig` row created on first read with `fan_mode=auto, oled_enabled=0, oled_metric=cpu`.

## API Design

### Contract being preserved

These stay byte-compatible — they have passing tests today and NFR-085/086/087 forbid changing them:

| Endpoint | Preserved behaviour |
|---|---|
| `POST /api/notifications/telegram` | Saves bot token + chat id; response `Saved successfully`. Gains the OPTIONAL `mute_hours` field; a POST without it changes no mute state. |
| `POST /api/notifications/email` | Saves the six SMTP fields. Gains OPTIONAL `digest_enabled` / `digest_hour`; a POST without them leaves the digest as it was — **not** reset (the partial-save semantics established by BG-061). |
| `POST /api/notifications/test` | Unchanged. **A test send is NOT muted** — the admin explicitly asked for it; mute suppresses *alerts*, not a manual connectivity check. |
| `GET /logs`, `GET /api/logs` | Unchanged page-level source dropdown (FR-010). |
| `POST /api/services/{name}/{start,stop,restart}` | Unchanged. |
| `GET /api/settings/backup` | Unchanged (the encrypted-listing route is deferred — out of scope). |

### New / changed endpoints

| Method | Path | Auth | Request | Response | Errors |
|---|---|---|---|---|---|
| `GET` | `/api/services/{name}/logs` | session (`requireAuth`) | path param `name` | `200` HTML fragment: the drawer body (monospace `<pre>`, escaped, redacted) | `400` invalid unit name (allow-list) · `302` → `/login` unauthenticated · `200` + error-state fragment when the helper is unavailable (a *rendered* error, not a 5xx — the drawer is an htmx swap target and must show a message, AC-081-004) · `200` + empty-state fragment when the journal is empty |
| `POST` | `/api/notifications/telegram` | session + CSRF | existing fields **+ optional** `mute_hours` ∈ {1,4,24} | `200` HTML status fragment | `400` invalid `mute_hours` · `403` bad CSRF |
| `POST` | `/api/notifications/mute/clear` | session + CSRF | — | `200` HTML fragment: the Telegram section's mute row, back in its chip-preset state | `403` bad CSRF |
| `POST` | `/api/notifications/email` | session + CSRF | existing fields **+ optional** `digest_enabled` (`on`/absent), `digest_hour` (0–23) | `200` HTML status fragment | `400` `digest_hour` out of range · `403` bad CSRF |
| `POST` | `/api/settings/hardware` | session + CSRF | `fan_mode`, `oled_enabled` (`on`/absent), `oled_metric` | `200` HTML status fragment | `400` invalid mode/metric · `403` bad CSRF |

### Internal package API (new exported surface)

```go
// internal/database — mute (FR-079)
func (db *DB) SetNotificationMute(hours int, now time.Time) (time.Time, error) // returns expires_at
func (db *DB) ClearNotificationMute() error
func (db *DB) NotificationMuteUntil() (expiresAt time.Time, muted bool, err error) // fail-open: err ⇒ muted=false

// internal/database — digest de-dup (FR-080)
func (db *DB) DigestLastSentDate() (string, error)
func (db *DB) MarkDigestSent(date string) error
func (db *DB) AlertsSince(t time.Time) ([]Alert, error)   // powers the 24h summary

// internal/database — hardware (FR-082 / FR-083)
type HardwareConfig struct {
    FanMode     string
    OLEDEnabled bool
    OLEDMetric  string
}
func DefaultHardwareConfig() HardwareConfig
func (db *DB) GetHardwareConfig() (HardwareConfig, error)  // creates the row on first read
func (db *DB) SaveHardwareConfig(c HardwareConfig) error   // validates the enums

// internal/notify — digest (FR-080)
type DigestScheduler struct{ /* db, notifier factory, clock */ }
func NewDigestScheduler(db *database.DB) *DigestScheduler
func (s *DigestScheduler) Start(ctx context.Context)
func (s *DigestScheduler) Stop()
func (s *DigestScheduler) Tick(now time.Time) error // exported for tests: one scheduler evaluation
```

`Tick(now)` taking the clock as a parameter is the whole testability story for FR-080: AC-080-001/002/003/004 are all "given this stored state and this instant, does exactly one email go out?" — no sleeping, no wall-clock flake.

## Implementation Approach

### FR-079 — Telegram mute window

- **Method.** `Dispatcher.send()` gains one guard before the fan-out loop:
  ```go
  notifiers := d.getNotifiers()
  if muted, _ := d.telegramMuted(); muted {
      notifiers = withoutChannel(notifiers, "telegram")   // email et al. still fan out
  }
  ```
  `telegramMuted()` calls `db.NotificationMuteUntil()`. The Settings Telegram form carries a chip-preset (`mute_hours`); `POST /api/notifications/mute/clear` deletes the row.
- **I/O contract.** In: `hours ∈ {1,4,24}` from the form; the current time. Out: `expires_at = now + hours` persisted; the section re-renders showing the remaining time. The dispatcher reads one row per fire — a singleton PK lookup, sub-millisecond on the Pi.
- **Failure behaviour.** `NotificationMuteUntil` **fails open**: any DB error, a missing row, or an unparseable expiry ⇒ `muted=false`, so a broken mute never silently swallows an alert (NFR-090). The error is logged once per fire at most. A muted fire still calls `CreateAlert` and still increments the SSE alert count — mute is delivery-only (AC-079-002).
- **Storm interaction.** The mute filter removes the notifier *before* `Notify` is called, so the FR-024 storm cache is never touched for a muted event; when the window expires, coalescing resumes with its existing state (NFR-085).

### FR-080 — Daily email digest

- **Method.** `notify.DigestScheduler` runs a 1-minute ticker. Each `Tick(now)`:
  1. Load the email `NotificationConfig`; if the channel is disabled or `digest_enabled` is false ⇒ return (AC-080-004).
  2. If `now.Hour() != digest_hour` ⇒ return.
  3. If `DigestLastSentDate() == now.Format("2006-01-02")` ⇒ return (AC-080-002).
  4. `alerts := db.AlertsSince(now.Add(-24h))`; render the summary (severity · source · timestamp per alert, or the explicit "no alerts fired" body when empty, AC-080-003); send through the **existing** SMTP notifier.
  5. `MarkDigestSent(today)` and `LogAction(nil, "digest", "send", "email", result, details)`.
- **I/O contract.** In: stored email config + `now` + the last 24h of alerts. Out: exactly one SMTP send; one `DigestState` row update; one `ActionLog` row.
- **Failure behaviour.** An SMTP error is logged, recorded in `ActionLog` with `result=failed` (NFR-091), and **`last_sent_date` is still marked** — a broken relay must not turn the digest into a retry storm that hammers SMTP every minute for an hour. The process never panics (NFR-090). Per-event alert emails are untouched: the digest is a separate caller of the same notifier (AC-080-005 / NFR-086).
- **Ordering note.** `MarkDigestSent` after a *successful* send would risk re-sending on a slow SMTP timeout at :59; marking on completion (success or failure) is the safer trade and is what the AC's "at most once per calendar day" demands.

### FR-081 — Per-service log drawer

- **Method.** New handler `handleServiceLogs`: read `{name}` from the path, build `source := "service:" + name`, reuse `isValidLogSource` **and** the helper's own `serviceNameRe` allow-list (defence in depth — the panel validates, and the root helper validates again, AC-081-002), then `s.privileged.SystemLogs(ctx, source, 100)`. Render `partials/service-logs.html` with the (already logfilter-redacted) output. Template auto-escaping handles the HTML (AC-081-003).
- **I/O contract.** In: unit name from the rendered row. Out: an HTML fragment with the last 100 lines, newest at the bottom, inside `max-h-96 overflow-y-auto`. The row's `[Logs]` button is `hx-get` with `hx-target` the row's drawer div — no fetch fires until it is clicked (AC-081-005).
- **Failure behaviour.** Helper unavailable ⇒ `200` with the error-state fragment ("Could not read logs — the privileged helper is unavailable"). Empty journal ⇒ the empty-state fragment. Invalid unit name ⇒ `400` before any helper call. Unauthenticated ⇒ `requireAuth` redirects to `/login`.
- **Why not reuse `handleFetchSystemLogs` directly:** it renders the *page-level* log panel and takes `source` from a query string. Sharing the fetch (`privileged.SystemLogs`) while keeping a row-scoped handler avoids widening the query-string surface into a path-param one for the existing route, which has its own tests.

### FR-082 / FR-083 — Hardware section

- **Method.** `GetHardwareConfig()` on settings render (one row); `handleHardwareSave` validates `fan_mode` and `oled_metric` against their closed sets, then `SaveHardwareConfig`. The section is a new partial `settings-hardware.html` built from the existing widget vocabulary (`data-widget="segmented"` ×2, `data-widget="toggle"` ×1) and registered in `templates.go` alongside the other six settings partials, so `settings.js` binds it with no new client code.
- **I/O contract.** In: three form fields + CSRF. Out: one `HardwareConfig` row; the settings page renders the stored values as selected.
- **Failure behaviour.** Invalid enum ⇒ `400`, stored value unchanged (AC-082-003 / AC-083-003). Missing CSRF ⇒ `403`, nothing persisted. A missing row on read ⇒ created with defaults (never an error to the user).
- **Explicit non-behaviour.** Nothing in this feature reads `HardwareConfig` outside the settings page. No goroutine, no I2C, no GPIO, no daemon connection — the owner's constraint. The section carries the visible note "Ultron stores these settings; it does not drive the fan or OLED panel yet." This is **declared technical debt** for Phase 4: the actuator is a separate future feature.

## Security Design

- **Auth.** Every new route sits behind the existing `requireAuth` middleware (session cookie → `GetSession`, which already filters expired sessions). The log drawer is a `GET` and needs no CSRF; all three new/extended `POST`s go through the existing CSRF middleware (`403` on a bad token — AC-082-004, AC-083-004).
- **Input validation.** Closed enums (`fan_mode`, `oled_metric`), a bounded integer (`digest_hour` 0–23), a bounded enum (`mute_hours` ∈ {1,4,24}). None of these values is ever interpolated into a shell command, a SQL string (parameterised statements only), or a template as raw HTML.
- **Command injection (NFR-088).** The drawer's unit name is validated twice: `isValidLogSource` in the panel, and `serviceNameRe` inside the root helper — the helper is the authority and already rejects option-like and injection-shaped names before invoking `journalctl` (this is the existing `TestServiceNameRe_RejectsOptionLikeNames` guarantee). The drawer adds **no new privileged action**: it calls the `logs` action that already exists on the socket.
- **XSS.** Journal output is attacker-influenceable (a malicious process can write anything to the journal). It is rendered through `html/template`, which escapes it, into a `<pre>` — never through `innerHTML` and never through the toast path (which is `textContent`-only since the CSS2 fix).
- **Secret leakage.** The journal tail passes through the existing `logfilter` redaction before rendering, so a bot token or password that leaked into the journal is not re-exposed in the browser (AC-081-003). The digest email body contains alert severity/source/message/timestamp — the same fields the per-event email already sends — and no configuration values.
- **Mute cannot be used to hide an attack.** A mute suppresses Telegram delivery only; the alert is still persisted, still visible in `/alerts`, still counted in the header badge, and the mute itself is recorded in `ActionLog` (NFR-091). An attacker with a session could mute Telegram — but they could already disable the channel outright, so this widens nothing.
- **CSP.** No inline `<script>` is added to any template; the drawer is htmx attributes and the hardware section binds through `settings.js`. The existing `script-src 'self' 'unsafe-inline'` policy moves no further from being enforceable.

## Performance & Scalability

Target hardware: Raspberry Pi (ARM64, limited RAM). The owner's explicit constraint is that this feature not repeat the CPU/memory cost of a previous hardware-control attempt.

| Path | Cost | Bound |
|---|---|---|
| Mute check on every alert fire | one singleton-PK SELECT (`WHERE id=1`) | Alerts fire at human frequency (a storm is coalesced by FR-024). No caching needed; adding a cache would introduce a staleness window in which a cancelled mute keeps suppressing. |
| Digest scheduler | one ticker wake-up per minute; 99.9% of ticks return after a config read + an hour compare | The 24h alert query runs **once per day**, bounded by `AlertsSince` + the existing retention policy (old alerts are pruned by the retention job, so the result set cannot grow unbounded). |
| Log drawer | one helper round-trip per click, 100 lines | Explicitly not a live tail (no-go zone): a follow would keep a journalctl process and a connection alive per open drawer — exactly the class of cost the owner rejected. Nothing is fetched until the admin clicks. |
| Hardware section | one SELECT on settings render, one UPDATE on save | Zero background cost, zero hardware I/O. This is the FR's own acceptance criterion. |
| Settings page render | +1 partial, +1 singleton SELECT | Negligible; the page already renders six sections and reads several config rows. |

New goroutines: **one** (the digest scheduler), joining the collector, docker/systemd monitors, alert engine, network probes, LAN orchestrator, retention job and backup scheduler that already run. It is stopped via context on shutdown like the others.

Query optimisation: `AlertsSince` uses the existing `created_at` ordering on `Alert`; the table is retention-pruned. Singleton tables need no index beyond their PK.

## Deployment Architecture

**Deployment model: native binary + systemd.** Not containerised — this is the deployment the project rejected Docker for (Phase 5 rejection, 2026-03-18) and it stays unchanged.

- Two binaries, as today: `ultron-ap` (unprivileged panel) and `ultron-helper` (root, Unix socket). **This feature changes neither binary's boundary**: no new helper action, no new privilege.
- Schema: the new tables are appended to the existing `CREATE TABLE IF NOT EXISTS` migration block. Deploying is `make build-arm`, copy, restart — the tables are created on first start. **No data migration, no downtime, and a rollback is safe**: an older binary simply ignores the three new tables (it never reads them), and the two new keys in the email config JSON are unknown fields it will drop only if it re-saves that form.
- Environments: dev (macOS, `go run`, ad-hoc SQLite) and prod (Pi 192.168.1.29, `/opt/ultron-ap`, systemd units). CSS is a committed artifact — the new section uses only classes already present in the existing markup (segmented/toggle/chip vocabulary), but `make css` runs anyway and CI should fail on a diff.
- CI: the existing GitHub Actions workflow (build · vet · race-test · arm64 · lint). The new tests join `go test ./...`; the digest scheduler's `Tick(now)` seam keeps them clock-free, so no test sleeps.

## Risk Analysis

**ADR-1 — Mute lives in its own table, not in the encrypted channel config.**
Context: `NotificationConfig.config` is AES-encrypted (`ULTRON_SECRET_KEY`). Putting `mute_until` there means the send path decrypts on every fire, and a missing/rotated key makes the read fail. Decision: a dedicated plaintext singleton table. Consequence: mute state is readable even with a broken key, and the read can **fail open** (deliver) rather than fail closed (silently swallow). A mute expiry is not secret material — nothing is lost by storing it in the clear.

**ADR-2 — The mute filter sits in the dispatcher, not in the Telegram notifier.**
Alternative considered: an early return inside `TelegramSender.Notify`. Rejected: it would run *after* the storm cache had already recorded the event, entangling mute with FR-024 coalescing (NFR-085 forbids changing that), and it would put channel-selection policy inside a transport. The dispatcher already owns "which notifiers get this event".

**ADR-3 — `MarkDigestSent` is written on completion, not only on success.**
A failed SMTP send that left `last_sent_date` unset would make the scheduler retry every minute until the hour rolled over — 60 sends at a broken relay. Marking on completion caps it at one attempt per day; the failure is visible in `ActionLog` and the journal instead.

| # | Risk | Severity | Mitigation |
|---|---|---|---|
| 1 | Mute fails *closed* on a DB error and silently swallows a critical alert. | high | `NotificationMuteUntil` returns `muted=false` on any error, missing row, or unparseable value. Tested explicitly (a corrupt row ⇒ delivery proceeds). |
| 2 | The digest re-sends in a loop when SMTP is down (60/hour). | medium | ADR-3: mark-on-completion. Test: a failing notifier still results in exactly one attempt within the hour. |
| 3 | A digest that sends *nothing* when there are no alerts is indistinguishable from a broken digest. | medium | AC-080-003 mandates an explicit "no alerts fired" email. It is a requirement precisely because silence is ambiguous. |
| 4 | The log drawer becomes a way to run arbitrary journalctl arguments. | high | The unit name never reaches a shell: the helper builds `journalctl -u <unit> -n 100` with the name validated against `serviceNameRe` (rejects `-`-leading, `;`, spaces). Validated in the panel *and* in the root helper. No new helper action is added. |
| 5 | The hardware section makes the admin believe the fan is being controlled. | medium | The section carries a visible scope note, and the FR/no-go zone declare the actuator as deferred technical debt. Phase 4 must declare it in `technical_debt`. |
| 6 | The new settings section breaks the existing accordion / form-state controller. | medium | It reuses the existing widget vocabulary and is registered like the other six partials; NFR-087 is a regression test (settings markup suite passes unchanged, no page-level inline script). |

## Technical Risk Flags

[RISK] Digest scheduling accuracy is bounded by the tick interval, not the clock
Conflict: FR-080 requires the digest "at the digest hour", but a 1-minute ticker means the send lands anywhere within that hour's first minute — and a machine suspended across the hour boundary could miss the window entirely.
Mitigation: the tick checks `now.Hour() == digest_hour` (not an exact instant), so any tick inside the hour sends; the `last_sent_date` guard makes the send idempotent for that day. A Pi suspended for the whole hour would skip that day's digest — accepted: it is a summary email, not an alert, and the alerts themselves were already delivered per-event.
Severity: low

[RISK] SQLite single-writer contention between the digest scheduler and the alert engine
Conflict: NFR-090 requires the scheduler never to destabilise the process, but SQLite allows one writer; the digest's `MarkDigestSent` write could collide with a burst of `CreateAlert` writes during a storm.
Mitigation: the digest writes twice a day (one `DigestState` update, one `ActionLog` row) and the connection already sets `busy_timeout=5000` with WAL journaling. The read (`AlertsSince`) does not block writers under WAL. Contention is bounded and the existing brute-force UPSERT test shows the pool tolerates far heavier concurrency.
Severity: low

[RISK] Mute is global and session-authenticated — anyone with the panel session can silence Telegram
Conflict: NFR-088's spirit (no new privilege) vs. FR-079 giving any authenticated user a delivery kill-switch.
Mitigation: accepted. This is a single-admin panel; the same session can already disable the Telegram channel entirely, so the mute grants no capability that did not exist. The mute is recorded in `ActionLog`, is visible in the UI while open, and never suppresses persistence or the in-panel alert view.
Severity: low

[RISK] The hardware section persists values nothing consumes
Conflict: FR-082/FR-083 deliver controls whose values no actuator reads — a user-visible promise the system does not keep.
Mitigation: the owner explicitly required this (a previous hardware-control attempt cost the Pi significant CPU/memory), the no-go zone declares it, the UI carries a scope note, and Phase 4 must record it in `technical_debt` so Phase 5 compliance sees it. The persisted schema is exactly what a future actuator would read, so no rework is created.
Severity: medium
