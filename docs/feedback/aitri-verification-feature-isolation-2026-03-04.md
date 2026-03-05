# Aitri Feedback - Verify does not enforce feature-isolated execution

Date: 2026-03-04
Project: Ultron
Affected features: `settings-page-ui-improvements`, `normalization`

## Summary

`aitri verify --feature settings-page-ui-improvements` reports `VERIFICATION PASSED`, but actually executes only one test file from a different feature (`normalization`), while still reporting full TC/US/FR coverage for `settings-page-ui-improvements`.

This creates a false positive where unimplemented or regressed US can pass the gate.

## Evidence

Command:

```bash
aitri verify --feature settings-page-ui-improvements
```

Observed output:

- Feature: `settings-page-ui-improvements`
- Command executed: `node --test tests/normalization/generated/tc-1-validate-us-1-primary-behavior.test.mjs`
- Evidence file: `docs/verification/settings-page-ui-improvements.json`
- Reported coverage in evidence: declared TC=5, passing=5, US-1/US-2/US-3 fully verified

The executed command and reported coverage contradict each other.

## Reproduction Steps

1. Have at least two approved features with generated tests (`normalization` and `settings-page-ui-improvements`).
2. Run:
   - `aitri verify --feature normalization`
   - `aitri verify --feature settings-page-ui-improvements`
3. Observe that both executions run the same normalization test command.
4. Open:
   - `docs/verification/normalization.json`
   - `docs/verification/settings-page-ui-improvements.json`
5. Confirm command mismatch vs feature-specific coverage report.

## Expected Behavior

- `verify --feature <X>` must execute tests only under `tests/<X>/...` (or a feature-scoped manifest).
- Coverage must be computed from executed artifacts for that feature only.
- If executed tests do not map to selected feature, verification must fail hard.

## Impact

- Delivery gate can pass without validating all US in selected feature.
- Teams may deploy partially implemented UI/UX while `deliver` shows green.
- Traceability and trust in FR/US/TC compliance is reduced.

## How it was unblocked in this session

1. Manual verification was run directly:
   - `node --test tests/settings-page-ui-improvements/generated/*.test.mjs` (5/5 passing)
   - `go test ./internal/server` (passing)
2. Deployment confidence was based on manual test evidence and runtime checks, not on Aitri verify output alone.

## Recommended Fixes for Aitri

1. Enforce feature-scoped test command resolution in `verify`.
2. Block pass state when executed command path does not include selected feature slug.
3. Bind `tcCoverage`/`usCoverage` to executed test result set, not static manifests alone.
4. Add a consistency check:
   - if `reported feature != executed test namespace` then fail with explicit error.
5. Add integration test in Aitri:
   - multi-feature repo
   - ensure `verify --feature A` never executes test files from `B`.
