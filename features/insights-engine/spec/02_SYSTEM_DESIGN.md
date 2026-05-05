# 02_SYSTEM_DESIGN — Insights Engine (feature)

## Executive Summary

`insights-engine` is added as a single new internal Go package (`internal/insights`) inside the existing Ultron-AP monolith. It is a declarative rules evaluator that consumes the same `metrics.Snapshot` the dashboard already receives, applies a bundled set of boolean conditions, and emits a current-verdict array onto the existing dashboard SSE channel as a new `verdicts` event type.

Design priorities, in order:

1. **Same monolith, no new process.** The engine is a goroutine inside `ultron-ap`, started from `cmd/ultron-ap/main.go` after the metrics collector. No new binary, no new systemd unit.
2. **No new datastore.** Two new SQLite tables (`rules`, `rule_state`) appended to the parent `schema` const in `internal/database/sqlite.go` — same convention as `lan_devices`. No second DB file. Rule definitions themselves are `go:embed`-ed JSON, not stored in the DB.
3. **Strict architectural separation from notify / alert engine (NFR-021).** The `internal/insights` package never imports `internal/notify` or `internal/alerts`. Verdicts are diagnostic context; alerts are notifications. Two responsibilities, two packages, one direction of data flow (neither imports the other).
4. **Reuse the existing SSE channel.** No new HTTP route is added for streaming; verdicts ride the same dashboard `EventSource` that already carries `metrics`, `charts`, `summary`, and `alert-count` events. A new event type `verdicts` is the entire delta.
5. **No new ticker.** Evaluation is driven by the parent FR-001 5 s metrics cadence — the SSE broadcast loop calls `engine.Eval(snapshot)` inline and appends a `verdicts` SSE frame to the same broadcast payload it already builds.
6. **Pre-compiled rule AST cached at startup.** Each rule's JSON condition is parsed once into a callable closure (`func(*EvalCtx) bool`) at load time and reused per tick. Steady-state evaluation does zero allocation (NFR-016).

## System Architecture

```mermaid
graph TD
  subgraph Browser
    es["Dashboard EventSource\n(existing /sse subscription)"]
    frag["Operational Indicators\nHTMX fragment"]
  end

  subgraph Ultron-AP monolith (single process)
    main["cmd/ultron-ap/main.go"]
    coll["internal/metrics.Collector\n(5 s tick, FR-001)"]
    broker["internal/server.sseBroker\n(broadcast loop)"]
    eng["internal/insights.Engine\n(stateless tick fn + per-rule state)"]
    rules["internal/insights/rules\n(go:embed bundled.json + AST cache)"]
    store["internal/insights/store\n(rule_state CRUD)"]
    alerts["internal/alerts.Engine\n(unrelated — FR-014)"]
    notify["internal/notify.Dispatcher\n(unrelated — FR-014)"]
  end

  db[("SQLite\n(parent file + rules + rule_state tables)")]

  main -->|insights.Start ctx, db, collector| eng
  coll -->|Latest Snapshot| broker
  broker -->|snapshot| eng
  eng --> rules
  eng --> store
  store --> db
  eng -->|VerdictSet| broker
  broker -->|event: verdicts JSON| es
  es -->|sse-swap| frag

  alerts -.->|isolated| notify
  eng -. "DOES NOT IMPORT" .- alerts
  eng -. "DOES NOT IMPORT" .- notify
```

Component responsibilities:

