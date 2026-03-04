# Implementation Brief: US-2

Feature: normalization
Story: As a Raspberry Pi operator/admin.
Trace: FR-2, AC-2

## 1. Feature Context
- Take the existing Ultron codebase as baseline and implement the recently documented stabilization and UX/security/QA improvements on top of it, without replatforming.
- Primary actor: Raspberry Pi operator/admin.
- Expected outcome: Operator can identify critical system state quickly, monitor live metrics reliably, navigate to corrective modules in at most two interactions, and keep secure/low-resource operation.

## 2. Acceptance Criteria
- Given a state-changing endpoint request, when auth session is invalid or CSRF/origin validation fails, then the request is rejected and an auditable security event is logged.

## 3. Test Cases to Satisfy
- TC-2: Validate us-2 primary behavior. (Trace FR: FR-2)

## 4. Scaffold References
- Interface: src/contracts/fr-2-the-system-must-preserve-strict-.js
- Test stub: tests/normalization/generated/tc-2-validate-us-2-primary-behavior.test.mjs

## 5. Dependency Notes
- Order rationale: Implement after US-1
- Plan sequence hint: - 1) Monitoring clarity + stale-state UX. 2) Security middleware hardening. 3) Observability and test coverage hardening.
- Plan dependency hint: - Existing collector modules, alert engine, auth/session middleware, SQLite persistence.

## 6. Quality Constraints
- Domain profile: Not specified
- Stack constraint: Not specified
- Forbidden defaults: Not specified
- Non-negotiable: keep FR traceability comments in interfaces and TC markers in tests.


## Architecture Decision Context
<!-- From .aitri/architecture-decision.md -->
# Architecture Decision: Ultron Monitoring Stabilization

## 1. Architecture Overview
- System boundary: single-node Raspberry Pi deployment running Ultron Go service, SQLite storage, and server-rendered HTMX UI.
- Components:
  - Web App (Go + HTMX endpoints + SSE broadcaster)
  - Collector/Monitors (metrics, docker, systemd adapters)
  - Alert Engine (rule evaluation and persistence)
  - Data Store (SQLite)
  - Privileged Helper boundary (isolated local control path; not exposed to web layer for this scope)
- Data flows:
  - Collector -> in-memory snapshot -> SSE channels (metrics/services/history cadences)
  - Snapshot + events -> Alert Engine -> SQLite alerts/history
  - Browser HTMX/SSE <- Web App templates + stream endpoints

## 2. C4 Level 2 Diagram (Mermaid)
```mermaid
flowchart LR

## Security Requirements Context
<!-- From .aitri/security-review.md -->
# Security Review: Ultron Monitoring Stabilization

## 1. Threat Profile
- Scenario A: Session hijack/CSRF on state-changing endpoints.
  - Goal: execute unauthorized actions as authenticated operator.
  - Likelihood: Medium.
  - Impact: High.
- Scenario B: SSE/HTMX endpoint abuse (high-frequency calls, connection flooding).
  - Goal: degrade service availability on low-resource Raspberry Pi.
  - 
