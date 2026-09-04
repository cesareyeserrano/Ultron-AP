# Technical Design Document (TRD / SDD) — NUT_UPS_Pi_On_Dashboard

## Executive Summary

This feature adds UPS monitoring to Ultron-AP by introducing one new Go package, `internal/ups`, that speaks the NUT network protocol to the already-running `upsd` and feeds the existing dashboard, persistence, alert, notify and insights subsystems. **It reuses the established `internal/network` architecture almost verbatim** — poll → sample → event → verdict → alert — because the UPS problem shape (a stateful external source polled on an interval, sampled to SQLite, with transitions that open/close events and raise debounced alerts) is identical to WAN/gateway monitoring already shipped.

Tech choices (all justified as ADRs in Risk Analysis):
- **NUT access:** pure-Go TCP client to `127.0.0.1:3493` (NOT `exec upsc`) — required by `ProtectSystem=full`/`NoNewPrivileges` and by RS-5 testability (ADR-01).
- **Persistence:** two new SQLite tables `ups_samples` + `ups_events`, modelled on the existing `NetSample`/`NetEvent` pair, in the existing DB (ADR-02).
- **Live update:** reuse the existing SSE dashboard channel (`/api/sse/dashboard`), adding a `ups` swap target — no new transport (ADR-03).
- **Battery %:** interpolated estimate from `battery.voltage`, never `battery.charge` (not published) (ADR-04).
- **Dev/mock data:** runtime toggle `ULTRON_UPS_MOCK` pointing the poller at an in-process simulated `upsd`, the same fixture the tests use — powers NFR-022 local rendering and RS-5 tests from one code path (ADR-05).
- **Alerts:** reuse `internal/alerts` + `internal/notify` with new UPS rules; no new alert engine (ADR-06).

No item in the Phase-1 `no_go_zone` is introduced: no shutdown/`load.off` path exists, no config editing, no runtime-minutes, single UPS only.

## System Architecture

```
                        ┌──────────────────────────────────────────────┐
   NUT (host)           │                 ultron-ap (Go)               │
 ┌───────────┐  TCP     │  ┌────────────────────────────────────────┐  │
 │  upsd     │◄─────────┼──┤ internal/ups                           │  │
 │  :3493    │ LIST/GET │  │  client.go  (NUT proto, read-only)     │  │
 │ (powest)  │ read-only│  │  poller.go  (10s loop, backoff,        │  │
 └───────────┘          │  │              unreachable state)        │  │
       ▲                │  │  state.go   (status→state, batt est.)  │  │
       │ ULTRON_UPS_MOCK│  │  store.go   (ups_samples/ups_events)   │  │
 ┌─────┴─────┐  swaps   │  │  snapshot.go(Snapshot for SSE/insights)│  │
 │ mock upsd │──────────┼─►│  mock.go    (simulated upsd fixture)   │  │
 │ (in-proc) │          │  └───────┬───────────────┬───────────┬────┘  │
 └───────────┘          │          │               │           │       │
                        │          ▼               ▼           ▼       │
                        │   internal/database  internal/alerts internal/│
                        │   (SQLite: ups_*)    +internal/notify insights │
                        │          │            (UPS rules→TG)   (vars)  │
                        │          ▼                                     │
                        │   internal/server/sse.go  DashboardData.UPS    │
                        │          │  sse-swap="ups"                     │
                        └──────────┼──────────────────────────────────┘
                                   ▼
                     Browser: UPS metric-tile (FR-017) + shutdown block (FR-023)
```