| Component | Path | Responsibility | Key types |
|---|---|---|---|
| `insights` (root) | `internal/insights/` | Public API: `Start(ctx, db, *metrics.Collector) (*Engine, error)`, `Eval(*metrics.Snapshot) []Verdict`, `Stop()`. Per-rule state (last value, transition counts, first_emitted_at). | `Engine`, `Verdict`, `EvalCtx` |
| `insights/lang` | `internal/insights/lang/` | JSON → AST parser, AST → closure compiler, `Eval(ctx) bool`. Sustained-window operator state. | `Expr`, `Compiled`, `SustainedState` |
| `insights/rules` | `internal/insights/rules/` | `go:embed bundled.json`. `Load() ([]Rule, error)`. Strict-schema decoder rejecting unknown fields. | `Rule`, `Severity` |
| `insights/store` | `internal/insights/store/` | All `rules` + `rule_state` SQLite reads/writes. Seeds `rules` from bundled set on startup. | `Store` |
| `insights/api` | `internal/insights/api/` | `GET /api/insights/verdicts` JSON handler. HTMX fragment renderer for the Operational Indicators section. | `Handler` |
| `internal/metrics.Collector` | (existing) | Already produces `Snapshot` on the 5 s tick. No change. | `Snapshot` |
| `internal/server.sseBroker` | (existing) | Broadcast loop in `internal/server/sse.go`. Extended to call `engine.Eval` and append a `verdicts` SSE frame. | `sseBroker` |
| `internal/database/sqlite.go` | (existing) | `schema` const gains two tables. Migration runs in existing `New()`. | (no new types) |
| `cmd/ultron-ap/main.go` | (existing) | Add `insights.Start(ctx, db, collector)` after collector / before alert engine. | — |
| `web/templates/dashboard.html` | (existing) | Add `<section id="operational-indicators">` above metrics grid; HTMX listens for `sse:verdicts`. | — |

No parent package is forked. `internal/alerts` and `internal/notify` are untouched.

## Data Model

All persistence reuses the existing SQLite file (FR-015). Schema is appended to the parent `schema` const in `internal/database/sqlite.go` so the existing `New()` migration path picks it up — same convention used for `lan_devices`.

```sql
CREATE TABLE IF NOT EXISTS rules (
  id              TEXT    PRIMARY KEY,                     -- stable rule id (matches bundled JSON)
  title           TEXT    NOT NULL,
  condition_json  TEXT    NOT NULL,                        -- canonical JSON of the condition AST
  severity        TEXT    NOT NULL CHECK (severity IN ('info','warn','critical')),
  verdict_text    TEXT    NOT NULL,
  recommendation  TEXT    NOT NULL,
  links_json      TEXT    NOT NULL DEFAULT '[]',           -- JSON array, never null (FR-040 AC-005)
  enabled         INTEGER NOT NULL DEFAULT 1,              -- 0/1
  source          TEXT    NOT NULL DEFAULT 'bundled'
                  CHECK (source IN ('bundled','user')),    -- 'user' reserved for v2; v1 only writes 'bundled'
  created_at      INTEGER NOT NULL,                        -- ms epoch UTC
  updated_at      INTEGER NOT NULL                         -- ms epoch UTC
);
CREATE INDEX IF NOT EXISTS idx_rules_enabled_severity
  ON rules(enabled DESC, severity);

CREATE TABLE IF NOT EXISTS rule_state (
  rule_id                 TEXT    PRIMARY KEY REFERENCES rules(id) ON DELETE CASCADE,
  last_evaluated_at       INTEGER NOT NULL DEFAULT 0,      -- ms epoch UTC of last tick this row saw
  last_value              INTEGER NOT NULL DEFAULT 0,      -- last condition truthiness (0/1) — FR-046 hysteresis
  last_change_at          INTEGER NOT NULL DEFAULT 0,      -- ms epoch UTC of last false→true or true→false flip
  transitions_in_window   INTEGER NOT NULL DEFAULT 0,      -- transitions in current 10 s flap window (FR-046)
  first_emitted_at        INTEGER NOT NULL DEFAULT 0       -- ms epoch UTC of current run of true (FR-042)
);
```

Notes:

- `rules` is **seeded** at startup from the `go:embed`-ed bundled set: insert-or-update rows whose `source='bundled'`. The `enabled` column is preserved across upgrades (it is the only operator-mutable field in v1; v1 has no UI editor per no-go-zone, but the persistence is in place for v2).
- `condition_json` is the canonical serialised AST — written from the loader, read by the API for debugging. The hot path uses the in-memory compiled closure, never re-parses the JSON.
- The `idx_rules_enabled_severity` index supports the engine's startup load query (`WHERE enabled=1` ordered by severity for stable verdict ordering) and the API's read.
- `rule_state` is **per-rule, bounded** — exactly one row per loaded rule. Its rowsize is fixed; total table size is bounded by rule count, not tick count, satisfying NFR-017's 100 KB ceiling.
- Sustained-window state (per-tick history for the `sustained:` operator) lives **in memory only** — a small ring buffer per rule. Crossing a process restart resets the window (acceptable: a sustained 30 s rule simply needs 30 s after restart to fire again). This avoids a high-write-volume table that would dominate backup size.
- Verdict history is explicitly NOT persisted (per no-go-zone — no time-series view in v1).

