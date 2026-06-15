# 02 — System Architecture: sustained-alert-window-fix

## Executive Summary

This feature is a localized correctness fix to the in-memory sustained-alert
confirmation window in `internal/alerts/engine.go` (`sustainedWindow`). No new
service, dependency, schema, endpoint, or UI is introduced — the change is
confined to one struct and one method (`sustainedWindow.add`) plus its tests.

The current implementation confirms a sustained breach only when the oldest
*retained* sample's timestamp equals the cutoff (`at - duration`) exactly. The
trim loop discards samples strictly `Before(cutoff)`, so every retained sample
already satisfies `at >= cutoff`; the final predicate
`!samples[0].at.After(cutoff)` is therefore true only in the knife-edge case
`samples[0].at == cutoff`. Under any sampling jitter the first breaching sample
is trimmed and the window never confirms — silently disabling sustained
alerting in production. The existing test TC-NA-076e passes only because its
samples are perfectly interval-aligned.

The fix replaces the cutoff-equality predicate with a **span-based**
confirmation: a breach is confirmed once the elapsed time between the *first*
breaching sample of the current run and the current sample reaches the
configured duration (`current.at - firstAt >= duration`). The window is reduced
to an O(1) state machine (breach-run start + last-sample time), which fixes the
bug (FR-016) and makes bounded memory (FR-017) trivial. This mirrors the
span-based check already used by the sibling helpers
`sustainedLossValue`/`sustainedLatencyValue` (engine.go:808), keeping the
package internally consistent.

## System Architecture

Unchanged at the system level. The alert engine remains a single in-process
goroutine that, per evaluation tick, computes a boolean `breaching` for each
rule and passes it through the per-rule `sustainedWindow` to decide whether the
breach has persisted long enough to fire. The only component touched:

```
Engine.evaluate (engine.go:~358-361)
   └─ per-rule sustainedWindow.add(ruleID, now, breaching)  ← THIS METHOD
         returns: confirmed bool  → feeds existing cooldown/dedup/dispatch
```

Call site (engine.go:358-361) and the downstream cooldown → dedup → dispatch
pipeline are **unchanged**. The `add` signature `(ruleID int64, at time.Time,
breaching bool) bool` is preserved, so no caller changes.

### ADRs

**ADR-001 — Confirmation predicate: span-based vs cutoff-equality (corrected)**
- Option A (chosen): confirm when `current.at - firstAt >= duration`, where
  `firstAt` is the timestamp of the first breaching sample in the current
  continuous run. Jitter-tolerant; matches the sibling helpers' approach
  (engine.go:808). Confirms within one interval of the true span reaching D.
- Option B (rejected): keep the slice + cutoff but fix the predicate to
  `samples[0].at <= cutoff` by *not* trimming the earliest sample. Works, but
  forces unbounded slice growth during a long breach (conflicts with FR-017) or
  a more complex "keep one sample before cutoff" trim.
- Decision: A. Simpler, correct, bounded, consistent with existing code.

**ADR-002 — Window state representation: O(1) fields vs sample slice**
- Option A (chosen): replace `samples []sustainedSample` with three scalar
  fields — `active bool`, `firstAt time.Time` (breach-run start), `lastAt
  time.Time` (most recent sample, used for gap detection). O(1) memory,
  satisfies FR-017 by construction.
- Option B (rejected): keep the slice and cap its length. Retains needless
  per-sample allocation and the risk of trimming the run-start sample.
- Decision: A. The slice's only consumer is `add` itself; no external code or
  test inspects `samples` (unexported field, verified by grep). Safe to replace.
- Blast radius: a logic error here either fires sustained alerts early (false
  positive) or never (false negative). Contained to alert *timing*; cannot
  corrupt data, crash the process, or affect non-sustained rules.

