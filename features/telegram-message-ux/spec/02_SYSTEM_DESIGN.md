# 02_SYSTEM_DESIGN.md — telegram-message-ux

## Executive Summary

This is an internal refactor of the existing `internal/notify` package — no new processes, deployment targets, or runtimes. The current `FormatAlertMessage(alert *database.Alert) string` (telegram.go:143) is replaced by a small set of pure rendering packages and a thin orchestration layer that pulls richer context from the alert engine and existing data sources.

**Stack (unchanged from project baseline, all already deployed):**
- Language: Go 1.22+ (single static binary, ARM64 target)
- Logging: `github.com/rs/zerolog` (already used)
- Telegram client: existing thin `net/http` wrapper in `internal/notify/telegram.go`
- Email client: existing `net/smtp` wrapper in `internal/notify/email.go`
- Templating: `text/template` (stdlib) for plain/MarkdownV2, `html/template` (stdlib) for email HTML
- Persistence: existing SQLite via `internal/database` — no schema changes
- Process model: existing unprivileged systemd unit + privileged helper over Unix socket (FR-011)

**Why this stack:**
- The project's hard constraint is a ~15 MB RAM footprint (NFR-001) and zero-runtime-dependency philosophy. Every choice above is stdlib + libraries already vendored.
- All required data sources (`/proc`, `journalctl`, `docker logs`) are already accessible to the unprivileged web user via existing access patterns; no new privilege escalation needed.
- `text/template` + `html/template` give us a single-pass renderer with built-in escaping, which is critical for FR-025 and the email-HTML-must-be-valid AC.

**What's new internally:** four small subpackages under `internal/notify/`:
- `render/` — pure renderer (no I/O). Input: `RenderInput` struct. Output: `Rendered{TelegramMD, EmailHTML, EmailPlain, EmailSubject}`.
- `cause/` — probable-cause derivation. Time-budgeted (200ms) wrappers around `/proc` reads, `journalctl --tail 3`, `docker logs --tail 3`, and an exit-code map.
- `markdown/` — MarkdownV2 escaper for the Telegram MarkdownV2 special set.
- `storm/` — in-memory fire-cache (rule_id → message_id, first_fired_at, fire_count) with 60-second TTL and editMessageText support.

**Public surface change:** the `Notifier` interface gets one new method `Notify(ctx, *Event) error`. Existing `Send(*database.Alert) error` is preserved as a backwards-compat shim that synthesises a minimal `Event` (so the legacy path still works for ad-hoc callers and a soft transition is possible).

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        internal/alerts (engine)                      │
│  - evaluates rules every snapshot                                    │
│  - tracks first_fired_at + cooldown per rule (in-memory map)         │
│  - on fire/resolve: builds notify.Event{Alert, Rule, Kind, ...}      │
└────────────────────────┬────────────────────────────────────────────┘
                         │ Notify(ctx, evt)
                         ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  internal/notify (Dispatcher)                        │
│  - fan-out to enabled notifiers (telegram, email)                    │
│  - mute-window check (existing FR-005 behavior, preserved)           │
│  - calls render.Render(input) ONCE; reuses output across notifiers   │
└──────┬──────────────────────────────────┬───────────────────────────┘
       │                                  │
       ▼                                  ▼
