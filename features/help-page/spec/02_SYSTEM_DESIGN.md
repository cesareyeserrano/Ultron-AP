# Help Page — System Design (Phase 2)

Feature: `help-page`
Parent project: Ultron-AP (single Go 1.25.7 binary, linux/arm64 Raspberry Pi)
Inputs: [01_REQUIREMENTS.json](./01_REQUIREMENTS.json) — FR-048 … FR-056, NFR-022 … NFR-026

---

## Executive Summary

The `help-page` feature ships a single authenticated, server-rendered `GET /help` route that renders an embedded glossary of ≥30 entries grouped by five fixed categories. Each entry is rendered in two voices — technical and plain — side-by-side on desktop, stacked on mobile. Content is bundled into the binary via `go:embed` (no SQLite, no runtime filesystem read, no admin UI). A typed in-page filter using ~25 lines of inline vanilla JS gives ≤100 ms keystroke-to-visual-update response. Every rule in the existing `internal/insights` engine has its `Links` validated against the loaded glossary at startup with **warn-on-missing, never-fail** semantics; the dashboard's existing verdict-card partial gains a `Learn more →` anchor pointing at the rule's first valid `/help#…` fragment.

The package boundary is enforced architecturally: the help-page package never imports `internal/insights`, `internal/alerts`, or `internal/notify`. Rule data crosses the boundary via dependency injection from `internal/server` as a plain `[]RuleLink` slice (no insights types leak). This pattern matches the existing parent isolation discipline (NFR-021 from BL-017).

Stack additions are **zero**: existing `net/http` ServeMux, existing `requireAuth` middleware, existing `html/template` stack, existing Tailwind CSS, existing structured logger. No new dependencies, no new datastore, no new SSE channel, no new tooling.

---

## System Architecture

### Component map

```
┌──────────────────────────────────────────────────────────────────────────┐
│                             cmd/ultron-ap (main)                          │
│                                                                           │
│   1. insights.New(cfg) → svc.LoadBundled()                                │
│   2. help.New(svc.RuleLinks()) — DI: pass plain []RuleLink, not engine    │
│   3. server.New(... helpSvc ...).registerRoutes(mux)                      │
└─────────────────┬────────────────────────────────┬───────────────────────┘
                  │                                │
                  ▼                                ▼
   ┌──────────────────────────┐   ┌────────────────────────────────────────┐
   │ internal/insights        │   │ internal/help    (NEW PACKAGE)         │
   │ (BL-017 — unchanged)     │   │                                        │
   │                          │   │  Loader  ── glossary.json (go:embed)   │
   │  + new public method:    │   │     │                                  │
   │    Service.RuleLinks()   │   │     ▼                                  │
   │    []RuleLink            │   │  Glossary (in-memory, immutable)       │
   │                          │   │     │                                  │
   │  RuleLink is defined in  │   │     ├─► Validator (FR-052, warn-only)  │
   │  internal/help, NOT in   │   │     │                                  │
   │  internal/insights —     │   │     ├─► Renderer (html/template)       │
   │  insights.RuleLinks()    │   │     │                                  │
   │  returns []help.RuleLink │   │     └─► AnchorResolver (FR-053 helper) │
   │  via a thin adapter.     │   │                                        │
   │                          │   │  Imports: html/template, embed,        │
   │  ──── ALTERNATIVE ────   │   │           encoding/json, log, net/http │
   │  See ADR-006 below.      │   │  Forbidden imports: internal/insights, │
   └──────────────────────────┘   │           internal/alerts,             │
                  ▲               │           internal/notify              │
                  │               └──────────────┬─────────────────────────┘
                  │                              │
                  │                              ▼
                  │             ┌──────────────────────────────────────────┐
                  │             │ internal/server  (existing)              │
                  │             │                                          │
                  └─────────────┤  + GET /help  → help.Renderer.Render(w)  │
                                │  + verdict-card partial uses             │
                                │    AnchorResolver.FirstValid(rule.Links) │
                                │    to compute the Learn more href        │
                                └──────────────────────────────────────────┘

  web/templates/                       web/templates/partials/
   ├─ base.html (existing)              ├─ sidebar.html  (FR-056: + Help)
   ├─ help.html (NEW — extends base)    └─ sse-verdicts.html (FR-053: link)
   └─ partials/
      └─ help-entry.html (NEW)
```

To avoid an import cycle (`internal/insights` returning a type from `internal/help`) **and** the boundary violation (help importing insights), `RuleLink` lives in a third tiny package `internal/help/contract` that holds **plain data types only** (no behaviour). Both `internal/insights` and `internal/help` import it; neither imports the other. This is the classic Go shared-types pattern. See ADR-006.

### Package layout (final)

