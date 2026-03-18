# Dev Roadmap: Ultron Monitoring Stabilization

## 1. Implementation Roadmap (Phases)
### Phase 1 — Core logic and interfaces (deployable)
- Deliverables:
  - Stabilized telemetry snapshot model and SSE channel contracts.
  - Status Ribbon view model (system/services/data freshness).
  - Deterministic stale-state and reconnect signaling.
- Deployability:
  - Read-only UI improvements can ship behind existing routes with no infra change.

### Phase 2 — Persistence and integration hardening (deployable)
- Deliverables:
  - Alert persistence consistency checks (SQLite transaction boundaries).
  - Structured observability fields for stream and collector health.
  - Auth/CSRF/origin protections validated for all state-changing paths.
- Deployability:
  - Backward-compatible middleware and persistence improvements.

### Phase 3 — Edge cases and operational hardening (deployable)
- Deliverables:
  - Concurrency safeguards for SSE fanout and collector update path.
  - Degraded-mode UX polish and failure messaging.
  - Security and abuse protections (rate limits, malformed input handling).
- Deployability:
  - Feature-safe hardening with no platform migration.

## 2. Interface Contracts
### Go service contracts (pseudo)
```go
// @aitri-trace US-ID: UNKNOWN FR-ID: UNKNOWN TC-ID: UNKNOWN
type SnapshotProvider interface {
    Current(ctx context.Context) (Snapshot, error)
}

// @aitri-trace US-ID: UNKNOWN FR-ID: UNKNOWN TC-ID: UNKNOWN
type AlertWriter interface {
    PersistAlert(ctx context.Context, in AlertRecord) error
}

// @aitri-trace US-ID: UNKNOWN FR-ID: UNKNOWN TC-ID: UNKNOWN
type StreamPublisher interface {
    PublishMetrics(ctx context.Context, payload MetricsPayload) error
    PublishServices(ctx context.Context, payload ServicesPayload) error
    PublishHistory(ctx context.Context, payload HistoryPayload) error
}
```

### Payload format constraints
- SSE `metrics` payload: `{timestamp, cpu, ram, temp, storage, freshnessSeconds, severity}`.
- SSE `services` payload: `{timestamp, units:[{name,state,severity}]}`.
- SSE `history` payload: bounded recent samples only.
- All payloads require stable keys and explicit null policy.

### Dependency map
- Required modules: `internal/metrics`, `internal/alerts`, `internal/server`, `internal/database`, `internal/auth`.
- External dependencies: SQLite + existing OS-level collectors only.
- Mocking strategy: fake collectors, temp SQLite, in-memory stream sinks.

### Complexity/race focus
- Hot path: collector update -> snapshot mutation -> SSE publish.
- Risks: concurrent snapshot access, slow client backpressure, DB lock bursts.

## 3. Testing Strategy
- Unit:
  - snapshot derivation, stale-state transitions, threshold mapping, validation helpers.
- Integration:
  - SSE/HTMX contract integrity, middleware auth/CSRF behavior, SQLite write/read consistency.
- E2E:
  - dashboard-first operator flow, reconnect and degraded-state scenarios.
- Security tests:
  - CSRF rejection, origin mismatch rejection, malformed input rejection, unauthorized route access.

## 4. Technical Debt Registry
- Debt: oversized handlers/helpers reduce modular clarity.
  - Rationale: historical growth in monolith.
  - Resolution: split by domain (streaming, settings, alerts) with compatibility tests.
- Debt: partial duplication in state/render logic.
  - Rationale: rapid iteration pressure.
  - Resolution: centralize view-model mapping and template helpers.

## 5. Technical Definition of Done
- Code:
  - Interfaces typed, boundaries explicit, no privileged path leakage from web layer.
- Tests:
  - Unit/integration/e2e/security suites pass in CI.
- Security:
  - CSRF/origin/session-cookie controls verified with regression tests.
- Observability:
  - Structured logs and core metrics emitted for stream, collector, and persistence paths.
- Docs:
  - Architecture/security/QA artifacts updated with implementation evidence.
- Traceability:
  - Requirement -> test -> evidence mapping complete.
