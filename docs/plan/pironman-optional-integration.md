# Plan: pironman-optional-integration

STATUS: READY

## Intent
- Reintroduce Pironman control in Ultron as an optional module, disabled by default.
- Keep core panel stable/lightweight on Raspberry Pi 5.
- Preserve security boundary: privileged work only through `ultron-helper`.

## Scope
- In scope:
- Feature flag (`ULTRON_FEATURE_PIRONMAN`) with default `false`.
- Optional module route and isolated UI flow.
- Deterministic apply model (explicit submit + single-flight helper queue).
- Capability states: `available | degraded | unavailable`.
- On-demand diagnostics and per-request telemetry.
- Out of scope:
- Global UI redesign.
- Pironman/Influx service lifecycle management from Ultron core.
- Changes to unrelated modules.

## Architecture
- Web layer:
- Optional routes only when feature flag is enabled.
- CSRF/session checks required for mutations.
- Integration layer:
- Read/apply through `internal/pironman` and helper socket client.
- Degraded/unavailable mapping on timeout/socket/API errors.
- Privileged boundary:
- `ultron-helper` remains single point for privileged actions.
- No direct `sudo` or host lifecycle commands from web handlers.

## Reliability Rules
- Apply executes only on explicit user action.
- At most one active apply operation at a time (helper queue single-flight/coalescing).
- Any timeout/error must return deterministic state and actionable feedback.
- Core panel remains operable even when Pironman is down.

## Security Rules
- Keep `NoNewPrivileges=true` on web service.
- Strict parameter validation before apply.
- CSRF + authenticated session mandatory.
- Audit trail for failures and rejected invalid requests.

## Resource Strategy (Pi5)
- No continuous hardware polling loops.
- Diagnostics are on-demand only.
- Idle targets:
- `ultron-ap` <= 2% CPU
- `ultron-helper` <= 1% CPU

## Delivery Phases
- Phase 1: Design + contracts + tests for optional module behavior.
- Phase 2: Implement feature flag and optional Pironman routes/UI.
- Phase 3: Harden failure mapping, telemetry, and guardrails.
- Phase 4: Pi runtime verification and release gating.

## 4. Product Review (Product Persona)
### Business value
- Reintroduces hardware control without reopening core instability.
- Keeps default operator flow minimal while enabling advanced optional use-cases.

### Success metric
- No stuck-busy regressions.
- Optional module remains disabled by default.
- Core panel remains responsive when Pironman is unavailable.

### Assumptions to validate
- Pironman API endpoint behavior remains stable on target firmware.
- Helper queue logic remains deterministic under burst apply input.
- Optional route exposure does not create measurable idle overhead on Pi5.

## 5. Architecture (Architect Persona)
### Components
- Feature flag configuration (`ULTRON_FEATURE_PIRONMAN`).
- Optional Pironman page and apply endpoint.
- Existing `internal/pironman` adapter and `ultron-helper` boundary.
- Diagnostics endpoint with capability and process snapshot.

### Boundaries
- Web process performs validation + orchestration only.
- Helper performs privileged hardware operations.
- No service lifecycle orchestration from web process.

### Data flow
- Authenticated operator opens optional module.
- Web handler reads capability state via helper read path.
- Operator submits explicit apply payload.
- Web handler validates/sanitizes and forwards through `internal/pironman` -> helper socket.
- Helper executes bounded API operations and returns deterministic outcome.
- Web handler returns applied/failed state and emits audit-friendly logs.

### Key decisions
- Default-off feature flag to protect stable core baseline.
- Keep optional module outside primary sidebar workflow.
- Reuse existing helper queue and timeout controls instead of creating parallel executors.
- Use explicit capability states in UI rather than implicit service probing.

### Risks & mitigations
- Risk: timeout storms when Pironman API is unstable.
- Mitigation: bounded timeouts + degraded mapping + fail-fast UX.
- Risk: accidental privilege regression in web layer.
- Mitigation: no direct privileged exec in handlers; helper-only boundary.
- Risk: resource drift on Pi5.
- Mitigation: on-demand diagnostics only, no hardware polling loop.

### Observability (logs/metrics/tracing)
- Log apply result and failure reason at handler/helper boundaries.
- Expose capability and process snapshot through diagnostics endpoint.
- Keep action outcomes traceable via existing action/audit logging patterns.

## 7. UX/UI Review (UX/UI Persona, if user-facing)
### Primary flow
- Open optional module from Settings.
- Review capability state and config.
- Edit values and press Apply.
- Receive deterministic applied/failed feedback.

### State handling
- `available`: form usable.
- `degraded`: form visible with actionable warning.
- `unavailable`: fail-fast message; no host side effects.
