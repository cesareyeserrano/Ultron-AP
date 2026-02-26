# Discovery: ux-ui-upgrade

STATUS: DRAFT

## 1. Problem Statement
Derived from approved spec retrieval snapshot:
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria

### Context snapshot
- A complete UX/UI upgrade for Ultron: modern and premium dashboard experience across desktop and mobile.
- Primary actor: Primary user: system admin/operator managing Raspberry Pi services from Ultron.
- Expected outcome: Admins can monitor key telemetry quickly, understand service health instantly, and execute actions confidently with a clear, polished, premium interface on desktop and mobile.

### Actors snapshot
- Primary user: system admin/operator managing Raspberry Pi services from Ultron.

### Functional rules snapshot
- The system must define and apply a dashboard visual asset strategy that uses existing local iconography and CSS-driven primitives first, and introduces new branded assets only when required for readability or hierarchy.

### Security snapshot
- UI changes must preserve all existing CSRF/authentication flows and must not expose sensitive configuration values in visible states.

### Out-of-scope snapshot
- No backend business-logic changes, no infrastructure changes, no monitoring engine rewrites; this feature is UI/UX and presentation-focused.

Refined problem framing:
- What problem are we solving? Current UI is functional but visually flat; chart readability and hierarchy are weak, settings are dense on mobile, and premium product perception is low. This affects daily scanning speed and operator confidence.
- Why now? Baseline: functional but low premium perception and slower visual scan. Target: clear hierarchy across core pages, improved chart legibility, no horizontal overflow on mobile, and consistent component language validated in E2E visual review.

## 2. Discovery Interview Summary (Discovery Persona)
- Primary users:
- System admins/operators managing Raspberry Pi services via Ultron web dashboard.

- Jobs to be done:
- Monitor telemetry quickly, detect service/container issues, manage operational actions confidently, and configure notifications/backup/settings with low cognitive load on desktop and mobile.

- Current pain:
- Current UI is functional but visually flat; chart readability and hierarchy are weak, settings are dense on mobile, and premium product perception is low. This affects daily scanning speed and operator confidence.

- Constraints (business/technical/compliance):
- No backend logic changes; keep Go templates + HTMX + Tailwind + vanilla JS; preserve CSRF/auth flows; maintain performance on Raspberry Pi; avoid introducing heavy frontend frameworks.

- Dependencies:
- Ultron backend/template layer, live SSE data pipeline, existing icon/assets, and current deployment/test pipeline.

- Success metrics:
- Baseline: functional but low premium perception and slower visual scan. Target: clear hierarchy across core pages, improved chart legibility, no horizontal overflow on mobile, and consistent component language validated in E2E visual review.

- Assumptions:
- A premium visual redesign can materially improve operator speed/confidence without backend changes; existing stack can support richer visuals and responsive behavior without hurting runtime performance.

- Interview mode:
- standard

## 3. Scope
### In scope
- Dashboard redesign, chart visual upgrade, metrics card redesign, navigation/header polish, settings information architecture for desktop/mobile, unified component styles (buttons/forms/tables/modals/toasts), icon variant strategy, and responsive QA across core pages.

### Out of scope
- Backend feature logic, monitoring engine behavior changes, auth/session logic changes, infrastructure/deployment changes.

## 4. Actors & User Journeys
Actors:
- System admins/operators managing Raspberry Pi services via Ultron web dashboard.

Primary journey:
- Admin logs in, scans health and trends at a glance, drills into issues, executes actions safely, and adjusts settings quickly on desktop or mobile.

## 5. Architecture (Architect Persona)
- Components:
-
- Data flow:
-
- Key decisions:
-
- Risks:
-

## 6. Security (Security Persona)
- Threats:
-
- Controls required:
-
- Validation rules:
-

## 7. Backlog Outline
Epic:
-

User stories:
1.
2.
3.

## 8. Test Strategy
- Smoke tests:
-
- Functional tests:
-
- Security tests:
-
- Edge cases:
-

## 9. Discovery Confidence
- Confidence:
-

- Reason:
-

- Evidence gaps:
-

- Handoff decision:
-