┌──────────────────────┐         ┌──────────────────────┐
│ TelegramSender       │         │ EmailSender          │
│ - storm.Cache check  │         │ - SMTP send          │
│ - sendMessage OR     │         │   (HTML+plain alt)   │
│   editMessageText    │         └──────────────────────┘
└──────────┬───────────┘
           ▼
   storm.Cache (in-memory, sync.Mutex,
   60s TTL, evict on resolve, sweep every 5m)

       ┌─── render.Render(in) ──────────────────────────────────┐
       │                                                         │
       │  RenderInput { Alert, Rule, Kind, FirstFiredAt, Now,   │
       │                Hostname, PublicURL, Surface, Trend?,   │
       │                Cause? }                                │
       │              ↓                                          │
       │  pure functions, no I/O — stdlib templates only        │
       │              ↓                                          │
       │  Rendered { TelegramMD, EmailHTML, EmailPlain,         │
       │             EmailSubject, TruncatedStep }              │
       │              ↓                                          │
       │  4096-char cap enforced HERE (FR-028)                  │
       └─────────────────────────────────────────────────────────┘

       ┌─── cause.Derive(ctx, surface, alert) — 200ms budget ───┐
       │  resource → cause/proc.go      (top-1 process scan)    │
       │  systemd  → cause/journal.go   (last-error grep)       │
       │  docker   → cause/docker.go    (exit-code map + logs)  │
       │  All wrapped with context.WithTimeout(200ms)           │
       │  Ran by Dispatcher in parallel with surface fetch      │
       │  Failure → Cause = nil; renderer omits the line        │
       └─────────────────────────────────────────────────────────┘
```

**Component responsibilities:**

| Package | Role | I/O | Notes |
|---|---|---|---|
| `internal/alerts` (existing) | Owns first_fired_at + cooldown state; constructs `Event` | reads DB | adds 1 in-memory map field; no schema change |
| `internal/notify/dispatcher.go` (modified) | Single fan-out + render coordination | calls render + cause + sub-notifiers | runs `cause.Derive` and surface-data fetch in parallel goroutines, joins on context |
| `internal/notify/render/` (new) | Pure rendering, deterministic | none | text/template + html/template; tested with table-driven goldens |
| `internal/notify/cause/` (new) | Probable-cause derivation | `/proc`, `journalctl`, `docker logs` (subprocess) | per-source 200ms ctx; sandbox via `exec.Command` argv |
| `internal/notify/markdown/` (new) | MarkdownV2 escape helper | none | inline implementation; ~30 lines |
| `internal/notify/storm/` (new) | Fire-cache + storm-protection logic | none (in-memory) | sync.Mutex; janitor goroutine sweeps every 5m |
| `internal/notify/telegram.go` (modified) | sendMessage / editMessageText / sendFile | net/http to api.telegram.org | calls `storm.Cache` to decide send vs edit |
| `internal/notify/email.go` (modified) | SMTP send (multipart/alternative) | net/smtp | renders both HTML and plain from same `Rendered` |

**Why parallel cause-derivation + surface fetch:** the renderer is pure and fast (NFR-005 ≤50ms). The slow part is the I/O — journalctl and docker logs. Running them in parallel keeps total wall-time bounded by the slowest source, not the sum.

---

## Data Model

**No SQLite schema changes** (explicit no_go_zone item). All new state is in-memory and rebuilt on process restart.

### In-memory state (new)

```go
// internal/notify/storm/cache.go
type FireCache struct {
    mu      sync.Mutex
    entries map[int64]*entry  // key = rule_id (AlertConfig.ID)
    now     func() time.Time  // injectable for tests
}

type entry struct {
    MessageID    int64       // Telegram message_id of the open chat row
    FirstFiredAt time.Time
    LastFiredAt  time.Time
    FireCount    int         // ≥1
}

// TTL: an entry older than 60s on read is treated as absent.
// Resolve clears the entry for that rule_id.
// A janitor goroutine sweeps every 5m and drops entries older than 10m
// (safety net; never load-bearing because reads check TTL).

