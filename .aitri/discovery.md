# Discovery

## 1. Problem Framing
Ultron already monitors Raspberry Pi system health, but operators still face friction in real-time visibility, interaction clarity, and operational confidence under load/failure conditions.

## 2. User and JTBD
- Primary user: Raspberry Pi operator/admin.
- JTBD: "When monitoring my system, help me understand status and act quickly without ambiguity or risky controls."

## 3. Scope Boundaries
- In scope: dashboard monitoring clarity, UX consistency, reliability signals, and safe operational boundaries.
- Out of scope: infrastructure/platform migration, replacement of core stack, unrelated feature expansion.

## 4. Constraints and Dependencies
- Keep existing stack (Go, HTMX, SSE, SQLite).
- Maintain low CPU/RAM footprint on Raspberry Pi.
- Preserve privileged boundary model and authentication protections.

## 5. Success Metrics
- Primary: critical status visible in first viewport and actionable within 2 interactions.
- Leading: reduced operator navigation steps and lower ambiguous-state occurrences.
- Lagging: fewer monitoring incidents caused by missed/unclear status.

## 6. Risk and Assumption Log
1. Assumption: current templates/services reflect production usage patterns.
2. Risk: optimization may regress readability or responsiveness on low-power devices.
3. Risk: insufficient acceptance criteria could reintroduce ambiguity in later phases.

## 7. Discovery Confidence
- Confidence: Medium
- Reason: baseline and constraints are clear, but measurable operational targets need refinement per feature.
- Evidence gaps: explicit SLA/SLO targets and incident baseline.
- Handoff decision: Ready for Product/Architecture.
