# Backlog: pironman-optional-integration

## Epics
- EP-1 Optional Module Gating
- EP-2 Deterministic Apply + Capability Contract
- EP-3 Security Boundary Hardening

### US-1
- Story: As a platform maintainer, I want Pironman integration disabled by default and enabled only via feature flag, so core Ultron stays unaffected by default.
- Trace: FR-1, AC-2
- Acceptance Criteria:
- Given `ULTRON_FEATURE_PIRONMAN=false`, when user navigates core, then no optional Pironman route/action is exposed.
- Given `ULTRON_FEATURE_PIRONMAN=true`, when user opens settings, then optional module entrypoint is available.

### US-2
- Story: As an admin operator, I want apply to execute only on explicit action and one operation at a time, so UI never gets stuck busy.
- Trace: FR-2, AC-1
- Acceptance Criteria:
- Given edited values, when Apply is not pressed, then no apply request is sent.
- Given active apply, when repeated apply clicks occur, then system remains deterministic and recoverable.

### US-3
- Story: As an admin operator, I want capability states `available|degraded|unavailable`, so I can understand health without guessing.
- Trace: FR-3, AC-3
- Acceptance Criteria:
- Given Pironman timeout, when read/apply runs, then status becomes `degraded` and core remains usable.
- Given helper/socket unavailable, when read/apply runs, then status becomes `unavailable`.

### US-4
- Story: As a security owner, I want strict helper boundary and validated payloads, so web process cannot escalate privileges.
- Trace: FR-4, AC-4
- Acceptance Criteria:
- Given invalid payload values, when apply is requested, then input is sanitized/rejected and rejection is auditable.
- Given normal apply, when action executes, then privileged operation stays in helper path only.
