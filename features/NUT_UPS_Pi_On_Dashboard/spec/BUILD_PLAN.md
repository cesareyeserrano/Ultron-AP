# BUILD_PLAN — NUT_UPS_Pi_On_Dashboard

Working file (no Aitri gate). Epics ordered by dependency. Every TC id in `03_TEST_CASES.json` (55 total) appears in exactly one epic's `Makes pass`. Build steps per epic: skeleton → persistence/integrations → hardening.

Target layout (change to an existing system — code lives in the real repo, not `src/`):
- New package: `internal/ups/` (client, poller, state, estimator, store, mock, config, snapshot)
- New template: `web/templates/partials/sse-ups.html`
- Edits: `internal/database/sqlite.go`, `internal/server/sse.go`, `internal/server/handlers*.go`/`templates.go`, `internal/alerts/engine.go`, `internal/insights/*`, `internal/config/*`, `web/templates/dashboard.html`, `.env.example`, CI workflow.

---

## Epic 1 — NUT client foundation (poll, config, mock)   [status: done]
  Evidence: 15/15 TCs green (14 Epic-1 + TC-036f contained-panic, implemented in poller). `go test ./internal/ups/` PASS; full-repo `go test ./...` PASS (no regression).
  Delivers:    US-016, US-024
  FRs:         FR-016, FR-024  (+ NFR-017 localhost-only, NFR-021 test suite/CI, NFR-022 mock)
  Makes pass:  TC-UPS-001h, TC-UPS-002e, TC-UPS-003f, TC-UPS-004e, TC-UPS-031h, TC-UPS-032e, TC-UPS-033f, TC-UPS-037h, TC-UPS-038e, TC-UPS-039f, TC-UPS-050h, TC-UPS-051e, TC-UPS-052f, TC-UPS-054f
  Build steps: skeleton (client.go NUT parser, config.go, state.go ParseStatus, mock.go) → integrations (poller.go loop+backoff, ULTRON_UPS_MOCK wiring, CI workflow) → hardening (read-only guard test, localhost-only, no-priv, table-driven state map)
  Why here:    Everything else consumes the client + Snapshot + config; nothing renders or alerts without polling first. The mock lands here so every later epic can be validated locally.
  Note:        TC-053h / TC-055e (NFR-022 "renders the card") moved to Epic 2 — they need the card partial that Epic 2 builds; TC-054f (mock off by default) stays here (pure client wiring).

## Epic 2 — Live UPS card + estimator + shutdown block   [status: done]
  Evidence: 24/24 TCs green (10 estimator/ups + 14 server render, incl. escaping/no-control/mock-render). Full-repo `go test ./...` PASS. Verified LIVE (`ULTRON_UPS_MOCK=OB`).
  Layout refinement (owner request, within FR-017 — same content, no FR/AC/TC scope change): split into (a) a COMPACT UPS tile in the metrics grid (family member, principal status + battery% subline, added to sse-metrics.html) and (b) a DETAIL panel below (sse-ups.html — carga/entrada/batería/beeper + shutdown block). TC-008e updated to assert the compact tile renders in the grid.
  Delivers:    US-017, US-018, US-023
  FRs:         FR-017, FR-018, FR-023  (+ NFR-016 clean degradation, NFR-018 no-shutdown path, NFR-019 escaping)
  Makes pass:  TC-UPS-005h, TC-UPS-006e, TC-UPS-007f, TC-UPS-008e, TC-UPS-009h, TC-UPS-010h, TC-UPS-011e, TC-UPS-012f, TC-UPS-027h, TC-UPS-028e, TC-UPS-029f, TC-UPS-030h, TC-UPS-034h, TC-UPS-035e, TC-UPS-036f, TC-UPS-040h, TC-UPS-041e, TC-UPS-042f, TC-UPS-043h, TC-UPS-044f, TC-UPS-045f, TC-UPS-046f, TC-UPS-053h, TC-UPS-055e
  Build steps: skeleton (estimator.go, sse-ups.html partial, DashboardData.UPS field) → integrations (SSE `ups` swap, dashboard.html tile, shutdown block read-only) → hardening (escaping tests, no-control assertion, degraded 'Sin datos', panic containment, secret-not-logged)
  Why here:    The P0 visible product — resolves ~80% of the value and is the thing you validate in the browser with the Epic-1 mock. Depends on Epic 1's Snapshot/config.

