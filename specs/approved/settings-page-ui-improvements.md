# AF-SPEC: settings-page-ui-improvements

STATUS: APPROVED
## 1. Context
I want to completely redesign the Ultron Settings page UI. Right now it looks MVP, generic, and lacks clear UI criteria. It is also unsafe because Raspberry shutdown/restart can happen with an accidental single click. I need a modern UI with strong design criteria, much better usability, and a compact layout, including strong safeguards for dangerous actions.

Primary actor: Administrator
Expected outcome: The administrator can configure all settings quickly from a compact modern UI, clearly understand each option, and complete critical actions safely with multi-step confirmation so accidental shutdown/restart cannot happen.
In scope: Complete Settings page UI redesign; compact information architecture; modern visual system (typography, spacing, hierarchy, states); clear grouping of settings sections; improved form controls and inline help; safer dangerous actions with explicit warning states and multi-step confirmations for shutdown/restart; accessibility and responsive behavior with a strict mobile-first approach.
Out of scope: No backend replatforming, no changes to unrelated pages, no new monitoring modules, and no removal of existing security controls.
Technology: No constraint — architect will propose the stack.
Requirement source: Provided explicitly by user in guided draft.

## 2. Actors
- Administrator

## 3. Functional Rules (traceable)
- FR-1: The system must provide a complete, mobile-first Settings UI where administrators can find, understand, and update configuration quickly through a compact, clearly structured interface with consistent components and feedback states.
- FR-2: The system must protect dangerous actions (shutdown/restart) with a typed confirmation word in a dedicated confirmation field plus a short cancel window with a visible countdown animation, so accidental execution is prevented while keeping Ultron lightweight.
- FR-3: The system must use existing Ultron design tokens and components by default, and any external UI asset is allowed only if it remains lightweight and does not degrade Raspberry Pi performance.

## 4. Edge Cases
- On small mobile screens, long setting labels, helper text, and validation errors can overlap or push critical controls off-screen, causing accidental taps or missed confirmations.

## 5. Failure Conditions
- FC-1: On mobile viewport, settings sections overlap, break hierarchy, or hide critical controls.
- FC-2: Shutdown or restart can be executed with an accidental single click without typed confirmation plus cancel window.
- FC-3: State-changing settings requests are accepted without valid session, CSRF, or same-origin validation.

## 6. Non-Functional Requirements
- NFR-1: The Settings page must follow a strict mobile-first layout and remain readable and usable on small screens.
- NFR-2: The UI and interaction model must stay compact and modern with consistent visual hierarchy and feedback states.
- NFR-3: The implementation must keep Ultron lightweight on Raspberry Pi and avoid performance regression from UI changes.

## 7. Security Considerations
- All state-changing settings actions must require a valid authenticated session, CSRF protection, and same-origin validation, and all rejected dangerous-action attempts must be audit-logged.

## 8. Out of Scope
- No backend replatforming, no changes to unrelated pages, no new monitoring modules, and no removal of existing security controls.

## 9. Acceptance Criteria
- AC-1: Given an authenticated administrator on the Settings page on a mobile viewport, when the page loads, then the layout is compact, readable, and all primary settings sections are reachable without UI overlap or broken hierarchy.
- AC-2: Given an authenticated administrator initiates shutdown or restart from Settings, when they have not typed the required confirmation word, then the action is blocked and no state change is executed.
- AC-3: Given an authenticated administrator has typed the required confirmation word for shutdown or restart, when the action is submitted, then a visible countdown cancel window is shown before execution and the action can be canceled during that window.

## 10. Requirement Source Statement
- All requirements in this draft were provided explicitly by the user.
- Aitri structured the content and did not invent requirements.

## 11. Resource Strategy
- External assets are allowed only if they keep Ultron lightweight; if that conflicts with performance or footprint, use only existing project assets and components.

## UX Context (from ux-design)
<!-- Auto-injected by aitri draft — do not edit manually -->
# UX Design: Ultron Monitoring Stabilization

## 1. Hero Flow
1. Operator opens Dashboard.
2. Above-the-fold summary shows CPU, RAM, temperature, storage, and service health with clear severity colors.
3. Operator sees a global alert chip in header with count and severity.
4. If healthy, user exits in <10 seconds.
5. If alert exists, click alert chip -> Alerts view filtered to active critical/warning.
6. Operator reviews issue card, opens related module (Docker/Services/Logs) in one click.
7. Action outcome appears as explicit success/error state with retry path.

## 2. Component Innovation
- `Status Ribbon`: compact top ribbon with 3 zones (System, Services, Data Freshness).
- Each zone exposes one primary state and one confidence hint (e.g., `Data age: 4s`).
- Improves mental model by separating "system health" from "data quality".

## 3. State Matrix
- Dashboard:
  - Ideal: live metrics update smoothly, no layout shift.
  - Pre-emptive: skeleton placeholders and "connecting" badge.
  - Failure/Recovery: stale-data banner with last update timestamp + reconnect action.
- Alerts:
  - Ideal: grouped by severity with clear timestamps.
  - Pre-emptive: loading group placeholders.
  - 
<!-- End UX context -->

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
