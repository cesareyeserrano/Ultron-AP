## Feature
Fix the sustained-alert confirmation window so that "metric breaching for >= duration" rules fire reliably under real-world clock jitter and non-aligned sampling.

## Problem / Why
The sustained-window logic in `internal/alerts/engine.go` (`sustainedWindow.add`) confirms a sustained breach only when the oldest retained sample's timestamp equals the cutoff (`at - duration`) exactly. After the trim loop removes samples strictly `Before(cutoff)`, every retained sample satisfies `at >= cutoff`, so the final check `!w.samples[0].at.After(cutoff)` is true only in the knife-edge case `samples[0].at == cutoff`. With any sampling jitter the confirming call lands slightly after `t0 + duration`, the first breaching sample is trimmed, and the window never confirms. The result: sustained rules (e.g. "latency > X for 60s") silently never fire in production. The existing test (TC-NA-076e) passes only because its samples are perfectly interval-aligned.

## Target Users
Existing operators/admins who configure sustained ("for N seconds/minutes") alert rules and rely on them firing. No new user type.

## New Behavior
- The system must confirm a sustained breach when the breach has been continuous for at least the configured duration, measured as the span between the earliest retained breaching sample and the current sample (`at - samples[0].at >= duration`), tolerant of clock jitter and non-aligned sampling.
- The system must continue to reset the window when a non-breaching sample arrives or when a sampling gap exceeds the existing threshold (`> 2*interval`).
- The system must keep memory bounded (not retain unbounded samples) while preserving the earliest sample needed to evaluate the duration span.

## Success Criteria
- Given a rule with duration D and breaching samples arriving with small jitter, When the elapsed breach span reaches >= D, Then the window confirms (returns true).
- Given breaching samples whose total span is < D, When add() is called, Then the window does not confirm.
- Given a non-breaching sample or a gap > 2*interval, When add() is called, Then the window resets and does not confirm until a fresh breach spans D again.
- Existing TC-NA-076e and all current alert tests continue to pass.

## Touch Points
MODIFIES:
- `internal/alerts/engine.go` — `sustainedWindow.add` (and `sustainedWindow` struct if needed for retention).
- Alert engine test cases covering sustained windows (e.g. TC-NA-076e and siblings) — add jitter/non-aligned coverage.
Relates to the network-alerts feature's sustained-alert FRs.

## Must Not Break (Regression Boundary)
- Non-breaching sample still resets the window and returns false (current behavior).
- Sampling gap > 2*interval still resets the window (current behavior).
- A zero/negative duration still short-circuits to returning the raw breaching flag (current `w.duration <= 0` path).
- TC-NA-076e (interval-aligned sustained confirmation) keeps passing.
- No change to alert cooldown, dedup, or dispatch behavior outside the sustained window.

## Out of Scope
- Reworking the sampling cadence or interval configuration.
- Changing how breaches are detected (threshold evaluation) — only the sustained-duration confirmation gate.
- Adding new alert rule types or new notification channels.