## Epic 3 — History persistence + outage events   [status: done]
  Evidence: 6/6 TCs green (013 restart-survival, 014 empty-series, 015 purge-boundary, 016 open, 017 close+duration, 018 no-double-count-on-restart). Full-repo `go test ./...` PASS. Added: ups_samples/ups_events tables, internal/ups/store.go, poller sampling + OL↔OB event detection, PruneUPSSamples wired into startRetentionJob, and a 24h battery-voltage sparkline in the detail panel (FR-019 chart).
  Delivers:    US-019, US-020
  FRs:         FR-019, FR-020
  Makes pass:  TC-UPS-013h, TC-UPS-014e, TC-UPS-015f, TC-UPS-016h, TC-UPS-017e, TC-UPS-018f
  Build steps: skeleton (ups_samples/ups_events CREATE TABLE, store.go writes) → integrations (sampling on each poll, open/close event on OL↔OB, PruneUPSSamples wired into startRetentionJob, 24h/7d chart series) → hardening (restart-survival, reconcile-open-on-boot no double count, empty-state)
  Why here:    Adds durable history + the "how many outages this month" answer. Depends on Epic 1 (Snapshot) and Epic 2 (state transitions); the alert engine (Epic 4) reads ups_events for durations.

## Epic 4 — Power alerts (engine extension + Telegram)   [status: done]
  Evidence: 8/8 TCs green (019 OB→warning, 020 OL→resolve+duration, 021 voltage-debounce, 022 LB→critical, 023 unreachable-once; 047 bounded logging, 048 recovery-once, 049 timestamped). Full-repo `go test ./...` PASS. Added internal/ups/alerter.go (rules + debounce/dedup via injected sink), wired into the poller + a main.go sink → notify dispatcher (Telegram) + CreateAlert history.
  Delivers:    US-021
  FRs:         FR-021  (+ NFR-020 bounded/observable logging)
  Makes pass:  TC-UPS-019h, TC-UPS-020e, TC-UPS-021f, TC-UPS-022f, TC-UPS-023f, TC-UPS-047h, TC-UPS-048e, TC-UPS-049f
  Build steps: skeleton (evaluateUPSRule branch in engine.go, RecentUPSSamples/Events queries) → integrations (OB/LB/RB/voltage/unreachable rules → CreateAlert + notify; OL-return → emitResolve with duration; debounce via rule_state) → hardening (dedup once-per-day/once-per-outage, bounded reconnect logging, timestamped lines)
  Why here:    Needs Epic 3's ups_events (outage duration) and Epic 1's unreachable state. This is the "avísame por Telegram" payoff.

## Epic 5 — Insights   [status: done]
  Evidence: 3/3 TCs green (024 weekly-outage insight, 025 real count=3, 026 no-degradation-from-single-sample). Full-repo `go test ./...` PASS. Added internal/ups/insights.go (dedicated producer — see technical_debt: bundled engine locked at 10 rules by FR-047 + static verdict text). Rendered as "Observaciones" in the detail panel. ALL 55 TCs green.
  Delivers:    US-022
  FRs:         FR-022
  Makes pass:  TC-UPS-024h, TC-UPS-025e, TC-UPS-026f
  Build steps: skeleton (UPS-derived vars: ups_outages_7d, resting-battery trend) → integrations (feed insights EvalWithVars; static verdict text) → hardening (no degradation claim from a single sample)
  Why here:    P2, reads everything below it (events + samples). Last because it depends on Epics 3/4 data and is the lowest priority.

---