// internal/alerts/engine.go (modified)
type firingTracker struct {
    mu     sync.Mutex
    fired  map[int64]time.Time  // rule_id → first_fired_at, set on fire,
                                // cleared on resolve
}
```

**Constraints on each field:**

| Field | Type | Constraint | Source |
|---|---|---|---|
| `entry.MessageID` | int64 | Telegram message_id; 0 = invalid; never re-used across sends | Telegram API |
| `entry.FirstFiredAt` | time.Time | UTC; set once on first fire | `time.Now().UTC()` |
| `entry.FireCount` | int | ≥1; incremented on storm-edit | renderer |
| `firingTracker.fired[rule_id]` | time.Time | UTC; cleared on resolve | engine |
| `Event.Surface` | enum string | `"resource" \| "systemd" \| "docker"` | engine derives from `AlertConfig.Metric` |
| `Event.Kind` | enum string | `"fire" \| "resolve"` | engine state transition |

### Read-only inputs (existing, unchanged)

- `database.Alert` — used as-is; `Source` and `Value` fields read by renderer.
- `database.AlertConfig` — looked up by `Alert.ConfigID`. Provides `Metric`, `Operator`, `Threshold`, `Severity`, `Name`. Looked up once per send (not cached) — alert dispatch is rare enough that this is not a hot path.
- `metrics` ring buffer (existing, in `internal/metrics`) — read for trend hint (FR-022). Renderer accepts `Trend *struct{Prior, Current float64; PriorAt time.Time}` as an optional field; the dispatcher fills it from the metrics package when surface = resource.

### Config (env, 12-factor)

| Var | Required | Default | Use |
|---|---|---|---|
| `ULTRON_PUBLIC_URL` | no | derived from `<configured-host>:<port>` | Deep-link footer (FR-023) |
| (existing) `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` | per FR-005 | — | unchanged |
| (existing) `SMTP_*` | per FR-006 | — | unchanged |

---

## API Design

This feature does **not** introduce new HTTP endpoints. It re-uses the existing `POST /settings/telegram/test` (Test Telegram button). The internal Go API surface is described below — that is the contract Phase 4 implements.

### Internal Go API — `internal/notify`

```go
// Public (consumed by internal/alerts)
type Notifier interface {
    Name() string
    Send(alert *database.Alert) error            // EXISTING — preserved verbatim
    Notify(ctx context.Context, evt *Event) error // NEW — engine uses this
}

type Dispatcher struct { /* fields unchanged + storm + render */ }

func (d *Dispatcher) Notify(ctx context.Context, evt *Event) error
func (d *Dispatcher) NotifyTest(ctx context.Context) error  // for "Test Telegram" button

// Event — the new context object the engine assembles
type Event struct {
    Alert        *database.Alert      // existing struct; required
    Rule         *database.AlertConfig // matching config; required for fire, optional for resolve
    Kind         EventKind            // "fire" | "resolve"
    Surface      Surface              // "resource" | "systemd" | "docker"
    FirstFiredAt time.Time            // engine-tracked; zero ⇒ unknown
    ResolvedAt   time.Time            // engine; zero ⇒ N/A
    Trend        *Trend               // optional, resource only
    Hostname     string               // os.Hostname() cached at process start
    PublicURL    string               // ULTRON_PUBLIC_URL or derived
}

type EventKind string  // "fire" | "resolve"
type Surface  string   // "resource" | "systemd" | "docker"

type Trend struct {
    Prior   float64
    Current float64
    PriorAt time.Time   // age of the prior sample (must be 4m30s–5m30s)
}
```

### Internal Go API — `internal/notify/render`

```go
type RenderInput struct {
    Event   *notify.Event   // upcast to avoid a cycle, or move Event to render — TBD in code
    Cause   *Cause          // optional; nil ⇒ omit the situation line
    Surface SurfaceData     // typed: ResourceData | SystemdData | DockerData; nil for some kinds
    Now     time.Time
}

type Rendered struct {
    TelegramMD     string  // ≤ 4096 chars, MarkdownV2-escaped
    EmailHTML      string  // valid HTML5
    EmailPlain     string  // RFC2046 multipart/alternative fallback
    EmailSubject   string  // ≤ 80 chars, no leading emoji
    TruncatedStep  string  // "none" | "journal_300" | "trend" | "cause" | "journal_100" | "minimal"
}

func Render(in RenderInput) (Rendered, error)
```

`Render` is deterministic and side-effect-free. All branching (fire vs resolve, surface variant, truncation steps) is internal. Tested by golden files.

### Internal Go API — `internal/notify/cause`

```go
type Cause struct {
    Source string  // "proc" | "journal" | "exitcode" | "none"
    Line   string  // already escaped for MarkdownV2 — renderer wraps with prefix
}