## Condition Language

Rule conditions are JSON-encoded boolean expressions. The grammar is intentionally tiny so the parser is one file and so a rule is auditable at a glance.

### Grammar

| Node | Shape | Meaning |
|---|---|---|
| Literal — variable | `{ "var": "cpu_pct" }` | Read a named telemetry variable from the eval context. |
| Literal — constant | `{ "const": 90 }` (number, string, or bool) | Inline value. |
| Comparison | `{ "op": "gt", "left": <expr>, "right": <expr> }` | One of `eq, ne, gt, gte, lt, lte`. |
| Logical | `{ "op": "and", "args": [<expr>, <expr>, ...] }` | One of `and, or, not`. `not` takes `args` of length 1. |
| Sustained window | `{ "sustained": { "var": "cpu_pct", "op": "gt", "value": 90, "window_ms": 30000 } }` | True iff the inner comparison has been continuously true for `window_ms` of consecutive ticks. |

Operator precedence is fixed by the JSON tree shape; there is no infix surface, so no operator-precedence ambiguity exists.

### Example — happy path

`cpu_pct > 90 AND temp_c > 75`:

```json
{
  "op": "and",
  "args": [
    { "op": "gt", "left": { "var": "cpu_pct" }, "right": { "const": 90 } },
    { "op": "gt", "left": { "var": "temp_c" }, "right": { "const": 75 } }
  ]
}
```

### Example — sustained window

`sustained(cpu_pct > 90, 30s)`:

```json
{
  "sustained": {
    "var": "cpu_pct",
    "op": "gt",
    "value": 90,
    "window_ms": 30000
  }
}
```

### Missing variables (FR-041)

The eval context exposes a typed lookup `Get(name) (value, present bool)`. When a rule references a variable whose feed is unavailable (e.g. `lan_device_count` while the lan-devices feature is disabled, or any variable on a tick where the collector returned an error), the lookup returns `present=false`. Any comparison with a missing operand yields `false` — never an error, never a panic. The whole rule therefore degrades to "not firing" rather than disabling the engine. The first time the engine encounters a missing variable for a given rule, it logs `skipped-missing-var rule_id=... var=...` exactly once per process lifetime (rate-limited via a `sync.Map[ruleID]struct{}`).

### Compilation

At engine startup, each rule's condition tree is walked once and converted into a closure of type `func(*EvalCtx) bool`. Sustained-window nodes capture their per-rule ring buffer by reference. The closure is stored on the in-memory `Rule` struct; the hot path on every tick is one closure call per enabled rule — zero allocation, no map lookup of the operator name.

Malformed conditions (unknown op, wrong-type literal, etc.) are rejected at compile time with a structured log line; the rule is dropped from the active set and the engine continues with the rest (FR-040 AC-001 / FR-041 AC-003).

## API Design

### `GET /api/insights/verdicts`

- **Auth:** session cookie required (parent FR-007). No CSRF on GET.
- **Response 200:** JSON array of currently-active verdicts, sorted critical → warn → info, then within each severity by `first_emitted_at` desc:

  ```json
  [
    {
      "rule_id": "thermal_throttle_probable",
      "title": "Thermal throttling probable",
      "severity": "critical",
      "verdict_text": "CPU sustained >90% with temperature >75 °C — the SoC will throttle shortly.",
      "recommendation": "Reduce load: kill the heaviest CPU consumer or improve cooling.",
      "links": [],
      "first_emitted_at": "2026-05-05T17:32:08Z",
      "last_evaluated_at": "2026-05-05T17:33:53Z"
    }
  ]
  ```

- **Response 401:** unauthenticated (parent-standard auth-redirect for HTML, JSON 401 for `Accept: application/json`).
- **Empty state:** `[]` (never `null`).

