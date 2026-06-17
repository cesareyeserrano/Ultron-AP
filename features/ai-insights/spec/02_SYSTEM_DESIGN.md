# Technical Design Document (TRD / SDD) — AI Insights (feature: ai-insights)

## Executive Summary

This feature adds an AI explanation layer to Ultron-AP without changing its runtime shape. It is implemented entirely inside the existing `cmd/ultron-ap` web process as a new internal package plus handler/settings/notify wiring — **no new process, no new runtime dependency, no embedded model** (the LLM is an external HTTP endpoint).

Tech choices (all reuse the existing stack — Go 1.25.11, modernc/sqlite, htmx+SSE, Tailwind):
- **New package `internal/ai`** — a provider-agnostic client speaking **OpenAI-compatible Chat Completions over HTTPS** (the de-facto interface exposed by Ollama Cloud, OpenAI, GLM, vLLM, LM Studio…). The endpoint URL, model, and key are runtime config, so any compatible provider works without code changes (FR-020). Reason: one HTTP contract covers every candidate provider; a vendor SDK would lock us in and add transitive deps (against NFR-003).
- **Prompt assembly + redaction** in `internal/ai` — gathers an already-collected telemetry snapshot (metrics ring buffer, active insights, service/docker/systemd state, capped recent logs), redacts secrets (FR-023), and composes a bounded prompt.
- **Config + secret storage** — reuse `internal/database/secrets.go` (AES-GCM, `ULTRON_SECRET_KEY`) for the API key; AI config persisted in a dedicated `ai_settings` single-row table. Reason: same at-rest guarantee as Telegram/SMTP secrets (FR-017), no new secret store (constraint).
- **HTTP surface** — three authenticated handlers under `internal/server` (`/api/ai/explain`, `/api/ai/test`, `/api/settings/ai`), CSRF-protected for mutations (reuse FR-007/FR-012).
- **Telegram push** — a best-effort async hook in the alert dispatch path (`internal/notify`) that sends an additive AI follow-up after the rule-based alert (FR-026), plus a small category→emoji map applied to the alert header (UX spec).
- **Frontend** — htmx partials rendered by existing Go templates + the existing SSE/toast plumbing; no SPA, no new JS framework (honors no_go_zone).

## System Architecture

```
                      ┌────────────────────── cmd/ultron-ap (web, unprivileged) ──────────────────────┐
  Browser (htmx)      │                                                                                │
  ───────────────►  internal/server (handlers, CSRF, auth session)                                     │
   GET  /settings     │   ├─ handlers_ai.go                                                             │
   POST /api/ai/explain│  │     POST /api/ai/explain  ─┐                                                │
   POST /api/ai/test   │  │     POST /api/ai/test     ─┤                                                │
   POST /api/settings/ai│ │     GET/POST /api/settings/ai (key masked on GET)                           │
                      │   │                            │                                                │
                      │   ▼                            ▼                                                │
                      │  internal/ai ──────────────────────────────────────────────────────┐          │
                      │   ├─ Service.Explain(ctx, scope) (timeout ≤10s, fail-closed)         │          │
                      │   ├─ snapshot.Collect()  ◄── metrics / insights / docker / systemd / logfilter  │
                      │   ├─ redact.Scrub()      ◄── DB secrets table (values to strip)      │          │
                      │   └─ Client (OpenAI-compatible Chat Completions over HTTPS) ─────────┼──► EXTERNAL
                      │                                                                      │   LLM endpoint
                      │  internal/database                                                   │  (Ollama Cloud/
                      │   ├─ ai_settings (enabled,endpoint,model,api_key_enc,push,timeout)   │   OpenAI/GLM…)
                      │   └─ secrets.go (AES-GCM, ULTRON_SECRET_KEY)                          │          │
                      │                                                                      │          │
   Telegram  ◄────────┤  internal/notify (alert dispatch)                                    │          │
   (alert + AI note)  │   ├─ rule-based alert  ── sent first, own path (unchanged) ──────────┘          │
                      │   └─ aiFollowUp(): go func{ ai.Explain → telegram.Send } (best-effort, async)    │
                      └────────────────────────────────────────────────────────────────────────────────┘
   (ultron-helper / privileged socket is NOT touched — AI is read-only, no actions)
```

