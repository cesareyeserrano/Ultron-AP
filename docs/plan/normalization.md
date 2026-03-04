# Plan: normalization

STATUS: DRAFT

## 1. Intent (from approved spec)
- Summary:
- Normalize the existing Ultron baseline and implement documented monitoring, UX, security, and QA improvements without replatforming.
- Success looks like:
- Critical status is visible immediately, degraded telemetry is explicit/recoverable, and state-changing endpoints remain protected by strict controls.

## 2. Discovery Review (Discovery Persona)
### Problem framing
- Operators lose time when degraded telemetry state is ambiguous and critical status is not immediately clear.

### Constraints and dependencies
- Keep existing stack (Go + HTMX + SSE + SQLite + Tailwind), low Raspberry Pi resource usage, strict least-privilege boundary.
- Dependencies: existing collectors, alert engine, SQLite, auth middleware, local privileged helper boundary.

### Success metrics
- Critical state visible in first viewport.
- Corrective navigation in <=2 interactions.
- Stale-data state with timestamp/recovery shown when telemetry degrades.

### Key assumptions
- Existing Ultron modules remain the implementation baseline.
- No infrastructure migration is required for this feature.

## 3. Scope
### In scope
- Dashboard monitoring clarity and first-viewport status hierarchy.
- SSE reliability with explicit stale-data and recovery states.
- Security hardening for auth/session/CSRF/origin + boundary validation.
- Observability and test traceability improvements.

### Out of scope
- Replatforming stack/infrastructure.
- New unrelated modules beyond monitoring stabilization.
- Heavy new frontend dependencies.

## 4. Product Review (Product Persona)
### Business value
- Faster operator diagnosis and safer day-to-day operation on Raspberry Pi.
- Reduced ambiguity during degraded telemetry periods.

### Success metric
- Primary KPI: critical status discoverability + <=2-step corrective navigation.
- Security KPI: invalid/forged state-changing requests are rejected and logged.

### Assumptions to validate
- UI improvements do not regress performance on low-resource hardware.
- Security controls are consistently applied across all state-changing endpoints.

## 5. Architecture (Architect Persona)
### Components
- Web app (Go + HTMX + SSE)
- Collector adapters (metrics/docker/systemd)
- Alert engine
- SQLite store
- Local privileged helper boundary (not exposed from web handlers)

### Data flow
- Collectors update snapshot -> SSE publishes channel updates.
- Snapshot/events feed alert evaluation -> persisted to SQLite.
- Browser consumes HTMX fragments and SSE updates.

### Key decisions
- Keep modular monolith on Raspberry Pi.
- Keep SSE transport with cadence tuning.
- Keep SQLite transactional persistence for alerts/history.
- Preserve strict least-privilege boundary.

### Risks & mitigations
- SSE fanout latency under concurrent clients -> bounded cadence + compact payloads.
- Collector outages -> stale-data + last-known-good fallback.
- SQLite contention -> bounded retries and controlled degradation.

### Observability (logs/metrics/tracing)
- Structured logs with request id/channel/latency/failure reason.
- Metrics for stream latency, client count, collector cycle, stale duration.
- Correlated request ids for traceability.

## 6. Security (Security Persona)
### Threats
- Session/CSRF misuse on state-changing endpoints.
- SSE/HTMX abuse causing resource pressure.
- Privileged-boundary bypass via malformed input.

### Required controls
- Authenticated access for protected routes.
- CSRF token + Origin/Referer checks.
- Secure session cookies (HttpOnly/Secure/SameSite).
- No direct privileged execution from web handlers.

### Validation rules
- Whitelist-first input validation with explicit ranges/enums.
- Reject malformed request payloads with auditable logs.
- Preserve stable, non-sensitive output schemas in HTMX/SSE responses.

### Abuse prevention / rate limiting (if applicable)
- Per-session/IP limits for auth/high-cost endpoints.
- SSE connection cap and reconnect throttling.

## 7. UX/UI Review (UX/UI Persona, if user-facing)
### Primary user flow
- Dashboard first-view triage -> alert/status recognition -> corrective module navigation in <=2 interactions.

### Key states (empty/loading/error/success)
- Loading: skeleton/connecting state.
- Empty: clear no-data semantics.
- Error/degraded: stale-data banner with last-update timestamp and recovery path.
- Success: stable live updates without layout jumps.

### Accessibility baseline
- WCAG AA contrast in dark theme.
- Keyboard navigability for core flows.
- 44x44 touch targets and persistent focus states.

## 8. Backlog
> Create as many epics/stories as needed. Do not impose artificial limits.

### Epics
- Epic 1:
  - Outcome: Reliable and clear live monitoring UX on existing Ultron baseline.
  - Notes: Prioritize first-viewport status clarity and degraded-state signaling.
- Epic 2:
  - Outcome: Security and operational hardening with auditable controls.
  - Notes: Prioritize auth/session/CSRF/origin and boundary validation.

### User Stories
For each story include clear Acceptance Criteria (Given/When/Then).

#### Story:
- As a Raspberry Pi operator/admin, I want critical status visible at first glance, so that I can triage quickly.
- Acceptance Criteria:
  - Given an authenticated dashboard session, when telemetry is healthy, then critical status is visible in first viewport.
  - Given an authenticated session, when navigating to corrective module, then target module is reachable in two interactions or fewer.

#### Story:
- As a Raspberry Pi operator/admin, I want clear degraded-state signaling, so that I can distinguish stale data from healthy state.
- Acceptance Criteria:
  - Given telemetry delay/unavailability, when dashboard renders, then stale-data state with last-update timestamp is shown.
  - Given telemetry recovery, when stream resumes, then UI returns to healthy state without layout breakage.

#### Story:
- As a maintainer, I want protected state-changing routes, so that forged or unauthorized requests are blocked.
- Acceptance Criteria:
  - Given invalid session or CSRF/origin failure, when request reaches protected route, then request is rejected and logged.
  - Given malformed boundary inputs, when validation runs, then payload is denied without privileged execution.

## 9. Test Cases (QA Persona)
> Create as many test cases as needed. Include negative and edge cases.

### Functional
1. Validate first-viewport critical status discoverability for authenticated sessions.
2. Validate <=2 interaction navigation to corrective modules.

### Negative / Abuse
1. Reject malformed/invalid request payloads without panic.
2. Enforce reconnect/auth endpoint rate limits under repeated attempts.

### Security
1. Validate CSRF/origin/session controls on all state-changing routes.
2. Validate absence of direct privileged execution path from web handlers.

### Edge cases
1. Degraded telemetry shows stale state and recovery path correctly.
2. SQLite lock contention degrades gracefully while preserving read behavior.

## 10. Implementation Notes (Developer Persona)
- Suggested sequence:
- 1) Monitoring clarity + stale-state UX. 2) Security middleware hardening. 3) Observability and test coverage hardening.
- Dependencies:
- Existing collector modules, alert engine, auth/session middleware, SQLite persistence.
- Rollout / fallback:
- Incremental module rollout with fallback to previous rendering/state behavior on regression.