This endpoint exists for two reasons: (a) for synchronous loads of the dashboard fragment before the SSE stream warms, and (b) for an eventual `curl`-able diagnostic path. Live updates do **not** use this endpoint — they ride the SSE channel below.

### SSE event — new `verdicts` event type on the existing channel

The dashboard's existing `EventSource` (registered in `internal/server/sse.go`) already receives `metrics`, `charts`, `summary`, and `alert-count` events. The insights engine adds **one** new event type, emitted on the same broadcast cycle:

```
event: verdicts
data: <html-fragment of the Operational Indicators section, server-rendered from the verdict slice>

```

The payload is **HTML**, not JSON, matching the SSE convention already used by the other dashboard events (`partials/sse-metrics.html`, `partials/sse-summary.html`). The fragment is rendered by `partials/sse-verdicts.html` from the verdict slice on each tick, identical idempotency semantics to the other event types — each event is the full active set, never a delta. HTMX `sse-swap="verdicts"` replaces the section atomically.

### Wire-up

- The existing broadcast loop in `internal/server/sse.go` (`startSSEBroadcast` → `buildSSEPayloadWithOptions`) is extended to call `s.insights.Eval(snapshot)` and `writeSSEEvent(&buf, "verdicts", verdictsHTML)` on every tick. No new ticker, no new broker, no new HTTP route.
- The synchronous `GET /api/insights/verdicts` route is registered in `internal/server/server.go` next to the other `/api/*` handlers, behind the existing `authMiddleware`.

## Security Design

- **Auth inheritance.** `GET /api/insights/verdicts` and the SSE channel both go through the parent `authMiddleware` (FR-007). An unauthenticated SSE subscriber is rejected before any verdict event is written — verdicts are not leaked.
- **No new write surface in v1.** v1 has no API endpoint that accepts a rule definition as request body. Rule definitions are bundled at build time via `go:embed`; there is no filesystem watcher, no rule-upload endpoint, no plug-in interface (NFR-019). Future POST endpoints (enable/disable toggle) will follow parent FR-012 CSRF — they are out of scope here.
- **No XSS surface.** Verdict text and recommendations are server-rendered into the HTMX fragment via Go's `html/template` (the same auto-escaping path the other partials use). The verdict-link list is a fixed set bundled in JSON, never reflected from request input.
- **Strict isolation from notify (NFR-021) — import boundary.** The `internal/insights` package and its sub-packages (`lang`, `rules`, `store`, `api`) MUST NOT import:
  - `github.com/cesareyeserrano/ultron-ap/internal/alerts`
  - `github.com/cesareyeserrano/ultron-ap/internal/notify`
  - any sub-package of either (`notify/email`, `notify/telegram`, etc.)

  This is the architectural codification of NFR-021. A `go list -deps ./internal/insights/...` import-graph assertion in CI (or a `forbidigo`/`depguard` lint rule) verifies the boundary on every build. **Architecture decision: "verdicts are not alerts."** Verdicts are read-only diagnostic context displayed in the dashboard; alerts are state-changing notifications dispatched through Telegram / email. The two responsibilities are owned by different packages and the data flow is one-way: the insights engine reads `metrics.Snapshot` and writes to the SSE broker; it never calls into `alerts` or `notify`. This decision is referenced by name from NFR-021's third acceptance criterion.

- **No outbound network calls.** The engine's import set contains no HTTP client, no SMTP client, no Telegram bot library. Verified by inspection of the package's `go list -deps` output.
- **Data sensitivity.** Verdict payloads contain no secrets, no MACs, no identifiers beyond what the dashboard already shows. Backups (FR-015) cover the new tables under the existing `ULTRON_BACKUP_KEY` envelope without configuration changes.

## Performance & Scalability

