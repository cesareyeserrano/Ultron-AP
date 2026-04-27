# Discovery — Ultron-AP

## Problem

Ultron-AP solves the problem of a Raspberry Pi operator needing centralized, real-time visibility into their system without SSH access. The device runs multiple Docker containers and Systemd services, but there is no single place to see health status, receive alerts when thresholds are crossed, or control services remotely. The panel must run on constrained hardware (~1–4 GB RAM, ARM CPU) and must not become the dominant resource consumer.

## Users

- **Primary user:** Raspberry Pi operator/admin (single user).
- **JTBD:** "When monitoring my Raspberry Pi, help me understand system and service health at a glance, receive proactive alerts, and act on issues without needing SSH — from any device on the network."

## Success Criteria

- **Primary:** Critical system state visible above the fold; corrective action reachable in ≤ 2 interactions.
- **Leading:** Reduced navigation steps; fewer ambiguous-state occurrences.
- **Lagging:** Zero monitoring incidents caused by missed or unclear status.

Quantified targets:
- Critical metric latency ≤ 5 s end-to-end.
- Memory footprint ≤ 15 MB at idle on ARM Cortex-A.
- WCAG 2.1 AA contrast on dashboard text.

## Out of Scope

- Multi-node deployments and cluster monitoring.
- External cloud integrations beyond Telegram/SMTP.
- SPA frontend frameworks (React, Vue, etc.).
- Runtime dependencies beyond the Go binary itself.
- Public-internet exposure (LAN/VPN only).

## Constraints and Dependencies

- Single Go binary with zero runtime dependencies.
- HTMX + Tailwind CSS frontend — no SPA framework.
- SQLite for persistence (configuration, users, alerts, history).
- SSE for live updates; HTMX/HTTP for commands.
- Strict privilege separation: web process unprivileged, host actions via Unix socket helper.
- ARM64 target; ~15 MB RAM footprint budget.

## Risk and Assumption Log

1. Assumption: the operator is the sole user; no multi-tenant requirements.
2. Assumption: LAN/VPN access is sufficient; public-internet exposure is out of scope.
3. Risk: SSE fanout under multiple browser clients may increase CPU — mitigated by differentiated cadences and compact payloads.
4. Risk: SQLite write contention under burst alert events — mitigated by bounded retries with jitter.
5. Risk: Privileged helper socket misconfiguration could expose host actions — mitigated by strict Unix socket permissions and audit logging.

## Discovery Confidence

Confidence: high
Evidence gaps: none — codebase is mature (v1.0.0-stable), all major features implemented and tested with 20/20 passing.
Handoff decision: ready — proceed to Phase 1 (Requirements).