```
internal/help/
   help.go            # Service, Loader, Renderer, exported API
   loader.go          # JSON parsing, schema validation (FR-049)
   validator.go       # Links validator (FR-052)
   anchor.go          # AnchorResolver — pure func, used by sse-verdicts.html
   render.go          # html/template glue + view-model assembly
   render_test.go
   loader_test.go
   validator_test.go
   anchor_test.go
   help_test.go
   glossary/
      glossary.json   # Bootstrap content, ≥30 entries (FR-055)
   testdata/
      …               # Synthetic fixtures for loader/validator tests

internal/help/contract/
   contract.go        # type RuleLink struct{ RuleID string; Links []string }
                      # zero behaviour, zero deps

internal/insights/
   insights.go        # + func (s *Service) RuleLinks() []contract.RuleLink
                      #   (3-line accessor, walks s.compiledRules)
```

### Data flow at startup (deterministic, single-threaded)

1. `main` constructs `insights.Service`, calls `LoadBundled()` (existing behaviour).
2. `main` constructs `help.Service` via `help.New(logger)`. Constructor parses the embedded `glossary.json` synchronously. On schema errors, the offending entry is rejected with a structured WARN line (FR-049 AC-1..4); other entries continue. Construction never returns an error for content issues — only for catastrophic embed failure (impossible in practice; treated as fatal).
3. `main` calls `helpSvc.ValidateLinks(insightsSvc.RuleLinks())`. This is the **eager** validator pass (FR-052). For each rule, every `#…` fragment is checked against the loaded entry-id index. Missing fragments emit one structured WARN per missing anchor naming `rule_id` and `missing_anchor`. External `http(s)://` links are skipped. The validator never fails startup, never mutates rules.
4. `main` passes `helpSvc` into `server.New` as a new field. The server registers `GET /help` and uses `helpSvc.AnchorResolver` from inside the verdict-card template helpers.

There is no goroutine, no ticker, no subscription. The help-page is steady-state read-only: the glossary is immutable once loaded; the AnchorResolver is read-only.

### Request flow at runtime (`GET /help`)

```
client ──GET /help──► chi/ServeMux
                        │
                        ▼
                 requireAuth (existing FR-007 middleware)
                        │ unauth → 302 /login (existing behaviour, no leak)
                        │ auth   → next
                        ▼
                 server.handleHelpPage(w, r)
                        │
                        ▼
                 help.Renderer.Render(w, request)
                        │
                        ├─ Build view-model from immutable Glossary
                        │  (zero allocations beyond the slice header)
                        │
                        └─ Execute html/template "help.html"
                            │
                            ▼
                          response (text/html; charset=utf-8)

After initial GET, the typed filter runs **client-side only** — zero further
HTTP. No XHR, no fetch, no SSE.
```

Render-side caching: the rendered HTML body is identical across requests for a given binary build (content is embedded, not user-scoped). We emit a `Cache-Control: private, max-age=300` response header (private because of FR-007 auth) and an ETag derived from a `sha256(glossary.json)` computed once at startup. This brings p99 way under 500 ms even on a cold Pi without engineering effort.

### How each FR/NFR maps to a component

| Req      | Component                           | Notes                                                                    |
| -------- | ----------------------------------- | ------------------------------------------------------------------------ |
| FR-048   | server route + Renderer             | `GET /help` registered on existing mux, behind `requireAuth`             |
| FR-049   | Loader + JSON schema validation     | Strict decoder; unknown-field rejection via `json.Decoder.DisallowUnknownFields` |
| FR-050   | help.html template + Tailwind grid  | Two-column on `md:` breakpoint, stacked default                          |
| FR-051   | help.html `id="entry-<slug>"` + CSS `:target` | Pure CSS highlight, 1.5 s fade via `transition`                  |
| FR-052   | Validator                           | Eager at startup; warn-only; no insights import                          |
| FR-053   | AnchorResolver + sse-verdicts.html  | Pure function `(links []string) (string, bool)`                          |
| FR-054   | Inline JS in help.html              | ~25-line vanilla JS, no framework, no debounce — operates on input event |
| FR-055   | glossary/glossary.json              | Bootstrap content, ≥30 entries                                           |
| FR-056   | partials/sidebar.html               | One new `<a>` element, reuses existing nav-item classes                  |
| NFR-022  | Render caching + small payload      | ETag + max-age=300; embedded JSON is ~30 KB                              |
| NFR-023  | go:embed in loader.go               | Verified by integration test `TestGlossaryRunsWithoutOnDiskFiles`        |
| NFR-024  | Semantic HTML in help.html          | `<section>`/`<article>`, labelled `<input type="search">`                |
| NFR-025  | Slug → id round-trip                | Slugs are immutable; CI test asserts no entry-id removal/rename in PRs   |
| NFR-026  | Package boundary + CI check         | `internal/help/contract` shared types; `make lint` runs `go list -deps`  |

### Failure Blast Radius (critical components)