### Adversarial pass (post-build, independent agent — verified against code)
Verdict: ship-with-fixes. 0 critical/high. Confirmed read-only, escaping-safe, race-clean, mock-can't-leak. Fixes applied:
- #2 (MEDIUM): insights ran an unindexed `ups_samples(state)` scan every 5s SSE tick → added `idx_ups_samples_state_ts`, made `restingVoltageDrop` bounded (COUNT + first/last decile via LIMIT), and cached battery-series + insights at poll time (poller.refreshCache), so the render path does zero DB work.
- #1 (MEDIUM): the secret-leak test captured only the format string, not args → now renders `fmt.Sprintf(format, args...)` so a leak in args is caught.
- #5 (LOW): `failures`/`reconnects` now incremented under `p.mu`.
- #3 (LOW): added a battery-degradation trend test (non-TC coverage).
Skipped: #4 (nutUnquote two-pass unescape) — cosmetic, not exploitable for real NUT values.
All 55 TCs + extra tests green with `-race`; full-repo `go test ./...` PASS.

### Verify (post-approval)
- Fixed `test_runner` + `vet` gate to run from the feature dir (`../../internal/...`) — `./...` matched no packages there.
- verify-complete's per-AC gate caught AC-021-004 (RB once/day) with no TC — a Phase-3 omission (US-021 had 6 ACs, 5 TCs). Added **TC-UPS-056f** (RB rule) + test → now 56 TCs.
- Result: **✅ verify passed — 56/56 (45 unit + 11 e2e), vet PASS, every AC covered.**

### Post-deploy layout refinements (owner requests, within FR-017/FR-019 — no FR/AC/TC scope change)
1. Compact UPS tile joined the metrics grid (family member); detail panel relocated before Operational Indicators.
2. Battery chart NORMALIZED to the network-latency tile chrome (colored current value by battery health, mono subtitle, 44px axis max/avg/min, colored sparkline, min/avg/max/samples footer; added `sparkAvg` helper + Snapshot.BattSeriesClass/Stroke).
3. Battery chart MOVED into the charts area as a sibling "UPS history" section (sse-charts.html), respecting the 5m…24h window selector — the poller cache now keeps timestamped Samples and gatherChartData slices them in memory (still zero DB on render). The detail panel no longer embeds a chart.
4. UPS panel DISSOLVED into a System Summary card (sse-ups.html restyled to the Apps/VPN/Containers card chrome; #ups-slot moved into the summary section). All FR-017 content preserved (state, load, input, battery+estimado, beeper, Sin datos, shutdown block) — TCs unchanged and green.
5. "carga" chart REPLACED — first by "frecuencia", then (owner value call: "what matters is what the UPS delivers to the Pi = continuity") by the **"cortes" tile**: outage count in the window (green 0 / yellow ≥1 / red in-progress), red↔bat timeline from sample states, and "último: hace X · duró Y / N cortes · T en batería" — FR-020's question rendered on the dashboard. ups.load AND input.frequency are still PERSISTED (input_freq via guarded additive ALTER, verified on a populated DB), only their charts were dropped. Also added the "en red desde hace X" counter to the summary card (Store.LastOnlineSince; formatDur extended with days).
6. Perf fix found during validation: UPS chart series were UNCAPPED (thousands of SVG points per tick at 24h) — now downsampled to ChartPoints like every sibling tile (avg for voltages, MAX for outage steps so a short outage never averages away).

### Notes / known constraints carried from design
- FR-021 alerts are an **engine extension** (`evaluateUPSRule` in `engine.go`), NOT config rows (adversarial finding #1).
- Purge is **new wiring** (`PruneUPSSamples` into `startRetentionJob`), not an existing job (finding #2).
- FR-022 (NICE) ships a **static** verdict; the literal count needs a future parameterized-verdict engine change (finding #3) — will be declared as `technical_debt` if the number can't be rendered.
- e2e tests (FR-017/FR-023 cards) require driving the live app with `ULTRON_UPS_MOCK`; runner + framework TBD against the repo's existing e2e setup (checked in Epic 2).