| Resource | Budget | How met |
|---|---|---|
| Per-tick evaluation latency | ≤5 ms p99 for the bundled 10 rules | Pre-compiled closures (one per rule); no JSON parsing on hot path; arithmetic-only inner loop. Each `sustained:` node is an O(1) ring-buffer push + count. |
| Per-tick latency p99 (full path) | ≤50 ms (NFR-016) | 5 ms eval + ≤10 ms HTMX render + ≤5 ms SSE serialisation = ≤20 ms typical, well under 50 ms. |
| Allocations per tick (steady-state) | 0 | Working set pre-allocated at `Start()`: a fixed `[]Verdict` slice sized to `len(rules)`, a fixed `EvalCtx` reused per tick, fixed ring buffers per `sustained:` node. The HTML fragment is rendered into a re-used `bytes.Buffer` from the broker. |
| CPU 5-min average attributable to engine | ≤1% (NFR-016) | At 0.2 Hz tick rate × ≤5 ms per tick = 0.1% CPU ceiling on a single core. Bursty load spikes (e.g. ICMP backoff in a sibling subsystem) do not affect the engine — it is a synchronous in-process call on the SSE broadcast goroutine. |
| RSS overhead | ≤5 MB (NFR-016) | 10 rules × (~2 KB AST + ~256 B state) ≈ 25 KB. Bundled JSON ≈ 4 KB. Re-used buffers ≈ 64 KB. Comfortable margin. |
| Parent FR-001 tick jitter | ≤100 ms p99 (NFR-016) | Engine evaluation is bounded by the per-tick budget above; the SSE broker already absorbs the existing render cost (`sse-summary` is heavier than the verdict fragment). No additional jitter source. |
| DB growth | ≤100 KB regardless of uptime (NFR-017) | `rules` and `rule_state` are bounded by rule count, not tick count. No verdict history table. |

**Precompilation step.** At `Start()`:

1. `rules.Load()` reads the embedded JSON, strict-decodes into `[]Rule`, rejects unknown fields.
2. `store.Sync(rules)` upserts every bundled rule into the `rules` table with `source='bundled'`; reads back `enabled` flags; logs orphan rows once.
3. `lang.Compile(condition)` walks each rule's condition tree and produces a `func(*EvalCtx) bool` closure.
4. The compiled rule + its initial `rule_state` row form an in-memory `*compiledRule` array, ordered by severity.
5. The hot path (`Eval`) is a single range over that array, calling each closure with a stack-allocated `EvalCtx` populated from the snapshot.

**Failure mode for an over-budget rule.** If a single rule exceeds 5 ms in evaluation, NFR-018 requires a structured log line and rate-limited reporting. The engine continues evaluating the remaining rules within the per-tick budget — no rule's slowness disables the entire panel.

## Deployment Architecture

- **Binary:** the existing `ultron-ap` binary gains the new package. No new artifact.
- **Service unit:** `deploy/ultron-ap.service` is unchanged. No new env vars in v1 — evaluation cadence inherits from the parent `MetricsInterval` config (already 5 s). The bundled rule set is fixed at build time.
- **Migration:** the new `rules` and `rule_state` tables are created by the existing `sqlite.New()` schema-init path, by appending their `CREATE TABLE` statements to the parent `schema` const. No data migration; on first deploy after upgrade the tables appear, the engine seeds `rules` from the embedded JSON, and `rule_state` populates lazily from the first tick.
- **Backup:** captured automatically by the existing `Backup()` (`VACUUM INTO`, parent FR-015) — both new tables are part of the same DB file. Encrypted under `ULTRON_BACKUP_KEY` like the rest. No new file paths.
- **Rollback:** dropping back to a pre-feature binary leaves the two tables behind (harmless — old binary ignores them). No reverse migration needed.
- **No Docker.** The project's deployment model is the systemd-managed Go binary on the Pi (per project-level rejection of Docker for the host service); this feature does not change that.
- **Pi target:** ARM64 binary built via `make build-arm`, copied to `/opt/ultron-ap/ultron-ap`, restart `ultron-ap` systemd unit. Same flow used for `lan-devices` / BG-017.

## Risk Analysis

**Failure blast radius.**