Components & responsibility (separation of concerns):
- `internal/ai/service.go` — orchestrates: load config → collect snapshot → redact → call client → parse to `Explanation{Cause, Remediation, CitedSignals[], Unverified}`.
- `internal/ai/client.go` — HTTP transport only (OpenAI-compatible request/response, timeout, TLS).
- `internal/ai/snapshot.go` — read-only collection from existing in-memory/DB sources; **no** new collectors.
- `internal/ai/redact.go` — strips secret values + secret-shaped tokens before they ever enter a prompt or log.
- `internal/database/ai_settings.go` — CRUD for the config row; key encrypted via `secrets.go`.
- `internal/server/handlers_ai.go` — HTTP contract, auth/CSRF, key masking.
- `internal/notify/ai_followup.go` — async best-effort Telegram follow-up + category-emoji map.

## Data Model

**New table `ai_settings`** (single row — single-admin; `id` pinned to 1). SQLite (modernc), strong consistency (ACID) like the rest of the schema.

| Column | Type | Constraints | Notes |
|---|---|---|---|
| `id` | INTEGER | PRIMARY KEY CHECK(id=1) | enforces single row |
| `enabled` | INTEGER | NOT NULL DEFAULT 0 | 0/1 — master AI toggle (FR-018/FR-019) |
| `endpoint_url` | TEXT | NOT NULL DEFAULT '' | provider base URL; validated https (NFR-005) |
| `model` | TEXT | NOT NULL DEFAULT '' | model name (FR-020) |
| `api_key_enc` | BLOB | NULL | AES-GCM ciphertext via secrets.go (FR-017); never plaintext |
| `telegram_push` | INTEGER | NOT NULL DEFAULT 0 | 0/1 — additive Telegram follow-up (FR-026) |
| `timeout_ms` | INTEGER | NOT NULL DEFAULT 10000 | request bound (NFR-006), 1000–60000 |
| `allow_insecure` | INTEGER | NOT NULL DEFAULT 0 | explicit non-https override (UX trade-off) |
| `updated_at` | TEXT | NOT NULL | RFC3339 |

Migration: additive `CREATE TABLE IF NOT EXISTS` in the existing migration path; seeds one row with defaults (disabled). No change to existing tables — existing data untouched (regression boundary).

**Transient (not persisted):** `TelemetrySnapshot` and `Explanation` are request-scoped structs, never written to disk.

## API Design

All endpoints are under the existing authenticated session (FR-007); mutations require the existing CSRF token (FR-012). JSON in/out. Errors use the existing `{ "error": "<message>" }` shape with conventional status codes.

### `POST /api/ai/explain`
- Auth: session required. CSRF: required.
- Request: `{ "scope": "system" | "insight", "insight_id": <int, required when scope=insight> }`
- Response 200: `{ "cause": "<text>", "remediation": "<text>", "cited_signals": ["cpu_temp","rule_id=8"], "unverified": false, "latency_ms": 1840 }`
- Errors: `409 {"error":"AI not configured"}` (FR-019) · `422 {"error":"insufficient telemetry to explain"}` (FR-016 empty) · `502 {"error":"provider error: 401 unauthorized"}` · `504 {"error":"provider timed out"}` (FR-021). Never 500/panic on provider faults.

### `POST /api/ai/test`
- Auth + CSRF. Request: `{ "endpoint_url","model","api_key"? }` (uses stored key if omitted).
- Response 200: `{ "ok": true, "resolved_model": "qwen2.5:14b", "latency_ms": 720 }`
- Errors: `502 {"ok":false,"error":"<reason>"}` — persists nothing (FR-025).

