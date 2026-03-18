# QA Plan: Ultron Monitoring Stabilization

## 1. Test Suite Architecture
- Unit tests:
  - Scope: collector transforms, alert rule evaluation, stale-state logic, validation helpers.
  - Tooling: `go test` (table-driven), lightweight mocks/fakes.
- Integration/Contract tests:
  - Scope: SSE channel payload shape, HTMX endpoint behavior, SQLite persistence boundaries, auth/CSRF middleware.
  - Tooling: Go HTTP test server + SQLite temp DB + golden payload assertions.
- E2E tests:
  - Scope: operator flow Dashboard -> Alerts -> Logs/Services, reconnection and degraded-state behavior.
  - Tooling: Playwright (or equivalent) with deterministic fixtures.

## 2. Breaking Point Analysis
- Concurrent SSE clients:
  - Break condition: render/emit latency exceeds acceptable update window.
  - Expected degradation: stale badge + continued manual refresh functionality.
- Collector lag/failure bursts:
  - Break condition: repeated collector timeout or unavailable source.
  - Expected degradation: last-known-good values with explicit freshness timestamp.
- SQLite contention:
  - Break condition: persistent lock contention beyond bounded retry window.
  - Expected degradation: keep read paths alive, queue/drop non-critical writes with alert logs.

## 3. Security and Vulnerability Audit
- Threat A (session/CSRF misuse):
  - Test: missing/invalid CSRF token on state-changing routes -> 4xx + audit log.
  - Test: invalid Origin/Referer on protected routes -> rejected.
- Threat B (SSE/HTMX abuse):
  - Test: rapid reconnect flood from same IP/session -> capped connections and rate-limit responses.
  - Test: malformed query/path input -> rejected by validation, no panic.
- Threat C (privileged-boundary bypass):
  - Test: malformed helper payloads -> rejected by strict allowlist.
  - Test: verify no direct privileged command path from web handlers.

## 4. Data Validation Rules
- Request/response constraints:
  - Required fields validated for type and range.
  - Enumerations enforced for status and severity fields.
  - Null/empty handling explicit for optional telemetry.
- Edge and abuse cases:
  - Min/max values for CPU/RAM/temperature and timestamps.
  - Empty dataset rendering (no crashes, deterministic fallback UI).
  - Injection attempts in query/body/path rejected with non-2xx responses.
- Contract integrity:
  - SSE payload must keep stable keys and value types.
  - HTMX endpoints must return expected fragments/status codes.

## 5. Quality Gate Status
- Ready for Dev: GO (conditional)
  - Criteria:
    - Unit + integration test plan complete with explicit assertions.
    - Security must-have tests enumerated and mapped to threats.
    - No unresolved ambiguity in core behavior expectations.
- Ready for Prod: NO-GO until validated
  - Exit criteria:
    - All critical/security tests passing.
    - No unresolved high/critical security risks.
    - SSE degradation and stale-state scenarios verified.
    - Traceability complete: requirements -> test cases -> evidence.
