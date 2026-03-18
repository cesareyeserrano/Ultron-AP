# Discovery — Ultron-AP

## 1. Problem Framing

Ultron-AP solves the problem of a Raspberry Pi operator needing centralized, real-time visibility into their system without SSH access. The device runs multiple Docker containers and Systemd services, but there is no single place to see health status, receive alerts when thresholds are crossed, or control services remotely. The panel must run on constrained hardware (~1-4GB RAM, ARM CPU) and must not become the dominant resource consumer.

## 2. User and JTBD

- **Primary user:** Raspberry Pi operator/admin (single user).
- **JTBD:** "When monitoring my Raspberry Pi, help me understand system and service health at a glance, receive proactive alerts, and act on issues without needing SSH — from any device on the network."

## 3. Scope Boundaries

- **In scope:** Real-time system metrics dashboard, Docker/Systemd monitoring and controls, threshold-based alerting (Telegram + Email), authentication, action audit trail, hardware integration (Pironman 5), VPN (Tailscale) status.
- **Out of scope:** Multi-node deployments, external cloud integrations, SPA frontend frameworks, runtime dependencies beyond the Go binary.

## 4. Constraints and Dependencies

- Single Go binary with zero runtime dependencies.
- HTMX + Tailwind CSS frontend — no SPA framework.
- SQLite for persistence (configuration, users, alerts, history).
- SSE for live updates; HTMX/HTTP for commands.
- Must maintain strict privilege separation: web process unprivileged, host actions via Unix socket helper.
- ARM64 target; ~15MB RAM footprint budget.

## 5. Success Metrics

- **Primary:** Critical system state visible above the fold; corrective action reachable in ≤ 2 interactions.
- **Leading:** Reduced navigation steps; fewer ambiguous-state occurrences.
- **Lagging:** Zero monitoring incidents caused by missed or unclear status.

## 6. Risk and Assumption Log

1. Assumption: the operator is the sole user; no multi-tenant requirements.
2. Assumption: LAN/VPN access is sufficient; public internet exposure is out of scope.
3. Risk: SSE fanout under multiple browser clients may increase CPU — mitigated by differentiated cadences and compact payloads.
4. Risk: SQLite write contention under burst alert events — mitigated by bounded retries with jitter.
5. Risk: Privileged helper socket misconfiguration could expose host actions — mitigated by strict Unix socket permissions and audit logging.

## 7. Discovery Confidence

- Confidence: HIGH — codebase is mature (v1.0.0-stable), all major features implemented and tested.