**Components (all new code lives in `internal/ups` unless noted):**
- `client.go` — opens a TCP session to `upsd`, issues `LIST VAR powest` / `GET VAR`, parses the reply into `map[string]string`. Authenticates with the read-only NUT user. **Read commands only.**
- `poller.go` — ticker (default 10 s), calls the client, on error retries with capped exponential backoff and flips to `unreachable` after the configured timeout (default 2 min); never panics.
- `state.go` — maps `ups.status` tokens (`OL`, `OB`, `LB`, `OL CHRG`, `RB`, `OFF`, `BYPASS`, `ALARM`) to a `State` enum + Spanish label + severity; computes the estimated battery % (FR-018).
- `store.go` — writes samples, opens/closes outage events, purges by retention. Thin wrapper over `internal/database`, mirroring `internal/network`'s store.
- `snapshot.go` — immutable `Snapshot` the SSE layer and insights read.
- `mock.go` — an in-process fake `upsd` the poller connects to when `ULTRON_UPS_MOCK` is set; also the test fixture (RS-5).
- **Wiring (existing packages):** `internal/server/sse.go` gains a `UPS *ups.Snapshot` field on `DashboardData` and a `ups` swap (additive — the named-event SSE pattern already exists for `metrics|charts|verdicts|summary`); `internal/alerts/engine.go` gains a **new `evaluateUPSRule` branch in its `evaluate()` switch** plus DB query methods `RecentUPSSamples`/`RecentUPSEvents` — the engine is a **closed switch** (today only `evaluateMetricRule` + `evaluateNetworkRule`), so UPS rules are NOT "just rows": they need an evaluator branch mirroring the network path (`evaluateNetEvents`/`handleWANEvent`); `internal/insights` reads UPS-derived vars via the existing `EvalWithVars` seam; `internal/config` gains the new env keys; `internal/database/sqlite.go` gains the two `CREATE TABLE` statements and the purge method. These `engine.go` edits are the largest non-`internal/ups` change and are scoped, not incidental.

## Data Model

### Preservation contract (must NOT change)
All existing tables (`User`, `Session`, `Alert`, `AlertConfig`, `NotificationConfig`, `BackupConfig`, `ActionLog`, `NetSample`, `NetEvent`, `lan_devices`, `rules`, `rule_state`, `brute_force_attempts`, `NotificationMute`, `DigestState`, `HardwareConfig`) keep their schema and semantics. The UPS feature is **additive**: new tables + new rows in the existing `rules`/alert flow, no ALTER of existing columns. The `CREATE TABLE IF NOT EXISTS` migration pattern in `sqlite.go` is preserved (no destructive migration).

### Delta — two new tables (modelled on NetSample/NetEvent)

```sql
CREATE TABLE IF NOT EXISTS ups_samples (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    ts           INTEGER NOT NULL,          -- unix seconds
    status       TEXT    NOT NULL,          -- raw ups.status, e.g. "OL", "OB LB"
    state        TEXT    NOT NULL,          -- derived enum: online|onbattery|lowbatt|charging|replace|bypass|off|alarm|unreachable
    load_pct     REAL,                      -- ups.load, nullable when unreachable
    input_v      REAL,                      -- input.voltage
    battery_v    REAL,                      -- battery.voltage
    batt_pct_est REAL                       -- estimated %, NULL when unreachable; always "estimado" in UI
);
CREATE INDEX IF NOT EXISTS idx_ups_samples_ts ON ups_samples(ts);

CREATE TABLE IF NOT EXISTS ups_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    start_ts    INTEGER NOT NULL,           -- transition into OB
    end_ts      INTEGER,                    -- NULL while outage is open
    duration_s  INTEGER,                    -- computed on close
    kind        TEXT NOT NULL DEFAULT 'outage'
);
CREATE INDEX IF NOT EXISTS idx_ups_events_start ON ups_events(start_ts);
CREATE INDEX IF NOT EXISTS idx_ups_events_open  ON ups_events(end_ts);  -- find the single open event
```

**Field constraints / invariants:**
- `state` is derived server-side; the raw `status` is stored verbatim for auditability.
- `batt_pct_est` clamped 0–100; NULL (not 0) when unreachable (FR-017: no zeros).
- At most **one** open event (`end_ts IS NULL`) at a time — the store enforces "close any open event before opening a new one" and, on boot with an already-open event, reconciles instead of double-counting (FR-020 AC-020-003).
- Retention: rows older than `ULTRON_UPS_RETENTION_DAYS` (default 30) purged by the existing metrics purge scheduler pattern.