- **Embedded glossary cannot be parsed at startup.** Detected when `LoadBundled` returns zero valid entries with errors. Recovery path: the binary still starts; `GET /help` returns HTTP 200 with a "Help content unavailable — see server logs" stub (no broken nav, no 5xx). All other Ultron features are unaffected. WARN logged at boot, ERROR per-rejected-entry. Mitigation: build-time CI check (see Risk Analysis) parses `glossary.json` with the production loader and fails the build if any entry is invalid.
- **Insights-engine panic during `RuleLinks()` accessor call.** The accessor is a 3-line read of `s.compiledRules` under `s.mu.RLock` — same lock discipline as `Active()`. Recovery path: the validator skips on accessor error and logs `links-validator-deferred`. Help-page rendering is not affected (validator output is advisory only). Mitigation: `RuleLinks()` is exercised in the existing insights test suite.
- **Race on glossary read during validator + first request.** None — both run after `main` calls `LoadBundled` synchronously, before `http.ListenAndServe`. The `Glossary` value is treated as immutable post-construction; we deliberately do not expose a setter. Documented in code with `// Glossary is immutable after New()`.

---

## Data Model

### Persistence: NONE

Per FR-049, NFR-023, and the no_go_zone, this feature introduces **zero** persistent state: no tables, no migrations, no key-value files, no on-disk cache. The parent FR-015 backup pipeline is unmodified.

### In-memory entities

```go
// In internal/help/contract — shared, behaviour-free.
type RuleLink struct {
    RuleID string
    Links  []string // copy of rule's Links field; advisory
}

// In internal/help — package-internal types.
type Entry struct {
    ID          string       // stable slug; matches /^[a-z0-9][a-z0-9-]*$/
    Title       string       // short headline
    Category    Category     // enum, see below
    Technical   string       // body — what it measures, source, units
    Plain       string       // body — what it means in plain words
    Thresholds  []Threshold  // optional; nil when absent (no empty table)
    SourcePath  string       // optional; "" when absent
}

type Threshold struct {
    Label    string
    Value    string
    Severity string // "info" | "warn" | "critical" — free-form display only
}

type Category string
const (
    CategorySystemMetrics      Category = "system-metrics"
    CategoryNetworkProbes      Category = "network-probes"
    CategoryServicesContainers Category = "services-containers"
    CategoryVPN                Category = "vpn"
    CategoryInsightsVerdicts   Category = "insights-verdicts"
)

// Glossary is the loaded, immutable catalogue.
type Glossary struct {
    entries  []Entry
    byID     map[string]int // O(1) anchor lookup; used by AnchorResolver
    byCat    map[Category][]int // ordered slice of indexes per category
    sha256   string // for ETag
}
```

`byID` is the single source of truth for "is this anchor real?" — used by both the FR-052 validator and the FR-053 AnchorResolver. Equal-cost lookup, no string scan.

### JSON schema (the `glossary.json` wire format)

```json
{
  "version": 1,
  "entries": [
    {
      "id": "verdict-thermal-throttling",
      "title": "Thermal throttling probable",
      "category": "insights-verdicts",
      "technical": "Triggered when CPU temperature ≥ 80 °C **and** any of {cpu_freq_throttled, soft_lockup, ARM cpufreq capped} is observed. Source: internal/metrics/system.go (sample), evaluated by internal/insights/rules/rules.go (rule id thermal-throttling).",
      "plain": "Your Pi is too hot, so the CPU is slowing itself down to cool off. Programs may feel laggy until temperature drops below the threshold.",
      "thresholds": [
        {"label": "warn",     "value": "≥ 80 °C", "severity": "warn"},
        {"label": "critical", "value": "≥ 85 °C", "severity": "critical"}
      ],
      "source_path": "internal/insights/rules/rules.go"
    }
  ]
}
```

Wire-level rules:
- `version` is required and must equal `1`. A future `version: 2` allows breaking schema changes without surprising old binaries.
- `entries` is required.
- Per-entry required fields: `id`, `title`, `category`, `technical`, `plain`. Optional: `thresholds`, `source_path`.
- Unknown top-level fields and unknown per-entry fields are **rejected**: the loader uses `json.Decoder.DisallowUnknownFields()` (FR-049 AC-4).
- `id` must match `^[a-z0-9][a-z0-9-]*$` and not exceed 64 chars. Validated by regex; failures logged.
- Duplicates: only the first occurrence is kept; the second is logged with `event=duplicate-entry-id, id=<id>` (FR-049 AC-3).
- HTML in `technical` / `plain`: bodies are treated as plain strings and rendered through `{{.}}` (auto-escaped). Inline `<code>` is desirable but disallowed in v1 — operators wanting code formatting use Markdown-like asterisks rendered as bold via the template (no Markdown engine added; see no_go_zone).

### Slug stability contract (NFR-025)

Slugs are part of the public surface — every `#…` fragment in user-shared URLs depends on them. Renaming a slug is a breaking change. Removal returns a hash that is ignored gracefully (FR-051 AC-5), per design.

---

## API Design

### New HTTP endpoints

| Method | Path    | Auth          | Returns                            | Notes                                |
| ------ | ------- | ------------- | ---------------------------------- | ------------------------------------ |
| GET    | `/help` | requireAuth   | 200 `text/html; charset=utf-8`     | Server-rendered; cache 5 min private |

That is the **only** new endpoint. There is no `/api/help/*`, no POST, no DELETE. NFR-023 AC-2 explicitly forbids it.

