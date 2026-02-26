# AF-SPEC: ux-ui-upgrade

STATUS: APPROVED
## 1. Context
A complete UX/UI upgrade for Ultron: modern and premium dashboard experience across desktop and mobile.

Primary actor: Primary user: system admin/operator managing Raspberry Pi services from Ultron.
Expected outcome: Admins can monitor key telemetry quickly, understand service health instantly, and execute actions confidently with a clear, polished, premium interface on desktop and mobile.
In scope: Dashboard visual redesign, improved metrics cards and charts readability, modern navigation/header, improved settings UX, consistent component system (buttons/forms/tables/modals), responsive layouts for desktop/mobile, and icon/color treatment updates.
Out of scope: No backend business-logic changes, no infrastructure changes, no monitoring engine rewrites; this feature is UI/UX and presentation-focused.
Technology: Keep current stack: Go server-rendered templates + HTMX + Tailwind CSS build + vanilla JS.
Requirement source: Provided explicitly by user in guided draft.

## 2. Actors
- Primary user: system admin/operator managing Raspberry Pi services from Ultron.

## 3. Functional Rules (traceable)
- FR-1: The system must define and apply a dashboard visual asset strategy that uses existing local iconography and CSS-driven primitives first, and introduces new branded assets only when required for readability or hierarchy.

## 4. Edge Cases
- When live data is delayed/unavailable, the UI must still present clear placeholders and statuses without layout jumps or unreadable states.

## 5. Failure Conditions
- UI presents unreadable or low-contrast content on dark surfaces.
- Desktop or mobile layouts introduce horizontal overflow in core pages.
- Dashboard/Settings redesign breaks operational clarity (critical status and actions are harder to locate than current baseline).

## 6. Non-Functional Requirements
- Performance parity: visual upgrade must not introduce noticeable interaction lag in normal dashboard navigation.
- Responsive reliability: desktop and mobile views must remain stable with no layout shift during live data updates.
- Consistency: shared component language (buttons, badges, panels, forms, modals, tables) must be applied across all in-scope pages.

## 7. Security Considerations
- UI changes must preserve all existing CSRF/authentication flows and must not expose sensitive configuration values in visible states.

## 8. Out of Scope
- No backend business-logic changes, no infrastructure changes, no monitoring engine rewrites; this feature is UI/UX and presentation-focused.

## 9. Acceptance Criteria
- AC-1: Given an authenticated admin on desktop and mobile, when navigating Dashboard, Docker, Services, Alerts, Logs, History, and Settings, then each page renders with the new premium visual system consistently and remains fully usable without horizontal overflow.
- AC-2: Given the Ultron icon on dark surfaces, when rendered in app chrome (sidebar/header/login/favicon context), then the icon remains clearly visible using an approved variant (`on-dark` light/metallic or accent-tinted), while preserving the original black silhouette for neutral/print contexts.

## 10. Requirement Source Statement
- All requirements in this draft were provided explicitly by the user.
- Aitri structured the content and did not invent requirements.

## 11. Resource Strategy
- Reuse existing local assets and icon set; no mandatory external assets required.
- Icon adjustment required in this feature:
  - Preserve current black icon shape as master form.
  - Add `on-dark` UI variant for readability on dark backgrounds.
  - Add optional `brand` accent variant for premium product expression.
