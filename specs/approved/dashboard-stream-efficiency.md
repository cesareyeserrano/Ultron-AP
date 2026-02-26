# AF-SPEC: dashboard-stream-efficiency

STATUS: APPROVED
Tech Stack: Go service + HTMX + SSE

## 1. Context
Optimize Ultron dashboard streaming for Raspberry Pi: move high-frequency dashboard updates to lightweight SSE JSON channels, keep UX fluid, add per-channel update cadence (metrics fast, charts medium, services slower), preserve security boundaries and low resource footprint.

---

## 2. Actors
- Raspberry Pi Operator: uses Ultron dashboard to monitor system state in real time.
- Platform Maintainer: keeps Ultron resource usage low and verifies production stability.

## 3. Functional Rules (traceable)
Use stable IDs. Sub-bullets add detail — they are included in the rule text.

- FR-1: Dashboard stream path must support lightweight updates for high-frequency cards.
  - Metrics panel and chart panel updates must be independently schedulable.
- FR-2: Dashboard must include temperature history chart.
  - Temperature chart must use last collected samples from server collector history.
- FR-3: Temperature indicator and temperature chart must expose 3 visual states.
  - Normal state: green.
  - Warning state: yellow.
  - High state: red.
- FR-4: SSE broadcast must avoid unnecessary heavy updates.
  - Charts must refresh at a slower cadence than metrics by default.

## 4. Edge Cases
- Temperature sensor unavailable (`nil`): dashboard must render placeholder and avoid panics.
- Sparse history (few points after startup): charts must render with available points only.
- Slow browser client: server must not block main update loop for a single client.

## 5. Failure Conditions
- SSE connection drops: client reconnects and receives latest values without server restart.
- Template render failure in one partial: error is logged and server process continues.
- Invalid threshold handling: any unknown value defaults to non-crashing fallback class.

## 6. Non-Functional Requirements
- NFR-1: Keep Ultron lightweight on Raspberry Pi 5.
  - Avoid adding client-side chart libraries.
  - Keep server-side rendering simple and bounded.
- NFR-2: Preserve dashboard responsiveness.
  - Fast metrics updates remain visually near-real-time.
- NFR-3: Maintain production safety.
  - No changes to privileged helper boundary or host control permissions.

## 7. Security Considerations
- Keep existing privilege separation: web process remains unprivileged and does not execute host-privileged commands.
- Do not introduce new external endpoints or unauthenticated routes for dashboard stream changes.
- Keep server-side template rendering safe; avoid eval/dynamic script injection.

## 8. Out of Scope
- Replacing SSE with WebSockets.
- Introducing heavy frontend frameworks or charting dependencies.
- Changing Docker/Systemd/Pironman control permissions.

## 9. Acceptance Criteria (Given/When/Then)
- AC-1: Given an authenticated operator on dashboard, when SSE stream runs, then metrics remain frequently updated and charts update at a slower cadence.
- AC-2: Given temperature samples in collector history, when charts render, then temperature history chart is visible.
- AC-3: Given temperature value in normal/warning/high range, when dashboard renders indicator and chart, then colors are green/yellow/red respectively.
- AC-4: Given active SSE clients, when chart cadence is reduced, then non-chart dashboard updates continue without waiting for chart partials.

## 10. Requirement Source Statement
- Requirements must be provided explicitly by the user.
- Aitri does not invent requirements.