## API Design

No new **HTTP** endpoints. The card is delivered through the existing authenticated SSE dashboard stream and rendered by a new server-side template partial. Contract preserved: `/api/sse/dashboard` keeps its current swaps; a `ups` swap is **added**, existing swaps unchanged.

### Internal Go package API (`internal/ups`)

```go
// State is the derived UPS state; String() returns the Spanish label.
type State string
const (
    StateOnline State = "online"; StateOnBattery State = "onbattery"
    StateLowBatt State = "lowbatt"; StateCharging State = "charging"
    StateReplace State = "replace"; StateBypass State = "bypass"
    StateOff State = "off"; StateAlarm State = "alarm"; StateUnreachable State = "unreachable"
)

type Snapshot struct {
    State       State
    RawStatus   string
    LoadPct     *float64  // nil when unreachable
    InputV      *float64
    BatteryV    *float64
    BattPctEst  *float64  // nil when unreachable; UI always tags "estimado"
    Beeper      string    // "enabled"|"muted"|"disabled"|""
    DelayShutdown, DelayStart *int // seconds; nil => "no disponible" (FR-023)
    CutoffV     float64   // configured low bound (21.0) shown as "punto de apagado"
    LastGood    time.Time
    Reachable   bool
}

// Client speaks NUT read-only. Source is the real upsd or the mock.
type Client interface { List(ctx context.Context) (map[string]string, error); Close() error }
func Dial(cfg Config) (Client, error)          // real TCP client
func DialMock(fixture MockScript) (Client, error) // in-proc simulated upsd (ADR-05)

// Poller runs the loop and publishes Snapshots.
func NewPoller(c Client, st *Store, cfg Config) *Poller
func (p *Poller) Run(ctx context.Context)      // never panics; sets Unreachable on timeout
func (p *Poller) Current() Snapshot            // latest snapshot for SSE

// Pure helpers (unit-tested directly):
func ParseStatus(raw string) State                     // "OB LB" -> StateLowBatt (LB dominates)
func EstimateBatteryPct(v, low, high float64) float64  // interpolate+clamp (FR-018)

type Store struct{ /* *database.DB */ }
func (s *Store) WriteSample(Snapshot) error
func (s *Store) OpenEvent(ts time.Time) error
func (s *Store) CloseOpenEvent(ts time.Time) (dur time.Duration, err error)
func (s *Store) ReconcileOpenOnBoot() error            // FR-020 AC-020-003
func (s *Store) Purge(before time.Time) (int, error)
func (s *Store) Series(from, to time.Time) ([]Sample, error) // charts (FR-019)
```

The `ups.status`→state precedence rule is explicit (a compound status like `OB LB` resolves to the most severe): `LB > OFF/ALARM > OB/RB/BYPASS > OL CHRG > OL`.

## Implementation Approach

### FR-016 — Native NUT client (MUST, logic)
- **Method:** `Dial` opens `net.Dial("tcp", "127.0.0.1:3493")`, sends `USERNAME`/`PASSWORD`/`LIST VAR powest`, reads until `END LIST VAR`. `Poller.Run` ticks every `PollInterval`.
- **I/O contract:** in = NUT text protocol lines; out = `map[string]string` of variable→value, or `Snapshot{Reachable:false}`.
- **Failure behavior:** on dial/read error, log once, back off (capped exponential), keep last snapshot but mark stale; after `UnreachableTimeout` (2 min) publish `StateUnreachable`. Never `exec`. Never a write/SET/INSTCMD command (compile-time: the client exposes no write method).

### FR-017 — Live UPS card (MUST, UX)
- **Method:** `sse.go` populates `DashboardData.UPS = poller.Current()`; a new `partials/sse-ups.html` renders the `metric-tile` per the UX spec state map; `dashboard.html` adds `sse-swap="ups"`.
- **I/O contract:** in = `Snapshot`; out = escaped HTML fragment pushed over SSE.
- **Failure behavior:** unreachable → "Sin datos" muted tile (never zeros/blank). All NUT-sourced strings pass through Go `html/template` auto-escaping (NFR-019).