| Component fails | What breaks | What survives | Mitigation |
|---|---|---|---|
| Bundled JSON corrupt at build | Engine starts with zero rules; logs `engine-idle` | All other monitoring (metrics, alerts, lan-devices, dashboards) | Compile-time test asserts `len(rules.Load()) == 10`; CI catches it. |
| One rule's condition is malformed | That rule is dropped at load with a structured log; remaining 9 rules load | Engine + dashboard + alert engine | FR-040 AC-001 covers — graceful per-rule failure. |
| Single rule exceeds 5 ms eval | Per-rule structured log (rate-limited, NFR-018) | Other rules + tick budget | Engine logs and continues; no panic, no per-tick deadline enforcement in v1. |
| Metrics snapshot unavailable for a tick | No new `verdicts` event for that tick; previous set held | Dashboard renders previous verdicts; metrics event continues | FR-039 AC-002 — single `snapshot-missing` log. |
| SQLite write contention on `rule_state` flush | Per-tick state flush blocks ≤5 s on `busy_timeout` | Read path (API) under WAL; eval continues from in-memory state | State flush is debounced (every 30 s, not per tick); a missed flush only loses transition counters across crash. |
| `lan-devices` feature disabled | Rule (10) `lan_device_offline_count` references a missing var | Other 9 rules unaffected | FR-041 AC-002 — missing var → false; rule never fires; `skipped-missing-var` logged once. |
| SSE broker over capacity | New `verdicts` event dropped per existing broker semantics (slow client skip) | Engine evaluation continues; next tick's full set re-publishes | Same backpressure model as `metrics` event. |

**Top risks.**

1. **Rule misconfiguration spamming verdicts** — a poorly-tuned threshold on a noisy metric could flicker a verdict on/off, churning the dashboard. *Mitigation:* (a) FR-046 hysteresis caps a flapping rule at its last stable value within a 10 s window; (b) v1 ships with **bundled-only** rules — every rule is hand-curated and the no-go-zone forbids runtime rule loading; (c) the AC for FR-047 explicitly requires 0 false positives across 1 h of healthy idle Pi, validated before ship.
2. **Telemetry variable schema drift** — a future change in `metrics.Snapshot` field names (e.g. `cpu_pct` → `cpu_total_pct`) would silently zero out rules referencing the old name. *Mitigation:* FR-041 makes a missing var evaluate to false rather than error, so the failure mode is "rule stops firing" not "engine crashes." A `skipped-missing-var` log entry is emitted once per startup per missing var, surfacing the drift in operator logs. A unit test pins the `EvalCtx` variable surface against a golden list to catch silent renames in CI.
3. **SSE bandwidth growth from verbose verdicts** — emitting the full HTML fragment every 5 s, even when nothing changed, costs bandwidth on a residential uplink across long sessions. *Mitigation:* the broker already coalesces the full payload into a single SSE write per tick; the verdict fragment for an empty verdict set is ≤200 bytes. After warm-up, an additional optimisation (out of scope for v1, documented for v2) is to emit `verdicts` only when the verdict set's content hash changes — preserving an idempotent "still alive" heartbeat at lower frequency.

## Technical Risk Flags

- **[RISK] Rule-author error space** — *severity: low.* Bundled-only rules, strict-schema decoding, compile-time AC-047 asserting 0 false positives over 1 h idle. Mitigated by curation. Accept for v1.
- **[RISK] Sustained-window state lost on restart** — *severity: low.* Per-rule ring buffers live in memory only. After a restart, a `sustained(cpu_pct > 90, 30s)` rule needs 30 s of fresh data before firing again. *Mitigation:* Operator-visible behaviour is "verdicts may be quiet for up to a minute after a restart" — acceptable for a dashboard restart. Persisting the ring buffer would dominate backup size and is rejected. Accept for v1.
- **[RISK] Import-boundary regression** — *severity: medium.* A future contributor could accidentally `import "internal/notify"` from inside `internal/insights` and re-couple the two responsibilities. *Mitigation:* a `depguard` / `forbidigo` lint rule enforces the boundary at CI time; the SECURITY DESIGN section above documents the exact forbidden imports. Defer hard enforcement (build break) to a follow-up if a violation ever lands.

No critical or high-severity technical risks identified. The feature reuses every parent contract — metrics tick, SSE channel, SQLite file, backup pipeline, auth middleware, design tokens — and its only structural addition is two SQLite tables and a new SSE event type on the existing channel.
