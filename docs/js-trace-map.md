# Client JS — Aitri trace map

`web/static/js/**` is embedded with `go:embed` and served **verbatim** to browsers.
Anything written in those files, comments included, is readable by anyone who opens
devtools. The Aitri trace ids used to live inline there, which handed the project's
internal requirements map to any visitor — recorded as **BG-080**, and as `RQ-SEC-005`
in `AUDIT_REPORT.md`.

The ids live here instead. This file is documentation: it is **not** under a `go:embed`
glob and never reaches a browser. The explanatory comments stayed in the JS, because
those are what a maintainer actually needs while reading the code; only the bare id
tokens moved.

**Rule for new client JS:** no `FR-`, `BG-`, `AC-` or `TC-` tokens in anything under
`web/static/`. Add the mapping here instead. `scripts/security-gate.sh` enforces this.

| File | Traces | What it implements |
|---|---|---|
| `sidebar.js` | BG-076, BG-038 | Collapsible sidebar. BG-076: it used to capture `#sidebar` and its buttons once at load, so after the first hx-boost swap the handlers pointed at detached nodes and the sidebar stopped resizing — and it only behaved from the second navigation on. BG-038 is the shared lifecycle hazard: re-bind on `htmx:afterSettle` and `htmx:historyRestore`, never once at load. |
| `settings.js` | FR-065, BG-038 | Settings form orchestration: form-state pill (FR-065 — rendered only when state is not idle), inline field errors, retry, and the destructive-action confirm. Loaded from `<head>` on every page for the BG-038 reason. |
| `widgets/toggle.js` | FR-061, BG-038 | Toggle switch wrapping a hidden checkbox. |
| `widgets/stepper.js` | FR-057, FR-060, BG-038 | Unit-aware numeric stepper. FR-060 is the hint string shared with the label. |
| `widgets/segmented.js` | FR-058, BG-038 | Three-button segmented severity control. |
| `widgets/chip-preset.js` | FR-059, BG-038 | Chip-preset row with a custom escape hatch. |
| `widgets/anchor-chip.js` | FR-063, BG-038 | Anchor-chip strip: click scrolls to and expands the target section. |
| `widgets/encryption-key.js` | FR-068, BG-038 | Encryption-key composite picker; validates on blur of the value input. |
| `widgets/service-logs.js` | FR-081, BG-038 | Per-service log drawer; htmx does the fetch via `hx-get`. |

## The BG-038 family

Every widget above carries the same guard, and it is worth stating once rather than
nine times: htmx history restores re-insert **cached DOM** without firing `afterSwap`,
so a widget bound only on swap is dead after a back-navigation. The fix is to bind on
`htmx:afterSettle` **and** `htmx:historyRestore`, with a per-node guard flag so
re-initialisation stays idempotent.
