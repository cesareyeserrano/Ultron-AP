# Hardware Integration Operations Policy

Date: 2026-02-25
Scope: Ultron core on Raspberry Pi 5 with optional Pironman stack.

## Non-Intrusive Host Rule
- Ultron core must not start, stop, or restart Pironman/Influx services from Settings or automatic jobs.
- Optional integrations are observed, not orchestrated, from core runtime.
- Privileged execution remains isolated to `ultron-helper` local boundary.

## Capability Contract
- Pironman integration state is reported as:
  - `available`: helper reachable and Pironman config parsed correctly.
  - `degraded`: helper reachable but Pironman API timeout or invalid payload.
  - `unavailable`: helper/runtime not reachable.
- Core UX must fail fast with explicit status and no side effects when unavailable/degraded.

## Resource Attribution Guardrail
- Process-level diagnostics must be on-demand (no periodic polling loop).
- Baseline budget targets for quick triage:
  - `ultron-ap`: <= 2.0% CPU
  - `ultron-helper`: <= 1.0% CPU
  - `pironman5-service`: <= 5.0% CPU
  - `influxd`: <= 2.0% CPU
- Snapshot includes CPU% and RSS MB per process.

## Security Posture
- Keep `NoNewPrivileges=true` on web process.
- Keep helper socket local and permission-scoped.
- Maintain CSRF/session enforcement on mutation endpoints.
