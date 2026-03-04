# AF-SPEC: normalization

STATUS: APPROVED
## 1. Context
Take the existing Ultron codebase as baseline and implement the recently documented stabilization and UX/security/QA improvements on top of it, without replatforming.

Primary actor: Raspberry Pi operator/admin.
Expected outcome: Operator can identify critical system state quickly, monitor live metrics reliably, navigate to corrective modules in at most two interactions, and keep secure/low-resource operation.
In scope: Included: dashboard status hierarchy improvements, live SSE reliability and stale-data handling, consistent UX patterns, security hardening (auth/session/CSRF/origin checks), observability fields, and QA/test traceability based on existing documentation.
Out of scope: Excluded: stack replatforming, infrastructure migration, and unrelated new product modules outside monitoring stabilization.
Technology: Keep existing stack: Go + HTMX + SSE + SQLite + Tailwind.
Requirement source: Provided explicitly by user in guided draft.

## 2. Actors
- Raspberry Pi operator/admin.

## 3. Functional Rules (traceable)
- FR-1: The system must present critical monitoring status clearly at first glance and keep live dashboard updates reliable with explicit degraded-state signaling.
- FR-2: The system must preserve strict security boundaries: authenticated access, CSRF/origin protection for state-changing routes, and no privileged execution from web handlers.

## 4. Edge Cases
- Live data can be delayed or unavailable; UI must show stale-data state with last-update timestamp and recovery path without layout breakage.

## 5. Failure Conditions
- FC-1: Critical monitoring status is not visible in the first viewport for authenticated operator sessions.
- FC-2: Live telemetry degradation is not signaled (missing stale-data timestamp/recovery state), causing ambiguous monitoring state.
- FC-3: Any state-changing route accepts requests without valid auth/CSRF/origin controls.

## 6. Non-Functional Requirements
- NFR-1: Preserve low resource usage on Raspberry Pi by keeping existing stack and avoiding heavy frontend dependencies.
- NFR-2: Maintain near-real-time perceived monitoring updates with explicit degraded-state behavior when data freshness drops.
- NFR-3: Preserve operational safety boundaries (no privileged execution path from web handlers).

## 7. Security Considerations
- Enforce whitelist-first input validation plus rate limits on auth and high-cost endpoints (including SSE reconnect abuse), and audit-log all rejects.

## 8. Out of Scope
- Excluded: stack replatforming, infrastructure migration, and unrelated new product modules outside monitoring stabilization.

## 9. Acceptance Criteria
- AC-1: Given an authenticated operator on Dashboard, when live telemetry is healthy, then critical status is visible in first viewport and corrective navigation is reachable in two clicks or fewer.
- AC-2: Given a state-changing endpoint request, when auth session is invalid or CSRF/origin validation fails, then the request is rejected and an auditable security event is logged.

## 10. Requirement Source Statement
- All requirements in this draft were provided explicitly by the user.
- Aitri structured the content and did not invent requirements.

## 11. Resource Strategy
- No external assets required; use existing repository assets/components.

## Pre-Planning Context (from dev-roadmap)
<!-- Auto-injected by aitri draft — do not edit this section manually -->
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
###
<!-- End pre-planning context -->
