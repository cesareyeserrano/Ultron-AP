## Feature
A declarative rules engine that consumes Ultron's existing telemetry streams (CPU, RAM, disk, temp, services, containers, WAN, LAN devices) and emits real-time verdicts: short, actionable diagnostic statements with severity and recommendation. Rules are data, not code — adding a new rule is one JSON entry, not a compile.

## Problem / Why
Ultron tells you WHAT is happening (CPU 90%, RTT 200 ms) but not WHAT IT MEANS. The operator has to mentally compose multiple metrics to diagnose ("CPU is high AND temperature is high → thermal throttling probable"). Every new module today ships its own ad-hoc thresholds and has no diagnostic glue. A rules engine fixes both.

## Target Users
Existing Ultron operators (Pi household admin). Same dashboard, same SSE stream, same /network page — a new "Operational indicators" section surfaces verdicts. No new user type.

## New Behavior
- The system must evaluate a registered set of rules every metrics tick (5 s) and emit a verdict for each rule whose condition is currently true.
- Each rule has: id, title, condition (boolean over telemetry vars), severity (info/warn/critical), verdict text, recommendation text, links (optional doc anchors for /help — BL-018).
- Rules must compose across modules: CPU + temp, gateway + cloudflare, ram + swap, lan-device-count + alert-rate, etc.
- Verdicts must stream over the existing SSE channel so the dashboard updates without polling.
- The dashboard must render an "Operational indicators" section listing active verdicts, sorted critical → warn → info, with title + recommendation visible.
- Rule definitions persist in SQLite and can be enabled/disabled (UI later).
- A bootstrap set of ~10 hand-curated rules ships in v1 covering: thermal, memory pressure, WAN/LAN disambiguation, disk near-full, container/service failed, sustained packet loss.

## Success Criteria
- Given the Pi has CPU > 90% for 30 s AND temp > 75 °C, when the operator looks at the dashboard, then a critical verdict "Thermal throttling probable" appears within 5 s with a recommendation to kill heavy loads.
- Given the gateway probe is OK AND the cloudflare probe is failing, when the operator looks at the dashboard, then a warning verdict "LAN ok, ISP/WAN down" appears.
- Given a rule's condition flips from true to false, then its verdict disappears from the dashboard within one SSE tick.
- The bootstrap rule set produces 0 false positives during 1 hour of normal operation on the Pi (validation: no verdicts emitted on a healthy idle Pi).

## Out of Scope
- Custom rule authoring UI (operator-defined rules) — rule definitions in v1 are code-bundled JSON; UI editor is a v2 follow-up.
- Cross-host correlation — rules evaluate the local Pi only.
- ML / anomaly detection — strictly threshold-and-boolean rules in v1.
- Push notifications for verdicts — that responsibility stays with the existing alert engine. Verdicts are diagnostic context, not alerts.
- The /help glossary surface (BL-018) — separate feature.
- Time-series-of-verdicts history view — only the live state in v1.