### Request/response contract for `GET /help`

**Request**
- Method: GET
- Headers (consumed): `Cookie` (parent session), `If-None-Match` (304 short-circuit on cached ETag)
- Headers (ignored): query parameters; the URL hash fragment is browser-side only

**Response (200)**
- `Content-Type: text/html; charset=utf-8`
- `Cache-Control: private, max-age=300`
- `ETag: "<sha256-prefix-16>"` (computed once at startup over `glossary.json`)
- Body: full HTML document extending `base.html`. Body skeleton:
  ```html
  <main id="help-page">
    <header><h1>Help & glossary</h1></header>
    <input type="search" id="help-filter" aria-controls="help-entries"
           aria-label="Filter glossary"
           placeholder="Type to filter…">
    <div id="help-entries">
      <section id="cat-system-metrics" data-category="system-metrics">
        <header><h2>System metrics</h2></header>
        <article id="entry-cpu-percent" class="help-entry"
                 data-search="cpu percent processor load …">
          <h3>CPU %</h3>
          <div class="grid md:grid-cols-2 gap-6">
            <div><span class="label">Technical</span><p>…</p></div>
            <div><span class="label">Plain</span><p>…</p></div>
          </div>
          <table class="thresholds">…</table>
          <code class="source-path">internal/metrics/system.go</code>
        </article>
        … more entries …
      </section>
      … other categories in fixed order …
    </div>
    <script>/* inline filter, see Performance & Scalability */</script>
  </main>
  ```

**Response (302)** — unauthenticated request, identical to the dashboard's existing FR-007 redirect to `/login`. Reuses `requireAuth` middleware verbatim.

**Response (304)** — sent when `If-None-Match` matches the startup ETag.

**Response (500)** — only on catastrophic template execution failure (e.g. corrupted binary). The Loader-on-startup pathway means content errors never produce a 5xx at request time.

### Internal Go API (the help package's exported surface)

```go
// Public API consumed by internal/server.
type Service struct { /* unexported */ }

func New(log LogFunc) (*Service, error)                  // parses embedded glossary
func (s *Service) ValidateLinks(rules []contract.RuleLink) // FR-052 entry point
func (s *Service) Handler() http.Handler                  // GET /help handler
func (s *Service) FirstValidAnchor(links []string) (href string, ok bool) // FR-053
func (s *Service) EntryCount() int                        // for the boot log line

type LogFunc func(format string, args ...interface{})    // matches insights.LogFunc shape
```

That is the entire public surface of `internal/help`. The two non-obvious choices:
- `Handler()` returns `http.Handler` rather than a free function so the server wires it via `mux.Handle("GET /help", s.requireAuth(helpSvc.Handler()))` — no new server method, no new helper.
- `FirstValidAnchor` returns `(string, bool)` so the verdict-card template helper can branch with a single `{{if}}` and skip rendering the anchor entirely (FR-053 AC-3).

### Insights package — new public accessor

```go
// Added to internal/insights/insights.go.
//
// RuleLinks returns a snapshot of every loaded rule's id and links field as
// plain data, suitable for the help-page links validator (FR-052). The slice
// is owned by the caller; mutating it does not affect the engine.
//
// @aitri-trace FR-052
func (s *Service) RuleLinks() []contract.RuleLink {
    s.mu.RLock()
    defer s.mu.RUnlock()
    out := make([]contract.RuleLink, 0, len(s.compiledRules))
    for _, cr := range s.compiledRules {
        links := append([]string(nil), cr.rule.Links...) // defensive copy
        out = append(out, contract.RuleLink{
            RuleID: cr.rule.ID,
            Links:  links,
        })
    }
    return out
}
```

This is the only change to `internal/insights`. It is a public accessor over already-public data; no behaviour is altered.

### Filter API: client-side (NFR-022, FR-054)

The filter is **not** an API — it is inline JS embedded in `help.html`:

```js
// 23 lines, no framework, no debounce.
(function () {
  const input = document.getElementById('help-filter');
  if (!input) return;
  const entries = document.querySelectorAll('.help-entry');
  const sections = document.querySelectorAll('section[data-category]');

  input.addEventListener('input', () => {
    const q = input.value.trim().toLowerCase();
    sections.forEach(sec => sec.classList.remove('hidden'));
    if (!q) {
      entries.forEach(e => e.classList.remove('hidden'));
      return;
    }
    sections.forEach(sec => {
      let anyVisible = false;
      sec.querySelectorAll('.help-entry').forEach(e => {
        const hay = e.dataset.search; // pre-lowercased on server
        const hit = hay.includes(q);
        e.classList.toggle('hidden', !hit);
        if (hit) anyVisible = true;
      });
      if (!anyVisible) sec.classList.add('hidden');
    });
  });
})();
```

The `data-search` attribute is rendered server-side as the **lowercased** concatenation of `title + " " + technical + " " + plain` (single string, single allocation per entry per request). Lowercasing once on the server avoids per-keystroke `.toLowerCase()` over every body text — turns ~30 entries × ~500 chars × per-keystroke from a costable iteration into a single `String.includes` per entry. Keystroke→repaint stays well under 100 ms even on a Pi-served viewport.

