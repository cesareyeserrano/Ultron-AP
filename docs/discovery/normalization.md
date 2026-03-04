# Discovery: normalization

STATUS: DRAFT

## 1. Problem Statement
Derived from approved spec retrieval snapshot:
- Retrieval mode: section-level
- Retrieved sections: 1. Context, 2. Actors, 3. Functional Rules, 7. Security Considerations, 8. Out of Scope, 9. Acceptance Criteria

### Context snapshot
- Take the existing Ultron codebase as baseline and implement the recently documented stabilization and UX/security/QA improvements on top of it, without replatforming.
- Primary actor: Raspberry Pi operator/admin.
- Expected outcome: Operator can identify critical system state quickly, monitor live metrics reliably, navigate to corrective modules in at most two interactions, and keep secure/low-resource operation.

### Actors snapshot
- Raspberry Pi operator/admin.

### Functional rules snapshot
- The system must present critical monitoring status clearly at first glance and keep live dashboard updates reliable with explicit degraded-state signaling.
- The system must preserve strict security boundaries: authenticated access, CSRF/origin protection for state-changing routes, and no privileged execution from web handlers.

### Security snapshot
- Enforce whitelist-first input validation plus rate limits on auth and high-cost endpoints (including SSE reconnect abuse), and audit-log all rejects.

### Out-of-scope snapshot
- Excluded: stack replatforming, infrastructure migration, and unrelated new product modules outside monitoring stabilization.

Refined problem framing:
- What problem are we solving? Intermittent ambiguity in degraded telemetry states and inconsistent monitoring clarity can delay diagnosis during daily operations.
- Why now? Critical status visible in first viewport for authenticated sessions, corrective navigation in <=2 interactions, and explicit stale-data signaling with recovery path when telemetry degrades.

## 2. Discovery Interview Summary (Discovery Persona)
- Primary users:
- Raspberry Pi operator/admin.

- Jobs to be done:
- Identify system state quickly, monitor live telemetry reliably, and reach corrective modules in at most two interactions.

- Current pain:
- Intermittent ambiguity in degraded telemetry states and inconsistent monitoring clarity can delay diagnosis during daily operations.

- Constraints (business/technical/compliance):
- Keep existing stack (Go + HTMX + SSE + SQLite + Tailwind), preserve low Raspberry Pi resource usage, and maintain strict auth/CSRF/origin and least-privilege boundaries.

- Dependencies:
- Local collectors (metrics/docker/systemd), SQLite persistence, existing Ultron web modules, and local privileged helper boundary.

- Success metrics:
- Critical status visible in first viewport for authenticated sessions, corrective navigation in <=2 interactions, and explicit stale-data signaling with recovery path when telemetry degrades.

- Assumptions:
- Assumptions embedded in approved spec scope

- Interview mode:
- quick

## 3. Scope
### In scope
- Approved spec functional scope

### Out of scope
- Anything not explicitly stated in approved spec

## 4. Actors & User Journeys
Actors:
- Raspberry Pi operator/admin.

Primary journey:
- Primary journey derived from approved spec context

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