### FR-018 — Estimated battery % (MUST, logic)
- **Method:** `EstimateBatteryPct(v, low, high)` = `clamp((v-low)/(high-low)*100, 0, 100)`.
- **I/O contract:** in = battery.voltage + configured bounds; out = float 0–100 wrapped in `*float64`, always surfaced with an "estimado" marker.
- **Failure behavior:** unreachable/absent voltage → nil (UI shows "—"), never fabricated.

### FR-023 — Read-only shutdown display (SHOULD, UX) — realized alongside FR-017
- Delays come from the same `LIST VAR` (`ups.delay.shutdown`/`start`); absent → nil → "no disponible". Cutoff = `cfg.BatteryLowV` (21.0), labelled "punto de apagado — gestionado por NUT". The partial renders **no** form/button/hx-post — enforced by NFR-018.

### FR-021 — UPS alerts (SHOULD, logic) — engine extension
- **Method:** a new `evaluateUPSRule` branch in `internal/alerts/engine.go` (the engine is a closed switch — ADR-06), reading `RecentUPSSamples`/`RecentUPSEvents`. OB/LB/RB/battery-near-cutoff/input-out-of-range/unreachable **fire alerts** through the existing `CreateAlert`+dispatcher path with the engine's cooldown/dedup/mute. The **OL-return "Info with duration"** uses the engine's **resolve seam** (`emitResolve`, notification-only, no Alert row — same as `handleWANEvent`), with duration = `end_ts − start_ts` from `ups_events`. Debounce/once-per-day/once-per-outage reuse the network `rule_state`/processed-event pattern.
- **Failure behavior:** a mains flicker inside the debounce window fires nothing; unreachable fires at most once per outage.

### MUST NFRs
- **NFR-016 (clean degradation):** module init is guarded by `ULTRON_UPS_ENABLED`; poller failures never bubble to boot. Existing tiles unaffected (isolated goroutine).
- **NFR-017 (no new privileges):** only a localhost TCP dial; no `ultron-helper`/`privileged` call added.
- **NFR-018 (no shutdown path):** `Client` interface has no write method; a `grep`-able test asserts the package references no `shutdown`/`load.off`/`INSTCMD`/`SET VAR`.
- **NFR-019 (escaping + secret):** template auto-escaping; NUT creds read from env via `internal/config`, encrypted at rest with `ULTRON_SECRET_KEY` if ever persisted, never logged.
- **NFR-020 (observability):** bounded reconnect logging (log on transition, not per-poll).
- **NFR-021 (CI):** `go test ./internal/ups/...` covers parser + state map + estimator against `mock.go`.

## Security Design
- **AuthN to NUT:** dedicated read-only NUT user (`ULTRON_NUT_USER`/`ULTRON_NUT_PASS`) in `/etc/nut/upsd.users`, distinct from `homeassistant` (RS-1). Credentials loaded by `internal/config` from `/etc/ultron-ap/ultron-ap.env`; encrypted at rest if stored in DB; never in repo/logs/chat.
- **AuthZ / blast radius:** the panel’s existing single-admin session + CSRF (FR-007/FR-012) still gate the dashboard; the UPS card is read-only and adds no state-changing route.
- **Input validation:** every value from NUT is untrusted → Go `html/template` contextual escaping on render; numeric fields parsed with `strconv` and rejected to nil on parse failure (no injection into SQL — parameterized queries only).
- **XSS:** explicitly no `innerHTML`/JS string interpolation of NUT values (the toast XSS regression must not return) — server-rendered escaped partial only.
- **Shutdown safety:** no code path can issue `shutdown.*`/`load.off`; enforced by the read-only `Client` interface + a static assertion test (NFR-018).
- **Mock safety:** `ULTRON_UPS_MOCK` is unset in the production systemd unit; when unset the poller can only reach the real read-only endpoint (ADR-05 consequence).