---

## Security Design

### Authentication
- `GET /help` is wrapped in the existing `requireAuth` middleware. Identical behaviour and identical redirect target as `/`, `/docker`, `/services`, etc. (FR-048 AC-2). Zero new auth code.

### Authorization
- All authenticated users have full read access to the glossary. The glossary contains no per-user, per-tenant, or sensitive operational data — it documents metric semantics, not metric values. No RBAC is added.

### Input handling
- The only user input on this surface is the typed filter, which is **client-side only**. It never touches the server, so there is no server-side injection vector to consider.
- The URL hash fragment (`/help#…`) is browser-controlled and never reaches the server. CSS `:target` matching is hash-based and safe by construction.
- The glossary content is build-time, not user-supplied. Content authors are committers; trust boundary is the git review process.

### Output handling — XSS safety
- All entry fields are rendered through `html/template` with auto-escaping (`{{.Technical}}`, `{{.Plain}}`, etc.). No `template.HTML` casts.
- `data-search` attributes are written via `{{.SearchHaystack}}` — auto-escaped for HTML attribute context.
- Slugs flow into HTML id attributes (`id="entry-{{.ID}}"`) and href fragments. The slug regex (`^[a-z0-9][a-z0-9-]*$`, 64 char max) is enforced at load time and is a strict subset of HTML-safe characters; no escaping concern remains.
- The inline `<script>` block is a static literal — no template substitution into JS context (which would be the only path to a script-injection problem).

### Transport
- Same server, same TLS posture as the parent dashboard. No new transport surface.