### `GET /api/settings/ai`
- Auth. Response 200: `{ "enabled":true, "endpoint_url":"https://…", "model":"…", "telegram_push":false, "timeout_ms":10000, "api_key_set":true, "api_key":"" }` — **raw key never returned** (FR-017); only `api_key_set` boolean + empty/masked string.

### `POST /api/settings/ai`
- Auth + CSRF. Request: full config; `api_key` empty string = keep existing, sentinel `"__clear__"` = delete.
- Validation (server-side, before persist — H5): `enabled=1` requires non-empty `endpoint_url` and a key (stored or provided); `endpoint_url` must parse and be `https` unless `allow_insecure=1`; `timeout_ms ∈ [1000,60000]`.
- Response 200: same shape as GET. Errors: `400 {"error":"endpoint_url required when AI enabled"}`.

### Internal package API (`internal/ai`)
```go
type Service interface {
    Explain(ctx context.Context, scope Scope) (*Explanation, error) // honors ctx deadline (timeout)
    Test(ctx context.Context, cfg Config) (resolvedModel string, err error)
    Enabled() bool
}
type Scope struct { Kind ScopeKind; InsightID int } // ScopeSystem | ScopeInsight
type Explanation struct { Cause, Remediation string; CitedSignals []string; Unverified bool; LatencyMS int64 }
// notify
func AIFollowUp(ctx context.Context, svc ai.Service, tg notify.TelegramSender, ev alerts.Event) // async, best-effort
```

## Security Design

- **Auth boundary:** every AI endpoint sits behind the existing session middleware (FR-007); no new anonymous surface. Mutations carry the existing same-origin CSRF check (FR-012).
- **Secret at rest:** `api_key` stored only as AES-GCM ciphertext via `secrets.go` (`ULTRON_SECRET_KEY`); decrypted in-memory only at call time (FR-017). `GET` returns `api_key_set` boolean, never the value. The key is excluded from all logs (NFR-008 asserts no secret substring).
- **Prompt redaction (FR-023):** before composing the prompt, `redact.Scrub()` removes (a) every secret value currently in the DB secrets table (Telegram token, SMTP pass, AI key, session tokens) by exact match, and (b) secret-shaped tokens (bearer/JWT/long hex/base64 ≥ N chars) by pattern. Unit tests assert no secret reaches the payload or logs.
- **Transport:** outbound call uses TLS; `endpoint_url` rejected at save if not `https` unless `allow_insecure=1` is explicitly set (single-admin trade-off, surfaced in UX).
- **SSRF posture:** the endpoint URL is operator-supplied. Single-admin trust model bounds this, but the client (a) does not follow redirects to a different host, (b) logs the outbound host, (c) carries the request timeout. Documented as an accepted risk (single trusted operator).
- **Read-only guarantee:** `internal/ai` has no import of `internal/privileged`, `systemd`, or `docker` controls — it cannot take an action (FR-016, no_go_zone B enforced structurally, asserted by a no-import test).
- **XSS:** AI output is rendered through the existing auto-escaping Go templates / `textContent` toast path (the UX spec mandates `textContent`, not `innerHTML`); cited-signal chips are escaped.

## Performance & Scalability

- **Latency bound (NFR-006):** `Explain` runs under a `context.WithTimeout(timeout_ms, default 10s)`. On deadline it aborts and returns 504; the HTTP handler returns promptly.
- **Non-blocking by construction:** snapshot collection reads existing in-memory ring buffers / cached state and a capped log slice — no heavy queries. The Telegram follow-up runs in its own goroutine with its own timeout so it never delays alert dispatch or metrics collection (NFR-007, FR-026).
- **Prompt size bounds:** snapshot caps — last N metric points per series, top-K active insights, ≤ M recent log lines (configurable constants) — to keep the prompt small, the call fast, and cost low.
- **Footprint (NFR-001):** no embedded model, no persistent goroutine pool; one transient HTTP request per explanation. RAM stays within the ≤15 MB envelope. Single-admin → effectively 1 concurrent request; no rate limiter needed, but `Explain` is safe under concurrency (stateless service).
- **Caching:** none required (on-demand, low frequency). Config is read from a single cached row, invalidated on save.

