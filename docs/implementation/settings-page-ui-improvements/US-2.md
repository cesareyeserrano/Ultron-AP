# Implementation Brief: US-2

Feature: settings-page-ui-improvements
Story: As a Administrator, I want protect dangerous actions (shutdown/restart) with a typed confirmation word in a dedicated confirmation field plus a short cancel window with a visible countdown animation, so accidental execution is prevented while keeping Ultron lightweight, so that the workflow remains reliable and traceable.
Trace: FR-2, AC-3

## 1. Feature Context
- I want to completely redesign the Ultron Settings page UI. Right now it looks MVP, generic, and lacks clear UI criteria. It is also unsafe because Raspberry shutdown/restart can happen with an accidental single click. I need a modern UI with strong design criteria, much better usability, and a compact layout, including strong safeguards for dangerous actions.
- Primary actor: Administrator
- Expected outcome: The administrator can configure all settings quickly from a compact modern UI, clearly understand each option, and complete critical actions safely with multi-step confirmation so accidental shutdown/restart cannot happen.

## 2. Acceptance Criteria
- Given an authenticated administrator has typed the required confirmation word for shutdown or restart, when the action is submitted, then a visible countdown cancel window is shown before execution and the action can be canceled during that window.

## 3. Test Cases to Satisfy
- TC-2: Validate us-2 primary behavior. (Trace FR: FR-2)

## 4. Scaffold References
- Interface: src/contracts/fr-2-the-system-must-protect-dangerou.js
- Test stub: tests/settings-page-ui-improvements/generated/tc-2-validate-us-2-primary-behavior.test.mjs

## 5. Dependency Notes
- Order rationale: No previous story dependency
- Plan sequence hint: - 1) Refactor Settings layout and section hierarchy for strict mobile-first rendering. - 2) Standardize component styling/feedback states using existing tokens and classes. - 3) Implement danger-zone confirmation flow (typed word + animated countdown + cancel path). - 4) Add/adjust endpoint checks and audit visibility for dangerous-action rejections. - 5) Add regression tests for UI layout, safeguards, and security rejection paths.
- Plan dependency hint: - Existing templates/CSS (`web/templates/settings.html`, shared partials, `web/static/css/app.css`). - Existing state-changing endpoints and CSRF/same-origin/session middleware. - Existing history/audit logging mechanism for reject evidence.

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