## Performance & Scalability
- **Poll cost:** one short-lived TCP request every 10 s; negligible on ARM (well within the ~15 MB idle budget, NFR of parent). No browser-side polling — SSE push reuses the existing stream.
- **Write volume:** ~1 sample/10 s ≈ 8.6k rows/day; 30-day retention ≈ 260k rows — same order as `NetSample`, which the DB already handles. Indexed on `ts`.
- **Query bounds:** chart queries are windowed (24 h / 7 d) and `LIMIT`-bounded; the series query uses `idx_ups_samples_ts`.
- **Purge (corrected — verified against code):** there is NO generic config-driven purge scheduler to piggyback. `internal/metrics` is an in-memory ring buffer (no SQLite, no purge); `db.PruneNetSamples` exists but is not scheduled in production; the only live job, `startRetentionJob` (`server.go`), hardcodes `PruneOldData(30)` for ActionLog/Alert/sessions and ignores config. So this feature must **add `db.PruneUPSSamples(days)`/`PruneUPSEvents(days)` and wire a call into `startRetentionJob` honoring `ULTRON_UPS_RETENTION_DAYS`** (FR-024). Small, but real new wiring — not a free reuse.
- **Single-node:** one UPS, one poller goroutine — no concurrency fan-out, no locking beyond the existing DB handle.

## Deployment Architecture
- **Model:** **native single statically-linked Go binary under systemd on the Raspberry Pi** — unchanged from the parent project. NOT containerized (Docker deploy is in the parent `no_go_zone`). `make build-arm` continues to produce the linux/arm64 binary (parent NFR-002/003 preserved).
- **New config (env, `/etc/ultron-ap/ultron-ap.env`):** `ULTRON_UPS_ENABLED`, `ULTRON_NUT_USER`, `ULTRON_NUT_PASS`, `ULTRON_UPS_POLL_SECONDS`, `ULTRON_UPS_BATT_LOW_V`, `ULTRON_UPS_BATT_HIGH_V`, `ULTRON_UPS_RETENTION_DAYS`, alert-threshold keys, and dev-only `ULTRON_UPS_MOCK`.
- **Ops step (documented, out of band):** create the read-only NUT user in `/etc/nut/upsd.users` and reload `upsd`. Not automated by the panel (RS-2).
- **Environments:** dev (Mac, `ULTRON_UPS_MOCK=1`, `make run`) → prod (Pi, real `upsd`, mock unset). CI runs `go test ./internal/ups/...` on every push to main (NFR-021).

## Risk Analysis

Top risks:
1. **Compound/again-unknown NUT status flags** (e.g. `OB LB`, or a Q1 quirk) mis-mapped → wrong card/alert. *Mitigation:* explicit precedence rule + table-driven tests over every documented flag combo against `mock.go`.
2. **Battery estimate mistaken for a real gauge** → user over-trusts it. *Mitigation:* always tagged "estimado"; runtime-minutes stays out of scope; ADR-04 records the honesty constraint.
3. **Unreachable flapping** spamming alerts/events. *Mitigation:* debounce + "unreachable at most once per outage" (FR-021); single-open-event invariant (FR-020).
4. **Mock leaking into prod** → fake data shown as real. *Mitigation:* mock gated by an env var absent in the prod unit; ADR-05; a test asserts mock is off by default.
5. **Regression in existing dashboard/SSE** from wiring edits. *Mitigation:* additive `DashboardData` field + isolated goroutine; regression NFRs (NFR-016/017) with tests.

### ADRs

**ADR-01: NUT access method**
- Option A — `exec upsc`: simple, but needs the binary, breaks under `ProtectSystem=full`/`NoNewPrivileges`, harder to unit-test.
- Option B — native Go TCP client: pure Go, testable against a mock, no privileged helper.
- **Decision: B.** Consequences: full control over the protocol + one code path for tests and mock; must implement a small NUT parser (bounded, ~1 file).