## Deployment Architecture

- **Deployment model: single native Go binary** (`ultron-ap`), cross-compiled `GOOS=linux GOARCH=arm64`, deployed to `/opt/ultron-ap` and run under systemd on the Raspberry Pi — **unchanged** from the current model. **No containers, no new services, no new runtime dependency** (the AI provider is an external network endpoint, not bundled). This matches NFR-002/003 and the project's established systemd path (the prior Phase-5 Docker rejection stands).
- **Config:** AI settings live in the DB (panel-managed), not env vars — the only required env remains `ULTRON_SECRET_KEY` for at-rest encryption (already provisioned).
- **Environments:** build on Mac → arm64 → scp → systemd restart (existing DEPLOYMENT.md flow). Rollback = prior binary (`.prev`).
- **CI/CD (NFR-009):** existing `security-gate.yml` (`go test ./...` + govulncheck) and `ci.yml` (vet, `-race`, arm64 build, lint) exercise `internal/ai` automatically on every push/PR.

## Risk Analysis

Top risks + mitigation (ADRs below):
1. **External LLM latency/availability** breaks the ≤10s UX → timeout + fail-closed error state + best-effort async push; the panel and alerts never depend on the provider.
2. **Secret leakage to a third-party model** (sending logs to an external LLM) → redaction layer (FR-023) with tests; https-only by default; operator-chosen endpoint (can be self-hosted Ollama).
3. **Regression of the existing alert path** (FR-004/FR-005) by adding the push → push is a separate async goroutine; rule-based alert is sent first on its own path; covered by a regression test.
4. **SSRF via operator-set endpoint** → single-admin trust, no cross-host redirects, host logged, timeout. Accepted.
5. **Hallucinated remediation** the operator might act on → read-only (never auto-applies), `unverified` labeling when ungrounded (FR-024), cited signals shown.

### ADR-01: AI provider integration approach
- Context: must reach an LLM and stay provider-swappable (FR-020), zero new runtime deps (NFR-003).
- Option A: Generic **OpenAI-compatible HTTP client** (hand-rolled, net/http). — broad compatibility, tiny dep surface; we own the contract.
- Option B: Vendor SDK (OpenAI-go / Ollama client). — faster start, but lock-in + transitive deps + per-provider divergence.
- Decision: **A** — one HTTPS contract covers Ollama Cloud/OpenAI/GLM/vLLM; no new module.
- Consequences: we maintain a small request/response mapping; trivial to add providers via config only.

### ADR-02: AI config & secret storage
- Context: persist config + an API key with the same at-rest guarantee as existing secrets (FR-017), no new secret store (constraint).
- Option A: **Dedicated `ai_settings` table + `secrets.go` for the key.** — explicit schema, clear validation.
- Option B: Reuse the generic notification-config JSON blob store. — fewer tables, but weaker typing and mixes concerns.
- Decision: **A** — typed single-row table; key encrypted via the existing AES-GCM mechanism.
- Consequences: one additive migration; existing tables untouched.

### ADR-03: Explanation delivery (sync vs stream)
- Context: deliver the explanation to the panel.
- Option A: **Single bounded request/response** (no streaming). — simple, matches no_go_zone (streaming excluded).
- Option B: SSE token streaming. — nicer feel, but more surface and explicitly out of scope this increment.
- Decision: **A**.
- Consequences: a loading state covers the wait (≤10s); upgrade path to streaming left open.

### ADR-04: Telegram push integration point
- Context: send an additive AI note when an alert fires without regressing alerting (FR-026, NFR-007).
- Option A: **Async hook in the existing alert dispatch** (`internal/notify`), goroutine + own timeout.
- Option B: Separate watcher polling the alert/insight store. — extra moving part, lag, duplicate-risk.
- Decision: **A** — fire-and-forget after the rule-based send; failure only logged.
- Consequences: zero added latency to the primary alert; needs a regression test proving the alert still sends when AI fails.