// Each Derive* fn respects ctx deadline (200ms via WithTimeout in dispatcher)
func DeriveResource(ctx context.Context, metric string) (*Cause, error)  // /proc top-1
func DeriveSystemd(ctx context.Context, unit string) (*Cause, error)     // journal grep
func DeriveDocker(ctx context.Context, container string, exitCode int) (*Cause, error)
```

### Internal Go API — `internal/notify/markdown`

```go
// EscapeV2 escapes the Telegram MarkdownV2 special set.
// Set: _ * [ ] ( ) ~ ` > # + - = | { } . !
func EscapeV2(s string) string
```

### Internal Go API — `internal/notify/storm`

```go
type Cache struct { /* unexported */ }

func New(now func() time.Time) *Cache
func (c *Cache) Decide(ruleID int64, t time.Time) Decision
type Decision struct {
    Send       bool        // true ⇒ sendMessage; false ⇒ editMessageText
    EditTarget int64       // valid only if Send == false
    FireCount  int         // ≥1; placed in subject when ≥2
}
func (c *Cache) RecordSend(ruleID, msgID int64, t time.Time)  // after successful send
func (c *Cache) Clear(ruleID int64)                            // on resolve
func (c *Cache) Sweep(t time.Time) int                         // janitor; returns evicted count
```

### Existing endpoints — unchanged

- `POST /settings/telegram/test` — handler now calls `Dispatcher.NotifyTest(ctx)` which builds a synthetic CPU-fire `Event` and dispatches to the Telegram notifier only. Response shape unchanged: `{"ok": true}` or `{"ok": false, "error": "<truncated 120 chars>"}`.

### Errors

| Error | Status / Return | Mitigation |
|---|---|---|
| `render.Render` panic | recovered → minimal-fallback body | NFR-006 |
| `cause.Derive*` deadline exceeded | `*Cause = nil`, renderer omits line | FR-029 AC-008 |
| Telegram `editMessageText` 400 "message is not modified" | swallowed | FR-024 AC-005 |
| Telegram non-2xx (other) | logged, error returned to dispatcher; storm cache NOT updated | preserves consistency |
| journalctl / docker logs subprocess error | block replaced with `*unavailable: <reason>` | FR-020 / FR-021 |

---

## Security Design

### Trust boundaries

- **Trusted:** values produced by our own code (template literals, severity tokens, friendly metric labels).
- **Untrusted (must escape / validate):** hostname, unit name, container name, image reference, journal lines, docker log lines, top-process names from `/proc/<pid>/comm`, alert message text.

### Markdown injection (FR-025, NFR-008)
All untrusted strings are passed through `markdown.EscapeV2` before substitution into the Telegram template. The escape set is the full MarkdownV2 special set: `_ * [ ] ( ) ~ ` ` ` > # + - = | { } . !`. Test corpus includes hostnames with dots, services with underscores, journal lines with backticks and asterisks, and an adversarial input fuzz test.

