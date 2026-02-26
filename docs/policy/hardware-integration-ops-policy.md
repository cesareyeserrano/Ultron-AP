# Hardware Integration Operations Policy

Date: 2026-02-25
Scope: Ultron core on Raspberry Pi 5 with external hardware stack.

## Non-Intrusive Host Rule
- Ultron core must not start, stop, or restart Pironman/Influx services from Settings or automatic jobs.
- Pironman is external-only and managed outside Ultron (`:34001`).
- Privileged execution remains isolated to `ultron-helper` local boundary.

## Capability Contract
- Ultron must not expose Pironman control endpoints, forms, or helper actions.
- Diagnostics are generic runtime snapshots and must not include Pironman-specific control logic.

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