### ADR-05: Frontend approach
- Context: build S1/S2/S3 UI consistent with the app.
- Option A: **htmx partials + existing Go templates + existing toast/SSE JS.** — consistency, no new deps (no_go SPA).
- Option B: New JS component/framework. — out of scope, breaks NFR-003.
- Decision: **A**.
- Consequences: reuse existing settings-card and modal/slide-over patterns and design tokens.

### ADR-06: State management & deployment target
- Context: where feature state lives and how it ships.
- Option A: **Server-side state in SQLite (single row) + single Go binary on systemd.** — matches existing architecture.
- Option B: Client-side/local state and/or containerized deploy. — inconsistent, against NFR-002/003 and prior Phase-5 rejection.
- Decision: **A**.
- Consequences: no infra change; one migration; existing deploy pipeline.

### Failure Blast Radius

Component: **External LLM provider**
- Blast radius: AI explanations and the Telegram AI follow-up only.
- User impact: explanation panel shows an error state with a one-line reason + Retry (FR-022); rule-based alerts and the whole monitoring panel keep working; `/health` stays 200.
- Recovery: retry on demand; fail-closed — no degradation of non-AI features.

Component: **`ai_settings` / secrets (config + key)**
- Blast radius: ability to call AI (config read fails or key undecryptable).
- User impact: AI treated as not-configured (FR-019) — `Explain` returns 409; the panel behaves exactly as without AI; no errors elsewhere.
- Recovery: re-enter config/key in Settings; `ULTRON_SECRET_KEY` is already provisioned and recoverable (operator Keychain).

Component: **Alert dispatch (with AI push enabled)**
- Blast radius: the additive AI follow-up message only.
- User impact: if AI generation fails, the operator still receives the normal rule-based alert; no duplicate, no drop (FR-026 AC-026-1f).
- Recovery: automatic — the async push errors are logged and ignored; next alert retries independently.

## Technical Risk Flags

[RISK] External LLM latency may exceed the ≤10s UX target
Conflict: NFR-006 / FR-016 require an explanation within ~10s, but an external model (esp. a large one or a cold Ollama Cloud route) can exceed that.
Mitigation: hard `context` timeout (default 10s, configurable) → 504 + error state; recommend a small 8–14B model (technology_preferences); fail-closed so nothing else waits.
Severity: medium

[RISK] Sending telemetry/logs to a third-party model can leak secrets
Conflict: FR-023 / NFR-005 forbid secrets leaving the device, but raw logs/metrics may embed tokens or passwords.
Mitigation: `redact.Scrub()` removes known secret values (exact match from the DB secrets table) and secret-shaped patterns before prompt assembly; unit tests assert no secret in payload/logs; operator may point at a self-hosted endpoint.
Severity: high

[RISK] AI Telegram push could delay or duplicate the rule-based alert
Conflict: FR-005/FR-004 must not regress, but adding work in the alert path risks blocking or double-sending.
Mitigation: rule-based alert sent first on its own path; AI follow-up is a separate goroutine with its own timeout, best-effort, errors only logged; regression test asserts alert delivery when AI fails.
Severity: medium

[RISK] SSRF via operator-configured endpoint URL
Conflict: the server issues outbound requests to an operator-supplied URL; a malicious/mistaken URL could hit internal services.
Mitigation: single-admin trust model; no cross-host redirect following; outbound host logged; request timeout; https-by-default. Accepted residual risk for the single-operator threat model.
Severity: low

[RISK] Hallucinated remediation the operator might act on
Conflict: FR-016 returns remediation text that could be wrong.
Mitigation: read-only (the system never auto-applies); `unverified` label when the model gives no citation (FR-024); cited signals surfaced so the operator can sanity-check.
Severity: low