### Subprocess argument injection (NFR-008)
- All subprocess invocations use `exec.Command` with explicit argv — never `sh -c`.
- Unit name (systemd) is validated against `^[A-Za-z0-9@:_\-.\\]+\.(service|socket|timer|target)$` before being passed as an argv element.
- Container name is validated against `^[A-Za-z0-9][A-Za-z0-9_.\-]{0,127}$` (Docker's documented charset).
- `/proc/<pid>` reads use `os.Open` with the integer pid only — pid is parsed via `strconv.Atoi`, no string concat.
- An invalid name short-circuits the cause derivation; the message renders without the cause line plus a warn log.

### HTML injection in email body (FR-027)
- HTML body uses `html/template`. All untrusted fields are interpolated via `{{.Field}}` — `html/template` auto-escapes per context. A static-analysis check (`go vet -shadow` + a custom CI grep for `template.HTML(`) ensures no manual `template.HTML(...)` casts on untrusted strings.

### URL safety (FR-023)
- `ULTRON_PUBLIC_URL` is read once at process start and validated against `url.Parse` requiring scheme `http` or `https` and a non-empty host. Invalid → fallback to `http://<host>:<port>` with the same validation. The footer link is generated using `url.URL{}.String()`, never string concat.

### Data exfiltration / log content
- Journal and docker log tails are sent to Telegram and SMTP destinations the operator has already trusted. There is no broadening of trust boundaries; an attacker who already has root on the Pi can already read these logs.

### Auth
- Existing project auth (FR-007 cookie session, FR-012 CSRF) protects the `Test Telegram` endpoint. No change.

### Headers
- N/A: this feature does not introduce new HTTP responses with body content. Existing headers on `/settings/telegram/test` are preserved.

---

## Performance & Scalability

### Budgets (per NFR-005 + FR-029)

| Stage | Budget | Notes |
|---|---|---|
| Pure render (`render.Render`) | ≤50 ms p95 | excludes I/O; benchmarked on linux/arm64 with stubbed inputs |
| Cause derivation (any one source) | ≤200 ms hard timeout | enforced by `context.WithTimeout` |
| Journal/docker log fetch | ≤500 ms hard timeout | per FR-020 / FR-021 |
| Total wall-time, fire path | ≤700 ms p95 | render + slowest parallel I/O |
| Telegram round-trip | unbounded by us | depends on network, not our concern |

### Concurrency
- Cause derivation and surface-data fetch run as **two parallel goroutines** joined by `errgroup` with the dispatcher's outer context. Either timing out cancels itself only — the other completes.
- The Telegram notifier serialises sends per-bot (single shared `*http.Client`), but the renderer's parallelism is upstream of that and not impacted.

### Caching
- `os.Hostname()` cached at process start (single read); never re-read.
- `ULTRON_PUBLIC_URL` parsed once at process start.
- `storm.Cache` is the only feature-introduced cache (in-memory, ~50 bytes per entry, expected steady-state size <100 entries).
- Friendly-label table is a `var = map[string]string{...}` package-level constant.

### Memory
- Storm cache worst-case ~10 KB at 200 active rules — negligible vs. the 15 MB process budget (NFR-001).
- Renderer allocates one `bytes.Buffer` per render and discards it; no pooling needed at our scale (≤1 alert/sec sustained).

### Throughput / scalability
- Single-tenant Pi product. Expected sustained alert rate: <1 / minute. Burst: up to 10 / second during a storm — handled by `storm.Cache` (those become edits, not sends).
- Telegram bot API rate limit: 30 messages/sec to different chats, 1 message/sec to the same chat. Storm protection keeps us well under the per-chat limit even in worst case.

---

## Deployment Architecture

**Unchanged from project baseline.** This feature ships as additional Go source files in the existing single-binary build.

| Aspect | Value |
|---|---|
| Target platform | Raspberry Pi 5 (ARM64), Raspberry Pi OS Bookworm (`linux/arm64`) |
| Build | `make build-arm` (existing) → single static binary |
| Service manager | `systemd` (existing unit, unprivileged user, `NoNewPrivileges=true`) |
| Privileged helper | existing root-owned helper over Unix socket (FR-011) — **not modified** |
| CI/CD | existing GitHub Actions pipeline, `make verify` on every push to main |
| Environments | single env (Pi). No staging needed for this feature; tests cover the contract end-to-end. |
| Observability | structured logs to stdout via zerolog → `journalctl -u ultron-web` (existing) |
| Health | existing `GET /health` unchanged (NFR-010) |
| Backups | existing encrypted backup flow unchanged (FR-015) — no new persistent state |

**Rollout:** standard `aitri normalize` flow per CLAUDE.md. No migration step. The first deploy carries an in-memory empty `storm.Cache`; the cache warms naturally on first alert.

---

## Risk Analysis

### Top 5 risks

**Risk 1 — Subprocess slowness blocks the alert path.**
*What can happen:* `journalctl` or `docker logs` hangs on a busy system; without bounds the alert send stalls.
*Mitigation:* per-source `context.WithTimeout(500ms for surface fetch, 200ms for cause)`; on timeout the block is replaced with `*unavailable: timeout` and a warn log entry.
*Residual risk:* low.

**Risk 2 — Storm cache memory leak.**
*What can happen:* if a rule fires once, never resolves, never re-fires (e.g. config deleted), the entry stays.
*Mitigation:* janitor goroutine sweeps entries older than 10 min every 5 min. Reads also check the 60s TTL — stale entries are functionally invisible regardless of physical eviction.
*Residual risk:* very low.

**Risk 3 — MarkdownV2 escape gap → message rejected by Telegram, alert lost.**
*What can happen:* a special character not in our escape set sneaks through; Telegram returns 400 "can't parse entities"; storm cache state diverges from reality.
*Mitigation:* (a) NFR-006 minimal-fallback path always produces a parseable body; (b) on 400 from Telegram the dispatcher retries once with the minimal-fallback; (c) fuzz tests in CI cover the full ASCII printable + common UTF-8 punctuation.
*Residual risk:* low — the minimal fallback is the safety net.

**Risk 4 — `editMessageText` updates a message the user has already deleted.**
*What can happen:* user deletes the chat row; next storm-edit fails with 400 "message to edit not found".
*Mitigation:* on that specific 400, clear the cache entry and fall back to `sendMessage`. Logged at info level (not error — user action is expected).
*Residual risk:* very low.

**Risk 5 — `os.Hostname()` returns localhost on misconfigured Pi.**
*What can happen:* subject line says "on localhost" — operator can't tell which Pi is alerting (matters if they manage more than one).
*Mitigation:* if `os.Hostname()` returns `localhost` or empty, log a warn at process start and prefer `ULTRON_HOSTNAME` env var if set. The hostname-resolution path is documented in DEPLOYMENT.md.
*Residual risk:* operator-config issue, not a code bug.

### Failure blast radius

**Component: `render.Render` (renderer)**
- Blast radius: stops producing nicely-formatted messages.
- User impact: receives the minimal-fallback body (severity emoji + friendly label + value + threshold + deep-link).
- Recovery: automatic via NFR-006 panic-recover. No manual intervention.

**Component: `cause.Derive*` (probable-cause subprocesses)**
- Blast radius: probable-cause line missing.
- User impact: alert still arrives with all other context; just no "top: ffmpeg" hint.
- Recovery: automatic on next fire when subprocess returns to health.

**Component: `storm.Cache`**
- Blast radius: storm protection disabled.
- User impact: storm of fires becomes N separate chat rows instead of one (legacy behavior).
- Recovery: process restart rebuilds an empty cache; no data loss because cache is ephemeral by design.

**Component: Telegram API (external)**
- Blast radius: outgoing alerts fail.
- User impact: no Telegram message; email notification (if configured) still arrives independently.
- Recovery: existing alert engine retry policy (unchanged); operator notices via dashboard `/alerts` page.

### ADRs

**ADR-01: Renderer separation — pure package vs. in-place modification of existing telegram.go**
*Context:* Today's `FormatAlertMessage` mixes formatting with the Telegram-specific MarkdownV2 idioms. We need to render two surfaces (Telegram + Email) from one model and keep rendering testable in isolation.
*Option A:* Extract a new `internal/notify/render` package with no I/O; both Telegram and Email call it.
*Option B:* Keep formatting inside each notifier; share a small helper for cause+trend.
*Decision:* **Option A.**
*Reason:* Two notifiers must produce identical logical content (FR-027). A shared pure renderer is the only way to make that a contract instead of a coincidence. Test cost is also lower: one set of golden files vs two.
*Consequences:* +1 package; renderer becomes the source of truth for content order; notifiers shrink to "send what render produced".

**ADR-02: Storm cache storage — in-memory map vs. SQLite-backed**
*Context:* FR-024 needs a 60-second window to group same-rule fires. State is tiny (one row per active rule).
*Option A:* In-memory `sync.Mutex`-protected map (rebuilds on restart).
*Option B:* New SQLite table `notification_storms` (rule_id, message_id, first_fired_at).
*Decision:* **Option A.**
*Reason:* (1) `no_go_zone` forbids SQLite schema changes. (2) Storm protection is a 60-second concern; a process restart that loses the cache simply means the next fire creates a new chat row, which is correct fallback behavior (worst case = legacy behavior). (3) ~50 bytes per entry × ≤200 active rules ≈ 10 KB — irrelevant against the 15 MB budget.
*Consequences:* No persistence ↔ acceptable; simplest possible implementation; one less DB migration to write tests for.

**ADR-03: Cause-derivation execution model — sequential vs. parallel-with-timeouts**
*Context:* FR-029 declares a 200 ms budget per source; FR-020/021 declare 500 ms for journal/docker fetch. If we run sequentially, the worst case is the sum (700 ms) and we miss the NFR-005 budget.
*Option A:* Sequential (cause first, then surface fetch).
*Option B:* Parallel via `errgroup` with independent contexts.
*Decision:* **Option B.**
*Reason:* The two sources are independent and both bounded; parallel execution gives total latency = max(200, 500) = 500 ms instead of 700 ms.
*Consequences:* +1 errgroup dependency (already in stdlib via `golang.org/x/sync/errgroup` which is already vendored). Minor: must propagate parent context for cancellation on dispatcher abort.

**ADR-04: Markdown escaping — inline implementation vs. third-party library**
*Context:* FR-025 requires escaping for Telegram MarkdownV2's 18-char special set.
*Option A:* Inline `~30-line` implementation in `internal/notify/markdown`.
*Option B:* Adopt a third-party MarkdownV2 escaper (e.g. `github.com/go-telegram-bot-api/telegram-bot-api/v5` exposes one).
*Decision:* **Option A.**
*Reason:* The escape rule is trivial (single regex pass with backslash insert); a third-party dep adds ≥3 transitive dependencies and a new versioning concern. The project's NFR-002 (zero-runtime-dep philosophy) discourages new deps where the in-house implementation is <50 lines. Tested with a fuzz test.
*Consequences:* Maintain the escape table ourselves. Telegram changes the MarkdownV2 set rarely; a comment links to the official docs and the test corpus is the source of truth.

**ADR-05: `Notifier` interface evolution — add `Notify` vs. modify `Send`**
*Context:* The engine needs to pass richer context (Rule, Kind, FirstFiredAt) to the notifier. The `requirements.constraints` field forbids changing the existing `Send(*database.Alert) error` signature.
*Option A:* Add a new method `Notify(ctx, *Event) error`; keep `Send` as a backwards-compat shim.
*Option B:* Change `Send` to take `*Event` and update all callers.
*Decision:* **Option A.**
*Reason:* The constraint is explicit. Option A also gives a graceful migration: the legacy `Send` can synthesise a minimal `Event` for callers that haven't moved (today, only ad-hoc `notify.Send(alert)` calls in tests). Once those move, `Send` can be deleted in a future release.
*Consequences:* Two methods on the interface during the transition; one deprecation cycle. Net cost: 8 lines of shim code. Net benefit: zero risk of breaking the alert engine's existing call site.

---

## Technical Risk Flags

[RISK] `database.Alert` struct lacks rule context that the new renderer needs
Conflict: FR-016 / FR-017 / FR-024 require operator, threshold, rule name and rule_id at render time, but `database.Alert` only carries `ConfigID *int64`, `Severity`, `Message`, `Source`, `Value`. The schema cannot be modified (no_go_zone item #6).
Mitigation: The dispatcher fetches the matching `AlertConfig` by `ConfigID` once per send (single indexed lookup, p99 well under 5 ms on the local SQLite). The fetched record populates the `Event.Rule` field. No schema change needed.
Severity: medium

[RISK] Engine does not currently track `first_fired_at` per rule
Conflict: FR-018 (resolve duration) and FR-019 (elapsed-since-breach) require `first_fired_at`. The alert engine tracks cooldown windows in-memory but not first-fired timestamps.
Mitigation: Extend the existing in-memory cooldown map to also store `first_fired_at`. Cleared on resolve. No persistence required because resolves and fires within a single process lifetime are what the user cares about; a process restart simply means the next fire is treated as a new incident, which is acceptable fallback.
Severity: medium

[RISK] Telegram MarkdownV2 special-character set may grow over time
Conflict: ADR-04 chose an inline escaper. If Telegram silently adds a new special char (historic precedent: `!` was added in 2020), our escape misses it and messages are rejected.
Mitigation: The minimal-fallback path (NFR-006) always produces a parseable body, so a missed escape causes one ugly message but never a lost alert. CI runs a fuzz test against the official Telegram API in a smoke test job (test bot + test chat in a dedicated workspace) so a new special char trips a test before it reaches production.
Severity: low

[RISK] `journalctl` access from the unprivileged web user
Conflict: FR-020 requires reading the journal from the web process (which runs unprivileged per FR-011).
Mitigation: Verified during architecture: the existing systemd service monitor (FR-003) already reads `journalctl --user-unit` patterns from this same user successfully. Same code path. If a unit name is requested the user cannot read, `journalctl` returns empty + non-zero — handled by the existing "journal unavailable" branch. No new privilege needed.
Severity: low

[RISK] `/proc` top-process scan cost on a heavily-loaded Pi
Conflict: NFR-005 demands ≤50 ms p95 for render; FR-029 demands ≤200 ms for cause derivation. A naive scan of /proc on a Pi with 300 PIDs takes ~80 ms.
Mitigation: (a) Reuse the existing metrics-collector's process snapshot (it already runs every 5 s and cached snapshot is read in O(1)). (b) If the cached snapshot is stale (>10 s old), do a one-shot scan with the 200 ms budget. (c) On budget exhaustion, omit the cause line.
Severity: low

[RISK] `editMessageText` rate limit on rapid storm bursts
Conflict: FR-024 expects edits to update one chat row when N fires arrive in 60 s. Telegram per-chat edit rate is 1/sec; on a tight oscillation we could exceed this.
Mitigation: The storm cache coalesces — even if 10 fires arrive in 1 s, we only call `editMessageText` once per "natural" fire boundary because the cache key is rule_id + dispatcher serialises sends per-notifier. We add a small debounce (250 ms) before edits to coalesce sub-second bursts.
Severity: low

---

## Traceability Checklist

- [x] Every FR-* is addressed:
  - FR-016 → render.Render metric-line block
  - FR-017 → render + friendly-label table; subject ≤80 chars
  - FR-018 → render fire-vs-resolve template branches
  - FR-019 → render elapsed/timestamp branch driven by `Event.FirstFiredAt`
  - FR-020 → cause/journal.go + render systemd template
  - FR-021 → cause/docker.go + render docker template
  - FR-022 → metrics package read for Trend; render trend block
  - FR-023 → render footer; URL parsed once at startup
  - FR-024 → storm.Cache + Telegram editMessageText path
  - FR-025 → markdown.EscapeV2 helper applied to all untrusted fields
  - FR-026 → Dispatcher.NotifyTest synthesises CPU-fire Event
  - FR-027 → render produces EmailHTML/EmailPlain from same model
  - FR-028 → render truncation chain enforced inside `render.Render`
  - FR-029 → cause package + render situation-line block
- [x] Every NFR-* has a corresponding design decision:
  - NFR-005 → parallel goroutines + per-source ctx timeout
  - NFR-006 → render panic-recover + minimal-fallback path
  - NFR-007 → zerolog structured log line wired in dispatcher
  - NFR-008 → exec.Command argv-only + name validation regexes
  - NFR-009 → tests live in render_test.go / cause_test.go / storm_test.go; existing CI runs them
  - NFR-010 → no new endpoint; existing /health unchanged
- [x] Every ADR has ≥2 options (5 ADRs, 2 options each)
- [x] no_go_zone items NOT introduced:
  - alert engine schema: untouched
  - SQLite schema: untouched (storm cache is in-memory)
  - settings page UI: untouched (button reused)
  - new notifier channels: not added
  - inline buttons / two-way bot: not added
- [x] Failure blast radius documented for ≥4 critical components (renderer, cause, storm, Telegram API)
- [x] Technical Risk Flags section complete: 6 flags declared with severity + mitigation
