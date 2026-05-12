## Feature
Network-class alert rules in the existing alert engine — landing the FR-022
gap left by network-monitoring (engine + UI + sustained-duration hysteresis).

## Problem / Why
FR-022 already specifies sustained latency, sustained loss, WAN outage,
DNS-resolver failure rate, and public-IP-change alerts routed through the
existing Telegram/email channels. The implementation never landed:
- `isValidMetric` in [handlers_settings.go:82](internal/server/handlers_settings.go#L82-L88) only accepts `cpu/ram/disk/temp`.
- [alerts/engine.go](internal/alerts/engine.go) has zero references to latency / loss / DNS / WAN / public_ip.
- Alert-rule form in [settings.html](web/templates/settings.html) only offers the four host metrics.
- BG-037 (rule_id=11 flapping every 15–20 min) is the user-visible symptom of the missing sustained-duration parameter — same root cause, fixed here.

This is the last unresolved gap from the network-monitoring rollout, and the
only thing keeping a network operator from getting paged on a real WAN
incident from Ultron-AP itself.

## Target Users
Raspberry Pi Operator — same persona as FR-022. Wants WAN/DNS/latency
incidents to page Telegram/email like CPU/RAM/temp already do, without
flapping.

## New Behavior
The system must:
- Evaluate `latency_<target>` rules: fire when RTT to a configured target
  exceeds `threshold` for >= `sustained_duration` seconds (default 300s),
  respect the existing 15-minute cooldown.
- Evaluate `loss_<target>` rules: same shape, on packet-loss percentage.
- Evaluate `dns_failure_rate` rules: fire when resolver failure-rate
  exceeds threshold for sustained-duration.
- Fire a `critical` alert on FR-018 WAN `outage_start`; fire an `info`
  resolve when WAN comes back.
- Fire an `info` alert on FR-026 public-IP change carrying old + new IP.
- Accept `sustained_duration` (seconds, integer >= 0) on every alert rule
  via a stepper widget in the settings form. Existing host rules treat
  the field as optional — `0` means "fire on first breach", preserving
  current behavior. This single field also closes BG-037 for host rules.
- Extend the metric `<select>` in the alert-rule form with the new
  network metric types; show a target picker when a target-scoped metric
  is selected.
- Persist the new fields via the existing `/api/alerts/rules` endpoint
  (extend `database.AlertConfig`, add a migration that defaults
  `sustained_duration=0` for existing rows).
- Render the new rules in the existing alerts panel with the same
  severity colour tokens, with no new dispatch channel.

## Success Criteria
- Given a latency rule with `target=8.8.8.8`, `threshold=100ms`,
  `sustained_duration=120s`, when RTT exceeds 100ms continuously for
  >= 120s, then a single alert is created (no flapping at 15–20 min
  cadence — closes BG-037).
- Given an existing CPU rule with `sustained_duration=0`, when CPU
  crosses threshold for one tick, then it fires immediately — backward
  compatibility preserved.
- Given the WAN goes down, when `outage_start` fires, then a `critical`
  alert is emitted within one tick; when it recovers, an `info` resolve
  is emitted.
- Given the public IP changes, when FR-026 detects it, then an `info`
  alert is emitted with both old and new IPs in the body.
- The settings alert-rule form lets the operator create any of the new
  rule types end-to-end and persists them in `alert_configs`.

## Out of Scope
- New notification channels — keep Telegram/email only.
- Multi-target rules (one target per rule).
- Speedtest-driven alerts (separate FR).
- Auto-suggested rule presets / templates.
- Migrating BG-037 host-rule into a per-rule `for_duration` framework
  distinct from `sustained_duration` — they collapse into the same field.