**ADR-02: Persistence**
- Option A — new `ups_samples`/`ups_events` tables in the existing SQLite (NetSample/NetEvent pattern).
- Option B — in-memory ring buffer only (like `metrics/ringbuffer`).
- **Decision: A.** Consequences: history survives restart and answers "outages this month" (FR-020); costs two tables + a purge job. NB: the purge is **new wiring**, not an existing solved job — see Performance & Scalability (a `PruneUPSSamples` added to `startRetentionJob`).

**ADR-03: Live-update transport**
- Option A — reuse the existing SSE dashboard channel, add a `ups` swap.
- Option B — a dedicated WebSocket/endpoint for the UPS.
- **Decision: A.** Consequences: zero new transport, consistent with every other tile; the card is coupled to the dashboard stream cadence (acceptable — it’s a dashboard tile).

**ADR-04: Battery percentage**
- Option A — display `battery.charge` from the device.
- Option B — estimate by interpolating `battery.voltage` between 21.0–27.4 V.
- **Decision: B.** The device does not publish `battery.charge`. Consequences: always labelled "estimado"; orientation-grade only; runtime-minutes excluded.

**ADR-05: Dev/mock data source**
- Option A — a Go build tag / separate mock binary.
- Option B — a runtime env toggle (`ULTRON_UPS_MOCK`) that points the poller at an in-process simulated `upsd`, shared with the test fixture.
- **Decision: B.** Consequences: one fixture powers both RS-5 tests and NFR-022 local rendering; must ensure the toggle is absent in the prod unit (documented + tested). **Runtime state selection (NFR-022 AC):** the mock must let the developer drive the card through states on demand — `ULTRON_UPS_MOCK` accepts a state value (e.g. `ULTRON_UPS_MOCK=OB`, `=LB`, `=unreachable`; `=1` cycles OL→OB→LB→OL on a timer) so every card state is visible in a local browser without a real UPS.

**ADR-06: Alert integration**
- Option A — a new UPS-specific alert subsystem.
- Option B — extend the existing `internal/alerts` engine + `internal/notify` dispatcher with UPS evaluators.
- **Decision: B.** Consequences: inherits cooldown/dedup/mute/Telegram-storm handling already built. **Important correction (verified against `internal/alerts/engine.go`):** the engine is a *closed dispatch switch* — `evaluate()` today handles only `evaluateMetricRule` (cpu/ram/disk/temp) and `evaluateNetworkRule` (latency/loss/dns/wan/ip). A UPS `AlertConfig` row inserts fine (no CHECK on `metric`) but would be **silently ignored** with no matching `case`. So UPS alerts require a real `evaluateUPSRule` branch + `RecentUPSSamples`/`RecentUPSEvents` DB queries, mirroring the network evaluators — not merely new rows. This is planned engine surgery, bounded to the switch and the network-style event handling (OB opens, OL emits a resolve with duration — see FR-021 note below).

## Technical Risk Flags

**[RISK] NUT protocol parser is new hand-written code — severity: medium**
The `nutdrv_qx`/Q1 device and NUT 2.8.1 wire format must be parsed correctly, including compound `ups.status`. *Stack compatibility:* fully compatible (pure Go `net` + text parsing). *Mitigation:* table-driven tests over documented variables/flags against `mock.go`; parser isolated in `client.go`/`state.go`. *Accept?* low blast radius (bounded, well-tested, read-only).

**[RISK] Mock-data mode could be enabled in production — severity: medium**
`ULTRON_UPS_MOCK` feeding fake data as real would mislead the operator. *Mitigation:* absent from the prod systemd env; default off; a test asserts the default and the prod unit template omits it. *Accept?* yes, with the test guard.