### CSRF
- `GET /help` is idempotent and side-effect-free; no CSRF token required (matches the dashboard's other GET pages).

### Encryption at rest
- Not applicable — no persistent data is added.

### Logging
- Validator log lines emit `rule_id` and `missing_anchor` only. These are not sensitive (rule ids and glossary slugs are public to anyone with dashboard access). No PII, no secrets.

---

## Performance & Scalability

### Targets (NFR-022)

| Metric                              | Target            | Approach                                     |
| ----------------------------------- | ----------------- | -------------------------------------------- |
| Server-side render p99              | <500 ms on Pi     | Cached ETag short-circuit + 30 KB body       |
| First render (cold) p99             | <500 ms on Pi     | template.Execute over ~30 entries; <50 ms typical |
| Filter keystroke→visual update p99  | <100 ms           | Single `String.includes` per entry, no DOM rebuild |
| Network XHR/fetch after first GET   | 0                 | Filter is client-side; no progressive endpoints |

### Sizing & headroom
- Glossary count at ship: ≥30 entries, expected ~50 within a year. Hard ceiling stated in no_go_zone: ≤100 entries before a follow-up indexing strategy is warranted. At 100 entries × ~2 KB rendered HTML each, page weight is ~200 KB — still well within the budget.
- Server-side render: `html/template` execution over 100 entries on the Pi's ARM64 cores measures sub-20 ms in our existing dashboard render path (a tighter loop). 500 ms p99 is generous; the ETag cache is belt-and-braces.

### Filter performance characterization
- Per keystroke: `entries.length` (≤100) string-include checks against a pre-lowercased haystack of ~500 chars each. Worst case ~50 KB of haystack scanned per keystroke — V8/JavaScriptCore handle this in <2 ms. The DOM update is `classList.toggle` on ≤100 elements; browser layout/paint dominate at ~10–20 ms. p99 budget of 100 ms is a 5× margin.
- No debouncing is needed at this scale and is deliberately omitted to keep the inline JS at ≤30 lines.

### Caching strategy
- ETag is `sha256(glossary.json)[:16]` computed once at `help.New()` and stored on the Service. The handler emits `ETag: "<value>"` and short-circuits with 304 when `If-None-Match` matches. This matters mostly when an operator hops between dashboard and help frequently — repeat GETs become 304s with empty bodies.
- `Cache-Control: private, max-age=300` allows the browser to skip the round-trip entirely for 5 minutes. `private` is required because the route is auth-gated.

### No back-pressure surface
- `GET /help` is an O(1) page load. No fan-out, no upstream call, no DB query. There is no path to back-pressure regardless of request rate.

---

## Deployment Architecture

### Distribution

The feature ships as part of the existing single-binary Ultron-AP build:

```
make build                 → produces bin/ultron-ap (linux/arm64)
                             with templates/ + static/ + glossary.json embedded
sudo systemctl restart ultron-ap.service
```

`glossary.json` lives at `internal/help/glossary/glossary.json` and is embedded via:

```go
//go:embed glossary/glossary.json
var glossaryJSON []byte
```

Removing `internal/help/glossary/glossary.json` from the filesystem after `go build` does not affect the running binary (NFR-023 AC-1). The integration test `TestGlossaryRunsWithoutOnDiskFiles` enforces this by running the binary in a `chroot`-like temp directory after build.

### Configuration
- Zero new environment variables.
- Zero new flags.
- Zero new config-file keys.

### Health, readiness, and observability
- Existing `/health` endpoint is unaffected. The help-page is not part of the readiness path — its failure does not flip readiness.
- A single new structured log line at boot: `event=glossary-loaded entries=<N>` (FR-055 AC-1).
- Per-rejected-entry boot log: `event=glossary-entry-rejected id=<id-or-index> reason=<reason>` (FR-049).
- Per-missing-anchor validator boot log: `event=insights-link-missing rule_id=<id> missing_anchor=<#…>` (FR-052).
- No new metrics. The handler does emit standard request log lines via the existing access-log middleware.

### Migration / rollout
- No data migration. No schema migration. No cache invalidation. Rollout is a binary swap.
- Rollback: revert binary; previous version had no `/help` route, no sidebar item — operator sees nothing degraded except the absence of the new feature.

### CI additions
1. **Build-time glossary lint** (`make lint`): the same Loader used at runtime parses `glossary.json` and asserts ≥30 entries, all categories represented per FR-055, exactly 10 entries with category=insights-verdicts, all bundled rules from `insights.New(...).RuleLinks()` resolve to a real entry. Failure breaks the build (FR-055 AC-7).
2. **Architectural boundary check** (`make lint`): `go list -deps ./internal/help/... | grep -E 'internal/(insights|alerts|notify)'` exits non-zero if any dependency leaks. CI step fails the PR (NFR-026 AC-3).
3. **Slug-stability check** (`scripts/check-glossary-slugs.sh`): in PR mode, compares the set of `id` values in `glossary.json` between base and head; flags removals/renames as breaking. The check is advisory (not blocking) — operators can still ship breaking removals consciously.

---

## ADRs

### ADR-001 — Glossary content format and embedding

**Decision:** Single `glossary.json` file, embedded via `go:embed`, parsed at startup with a strict decoder.

**Options evaluated:**

1. **One JSON file, `go:embed`** (chosen). Familiar to Go contributors, machine-validatable, easy to diff in PRs, decoder is in stdlib, schema enforced via `DisallowUnknownFields`. Single-file readability remains good at <200 entries.
2. **Directory of one-file-per-entry JSON**. Pros: smaller diffs in PRs, easier to grep. Cons: doubles the file count for ~30 entries, multiplies parse iterations, and provides no real benefit until we have ≥100 entries (which is the explicit no_go_zone ceiling). Defer until needed.
3. **YAML**. Pros: more human-friendly for body text (multi-line strings, no escapes). Cons: adds a new dependency (`gopkg.in/yaml.v3`); YAML's quirks (booleans, anchors, indent ambiguity) cause more confusion than they save in entry authoring; `go fmt` does not lint YAML.
4. **Go struct literals (`var glossary = []Entry{ {…} }`)**. Pros: zero parse time, compile-time validation. Cons: every entry edit requires a Go recompile; tooling for non-coders is worse than JSON; multi-line body strings are ugly with backtick raw strings; loses the "data, not code" property that simplifies the FR-049 schema validation tests.

**Trade-off accepted:** JSON's verbosity for multi-line bodies is worth the lint-ability and zero-dependency gain. Body fields use `\n` for paragraph breaks; the renderer converts `\n\n` to paragraph splits via `strings.Split` (no Markdown engine).

### ADR-002 — HTTP route registration pattern

**Decision:** Register `GET /help` directly on the existing `http.ServeMux` in `internal/server/server.go`, behind the existing `requireAuth` middleware. No new router, no new middleware.

**Options evaluated:**

1. **Extend existing ServeMux** (chosen). Matches the existing convention used by every other dashboard route; reuses `requireAuth`; zero new dependencies. The new `mux.Handle("GET /help", s.requireAuth(helpSvc.Handler()))` line in `registerRoutes` is the entire integration.
2. **Sub-mount the help-page on its own sub-router**. Pros: encapsulation. Cons: the feature has exactly one HTTP route — sub-mounting adds indirection without any payoff. Not worth the complexity.
3. **chi/gin/gorilla**. Out of scope: the parent project uses stdlib `net/http` exclusively. Adding a router framework for one route is unjustifiable.

### ADR-003 — Two-voice rendering layout (FR-050)

**Decision:** Tailwind utility grid: `grid md:grid-cols-2 gap-6` per entry. Plain comes first in DOM order so mobile (single-column) shows it first.

**Options evaluated:**

1. **CSS grid via Tailwind utilities** (chosen). Zero new CSS, responsive built-in via the `md:` breakpoint, no JS, satisfies FR-050 AC-3 (works with JS disabled).
2. **CSS flexbox with `flex-direction: column-reverse`** below `md:`. Pros: similarly utility-driven. Cons: `column-reverse` reverses tab order and screen-reader order, making DOM-order primacy of plain voice harder to express clearly. Grid is the natural fit.
3. **Tabs / accordion / toggle**. Explicitly rejected by FR-050 ("Both voices visible simultaneously by default; not behind a toggle, not in tabs, not collapsed").

### ADR-004 — Anchor highlight on hash arrival (FR-051)

**Decision:** Pure CSS `:target` selector with a 1.5 s background-color transition that fades to transparent.

```css
.help-entry:target {
  animation: help-target-flash 1.5s ease-out 0s 1 forwards;
}
@keyframes help-target-flash {
  0%   { background-color: var(--color-accent-soft); }
  100% { background-color: transparent; }
}
```

**Options evaluated:**

1. **Pure CSS `:target` with animation** (chosen). Zero JS. Browser-native scrolling and highlight. Highlight automatically re-fires on subsequent same-hash arrivals.
2. **JS `hashchange` listener that toggles a class**. Pros: more control over duration and exit conditions. Cons: more code; needs handling for `DOMContentLoaded` initial state vs. subsequent `hashchange`; explicitly disallowed by NFR-024 ("no CSS animation or transition longer than 150 ms ... with the single exception of the FR-051 :target highlight"). Pure CSS satisfies the requirement directly.
3. **No highlight, scroll only**. Rejected — FR-051 AC-2 requires the highlight.

### ADR-005 — In-page filter implementation (FR-054)

**Decision:** ~25 lines of inline vanilla JavaScript using `String.includes` against a server-pre-lowercased `data-search` attribute on each entry. CSS `:has()` was evaluated and rejected.

**Options evaluated:**

1. **Inline vanilla JS + `data-search`** (chosen). Trivial, debuggable, satisfies the ≤100 ms latency budget by orders of magnitude, degrades gracefully when JS is disabled (filter input is non-functional but page is fully usable per FR-054 AC-6).
2. **CSS `:has()` driven by an `<input>` and `:not(:has([data-search*="…" i]))`**. The case-insensitive substring selector exists, but the **search term** must be a literal in the stylesheet — there is no CSS feature that lets the typed value drive the selector dynamically without JS that mutates the stylesheet. So a "pure CSS" filter would require generating a `<style>` block at runtime via JS, which is more complex than the JS-direct approach and harder to reason about. Rejected.
3. **HTMX server-side filter**. Rejected — would issue an XHR per keystroke (or even debounced), explicitly forbidden by NFR-022 AC-4 ("no additional XHR / fetch calls are issued after the initial GET /help") and unnecessary at this entry count.
4. **JS framework (Alpine, lit, etc.)**. Forbidden by FR-054 AC-5.

**Trade-off accepted:** A 25-line inline `<script>` slightly violates the spirit of "no JS" purism, but the FR explicitly permits "inline vanilla JS or CSS :has() — no framework, no bundler". This is the minimal sufficient implementation.

### ADR-006 — Architectural boundary enforcement (NFR-026)

**Decision:** Introduce `internal/help/contract` — a tiny package containing only the `RuleLink` data type. Both `internal/insights` and `internal/help` import `internal/help/contract`. Neither imports the other directly. CI lint enforces the boundary via `go list -deps`.

**Options evaluated:**

1. **Shared types package `internal/help/contract`** (chosen). Standard Go pattern. Zero behaviour, zero deps. The insights `RuleLinks()` accessor returns `[]contract.RuleLink`. The help-page validator consumes `[]contract.RuleLink`. Compiles into no actual import-graph cycle, satisfies NFR-026 AC-1 cleanly.
2. **Dependency injection of `[]struct{ID string; Links []string}`**. Pros: even more decoupled — no shared type. Cons: anonymous struct types are awkward across package boundaries; loses the named type's documentation; `RuleLink` is a stable enough public concept that giving it a name is correct.
3. **Interface in `internal/help` consumed by insights**. Pros: classic Go inverted-dependency style. Cons: insights would have to import `internal/help` to satisfy the interface signature, which inverts the boundary back into a violation.
4. **Skip the boundary** (just import `internal/insights` from help). Rejected by NFR-026.

**Failure-mode note:** if a future contributor adds an import of `internal/insights` from `internal/help`, the CI step `go list -deps ./internal/help/... | grep internal/insights` triggers and fails the PR. This is the architectural enforcement promised by NFR-026 AC-3.

### ADR-007 — Links validator scheduling (FR-052)

**Decision:** Eager validation, run once synchronously in `main` after both glossary and rules are loaded.

**Options evaluated:**

1. **Eager, synchronous, one-shot at startup** (chosen). Simple, deterministic, log lines appear in the boot log where operators look. No goroutine, no scheduling complexity.
2. **Lazy on first verdict-card render**. Pros: only pays the cost when needed. Cons: per FR-052 AC-1/AC-2 the WARN log is the explicit signal to the developer who shipped the broken anchor — they want it at startup, not the next time a verdict triggers (which could be days later).
3. **Periodic re-validation via ticker**. Pros: catches drift. Cons: rules and glossary are immutable for the lifetime of the binary; periodic re-validation re-emits the same WARN every period for no new information. Actively bad.

**FR-052 AC-3 deferral:** if either glossary or rules are not yet loaded when the validator runs, log `event=links-validator-deferred` and retry once on the next tick. In practice, `main`'s init order makes this impossible — but the retry is documented behaviour for safety.

---

## Risk Analysis

### Operational risks

**R1 — A new bundled rule ships with a typo in `links` and the WARN passes unnoticed in the boot log.**
Likelihood: medium. Impact: low (operator clicks a missing anchor, page renders at the top per FR-051 AC-5; no error). Mitigation: the build-time CI glossary lint (Deployment Architecture §CI additions) cross-validates rule links against the glossary at PR time and fails the build, not just emits a runtime warning. This converts the runtime warning into a compile-time error for any in-tree change.

**R2 — Slug rename in a PR silently breaks user-shared `/help#…` URLs.**
Likelihood: low (slugs are infrequent edits). Impact: medium (broken links shared in chat / docs). Mitigation: the slug-stability check (advisory CI step) flags removals/renames in PRs. The check is advisory rather than blocking because legitimate removals must be possible.

**R3 — Glossary parsing is permissive of HTML in body fields (`technical`, `plain`).**
Likelihood: very low (entries are committed by trusted authors). Impact: low (XSS only against the same operator who is already authenticated). Mitigation: `html/template` auto-escaping is unconditional for `{{.Technical}}` / `{{.Plain}}`. We do **not** ever cast to `template.HTML`. Reviewer guidance lives in the glossary file's leading comment.

**R4 — Filter performance degrades catastrophically at >>100 entries.**
Likelihood: low (no_go_zone caps content growth at v1). Impact: medium (filter latency exceeds 100 ms). Mitigation: hard ceiling documented in no_go_zone; if approached, ADR-001 option 2 (per-entry files) and a lightweight client-side index become the v2 plan. Not addressed in v1.

**R5 — Boundary erosion (NFR-026) over time as developers reach for `insights.Service` types.**
Likelihood: medium (depends on team discipline). Impact: low at first, compounding over time. Mitigation: CI lint is mandatory — failing the PR is a non-negotiable signal. The `internal/help/contract` package's package doc explicitly states "this exists to enforce NFR-026 — do not add behaviour".

### Technical risks

**T1 — `html/template` whitespace handling produces large rendered HTML on Pi.**
Likelihood: low. Impact: low (modest extra bytes). Mitigation: rendered body stays well under 200 KB even at 100 entries; ETag caching makes the wire cost negligible after first request.

**T2 — Browser support for CSS `:target` animation timing is inconsistent.**
Likelihood: very low (well-supported since ~2018). Impact: low (highlight is shorter or longer than 1.5 s in some browsers — still satisfies FR-051 AC-2's 1–2 s window). Mitigation: none needed.

**T3 — `html/template` auto-escaping is bypassed accidentally.**
Likelihood: very low. Impact: high (XSS). Mitigation: code review checklist enforces "no `template.HTML` casts in `internal/help`"; a unit test (`render_test.go`) asserts that body containing `<script>alert(1)</script>` renders escaped.

---

## Technical Risk Flags

**[RISK] None detected with severity ≥ medium.** All identified risks are mitigated within the design above (CI gates for R1, R2, R5; auto-escaping for R3 and T3; no_go_zone for R4; broad browser support for T2).

The decision to pre-lowercase `data-search` server-side (Performance & Scalability §Filter API) is the only place where a content author's input bypasses the auto-escaping path *into an HTML attribute* — and it goes through `{{.SearchHaystack}}` which is auto-escaped for HTML attribute context. No new escape-context boundary is introduced.

---

## Traceability Checklist

- [x] Every FR-* and NFR-* is addressed in either the component map (System Architecture) or the API Design / Data Model / Performance / Deployment sections. See the explicit FR/NFR-to-component table.
- [x] Every significant tech decision has an ADR with ≥2 options evaluated (ADRs 1–7).
- [x] Failure blast radius documented for ≥2 critical components: embedded glossary parse failure, insights-engine accessor panic, glossary-read concurrency.
- [x] no_go_zone items are NOT present in the architecture: no admin UI, no SQLite, no i18n, no Markdown engine, no full-text search index, no `/help/<id>` deep routes, no SSE on `/help`, no internal/insights / alerts / notify imports from `internal/help`.
- [x] Tech stack is compatible with parent constraints: Go 1.25.7, linux/arm64, html/template, Tailwind, single binary, go:embed.
- [x] Data model covers all persistence FRs — none required, explicit "Persistence: NONE".
- [x] API design covers all integration and logic FRs — single GET /help, internal Go API for help.Service, insights.Service.RuleLinks() accessor.
- [x] Technical Risk Flags section is complete — declared "None detected with severity ≥ medium" with justification.

---

## @aitri-trace

FR-048 FR-049 FR-050 FR-051 FR-052 FR-053 FR-054 FR-055 FR-056
NFR-022 NFR-023 NFR-024 NFR-025 NFR-026
US-048 US-049 US-050 US-051 US-052 US-053 US-054 US-055 US-056
