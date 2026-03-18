# Product Spec

## Problem
Operators need reliable, low-friction, real-time monitoring in Ultron with clear status communication and safe boundaries.

## Users
- Raspberry Pi operator/admin.

## Goals
- Improve monitoring clarity and usability.
- Preserve security and least-privilege boundaries.
- Keep resource usage low on Raspberry Pi.

## Non-Goals
- Replatforming stack.
- Broad new product modules outside monitoring improvements.

## Functional Expectations
- Clear dashboard status hierarchy.
- Reliable live updates with graceful degraded states.
- Consistent navigation and interaction behavior.
- No privileged operations from web layer.

## Constraints
- Existing stack only (Go + HTMX + SSE + SQLite).
- Production safety first.
- No heavy client dependencies.

## Acceptance Direction
- Operator can identify critical state quickly.
- Monitoring flow remains stable under slow/missing data.
- Security controls and boundary model remain intact.