**[RISK] FR-022 insights cannot render dynamic numbers with the current engine — severity: low**
`Verdict.VerdictText` is a fixed `bundled.json` string; `EvalWithVars` fires on a boolean but does not interpolate values. FR-022 (NICE) therefore ships a *static* verdict, not "4 cortes esta semana". *Mitigation:* accept the static verdict for now; a parameterized-verdict engine extension is a separate future requirement. *Accept?* yes — FR-022 is NICE; flagged for the reviewer to accept or defer.

**[RISK] Alert engine + purge scheduler are extensions, not reuse — severity: medium**
Verified against `engine.go` (closed switch) and `server.go` (`startRetentionJob` hardcodes 30d, ignores config; no generic purge). FR-021 needs a real `evaluateUPSRule` branch + DB queries; FR-024 retention needs a new `PruneUPSSamples` wired into the retention job. *Mitigation:* both are bounded and mirror the network pattern; scoped explicitly in §Components/§Performance/ADR-06. *Accept?* yes — planned, not incidental; the risk was an inaccurate "free reuse" description, now corrected.

**[RISK] Battery estimate accuracy — severity: low**
Lead-acid voltage→charge is non-linear; the estimate is orientation-grade. *Mitigation:* always "estimado", no runtime-minutes (design constraint, not a bug). *Accept?* yes, by design.

**[RISK] Existing SSE/dashboard regression from wiring — severity: low**
*Stack compatibility:* additive change to `DashboardData` + one partial. *Mitigation:* isolated goroutine, regression NFRs with tests, module gated by `ULTRON_UPS_ENABLED`. *Accept?* yes.

Overall stack × requirements: **fully compatible** — Go + SQLite + existing SSE/alerts/notify/insights; no new runtime, no new privilege, no container. No critical/high flags.

## Failure Blast Radius

- **`internal/ups` poller dies / panics (contained):** runs in its own goroutine started at boot; a recovered panic logs and the card falls to "Sin datos". Blast radius: the UPS tile only — CPU/RAM/Docker/systemd/network tiles and all controls keep working (NFR-016). Recovery: poller restarts on next tick / process restart.
- **`upsd` down or UPS unplugged:** poller → `unreachable`, card → "Sin datos", one "UPS sin comunicación" alert; no log flood (NFR-020); no crash (parent AC "no se cae ni llena el log"). Recovery: auto-reconnect with backoff; recovery logged once.
- **SQLite write failure (shared dependency):** UPS store surfaces the error and skips the sample; it does not corrupt or block the shared DB handle used by metrics/alerts. Blast radius: UPS history gap only.

## Traceability Checklist
- Every FR addressed: FR-016→client/poller; FR-017→sse-ups partial; FR-018→EstimateBatteryPct; FR-019→ups_samples+Series; FR-020→ups_events+Reconcile; FR-021→`evaluateUPSRule` engine branch + notify (OB fires, OL emits resolve+duration); FR-022→insights `EvalWithVars` (see note); FR-023→shutdown block in partial; FR-024→config keys + `PruneUPSSamples` wiring. ✓
- **FR-022 mechanism note (NICE, verified):** the insights engine's `Verdict.VerdictText` is a **fixed string** from `bundled.json` — `EvalWithVars` fires a rule on a boolean trigger but does **not interpolate dynamic values**. So FR-022 as-is delivers a *static* verdict (e.g. "hubo varias interrupciones esta semana" / "el voltaje de batería en reposo está bajando"), NOT the literal count/number in AC-022-001/002. Rendering the actual number ("4 cortes") would require a parameterized-verdict extension to the insights engine, which is out of this feature's mechanism. Since FR-022 is NICE, ship the static verdict; flagged as a Technical Risk Flag for the reviewer to accept or defer.
- Every NFR addressed: NFR-016/017/018/019/020/021/022 → Implementation Approach + Security + Deployment. ✓
- Every ADR has ≥2 options. ✓
- No `no_go_zone` item introduced (no shutdown path, no config editing, no runtime-minutes, single UPS, no HA duplication, no container deploy). ✓
- Failure blast radius documented for ≥2 critical components (poller, upsd-down, SQLite). ✓