**ADR-003 — Confirmation boundary uses `>= duration` (not `duration - interval`)**
- The FR specifies `span >= duration`. The sibling `sustainedLossValue` uses
  `< duration - interval` as its "insufficient span" guard (one interval of
  slack). We deliberately choose the stricter `>= duration` for `add` because
  `add` is called once per tick with the live `now`, so the span is measured
  against real elapsed time, not a pre-fetched sample buffer; one interval of
  slack is not needed and would risk early firing. TC-NA-076e (confirming
  sample exactly D after first) still passes since `span == D` satisfies `>= D`.

## Data Model

No persistent data model change. SQLite schema, alert tables, and config rows
are untouched. The only "data" change is the in-memory `sustainedWindow` struct:

```
type sustainedWindow struct {
    duration time.Duration   // unchanged — configured sustain duration D
    interval time.Duration   // unchanged — engine sampling interval
    active   bool            // NEW — is a breach run currently open?
    firstAt  time.Time       // NEW — timestamp of the first breaching sample in the run
    lastAt   time.Time       // NEW — timestamp of the most recent sample (gap detection)
}
```

`sustainedSample` becomes unused and is removed (dead-code cleanup).

## API Design

No external API. The internal contract is the `add` method, signature preserved:

```
func (w *sustainedWindow) add(ruleID int64, at time.Time, breaching bool) bool
```

Behavioral contract (the corrected state machine):
1. `duration <= 0` → return `breaching` verbatim; do not mutate state. (NFR-007)
2. If a run is active and `at - lastAt > 2*interval` → reset the run (log
   `reason=sample_gap`) before evaluating the current sample. (NFR-006)
3. `breaching == false` → reset the run, return false. (NFR-005)
4. `breaching == true`:
   - if no run active → start one (`active=true`, `firstAt=at`).
   - always set `lastAt=at`.
   - return `at - firstAt >= duration`. (FR-016; single sample ⇒ span 0 ⇒ false for D>0)

Memory is O(1) per rule regardless of breach length. (FR-017)

## Security Design

Not applicable beyond no-regression. `sustainedWindow.add` is pure in-process
logic over a `time.Time` and a `bool`; it reads no user input, performs no I/O,
no network call, no command execution, no query, and handles no secrets. The
change introduces no new attack surface (NFR-011). The `ruleID` is used only in
a log line, as today.

## Performance & Scalability

Strictly improved. The change removes per-tick slice append/copy/trim
allocations in favor of three scalar field assignments. Per-rule memory drops
from O(samples-in-window) to O(1). No change to evaluation cadence or the number
of rules evaluated per tick. On the target ARM64 Pi this is negligible CPU and a
small reduction in allocation churn for rules in a long sustained breach.

## Deployment Architecture

Unchanged. Ships in the same single statically-linked Go binary via the existing
`make build-arm` → systemd deployment path. No migration, no config change, no
restart procedure change. The fix takes effect on the next binary deploy; no
operator action required.

## Risk Analysis

- **Behavioral change to alert timing (intended):** rules that silently never
  fired will now fire when a breach truly persists. Operators may see sustained
  alerts that were previously (incorrectly) suppressed. This is the fix working
  as intended; documented so it is not mistaken for a regression. Mitigation:
  Phase-3 tests assert both the positive (fires at span >= D) and the negative
  (does not fire at span < D) so behavior is pinned.
- **Removing `samples` slice could break a test that inspects internals:**
  mitigated — grep confirms `samples`/`sustainedSample` are referenced only
  within `add` and (potentially) `_test.go`; tests call `add` and assert the
  return value, not internal fields. Phase 3/4 will adjust any test constructor
  that set `samples` directly.
- **Boundary semantics (`>= D` vs `>= D - interval`):** chosen `>= D` per
  ADR-003; risk of one-interval-late confirmation is acceptable and preferable
  to early firing. Pinned by TC-NA-076e and a new jitter test.

## Technical Risk Flags

None detected. Pure-logic, single-file, dependency-free change on a fully
compatible stack (Go stdlib `time`); no schema, network, secret, or platform
risk. The only behavioral risk (alerts that now correctly fire) is the intended
outcome and is covered by Phase-3 tests.
