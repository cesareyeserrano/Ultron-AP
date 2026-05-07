## Feature
Richer, context-aware Telegram alert messages that immediately tell the operator **what fired, where, why, and what to do** — replacing today's 5-line generic block from `internal/notify/telegram.go:143` (`FormatAlertMessage`).

## Problem / Why
Today's Telegram message is:
```
🔴 CRITICAL ALERT
*Message:* <whatever caller passed>
*Source:* `cpu_usage`
*Current Value:* `92.4`
*Time:* 2026-05-06 11:14:02
```
Problems:
- No threshold shown — `92.4` has no reference point.
- "Source" is an opaque metric ID (`cpu_usage`), not "CPU on Pi-Ultron" — the operator can't tell at a glance what's affected.
- No rule name, no cooldown context, no link back to the dashboard.
- Resolved alerts use the same template as fire events — confusing.
- For systemd / Docker alerts the message lacks the service / container name and any log tail.
- No visual hierarchy beyond bold labels.
- No suggested next action.
The result is the user pulling up the dashboard anyway to figure out what's going on — the notification adds zero context the dashboard didn't already have.

## Target Users
The single Pi operator (admin) on a phone, often away from the dashboard. Same persona as today — no new user type.

## New Behavior
- The system must include the **rule's configured threshold and operator** alongside the current value (e.g. `CPU 92% (threshold > 80%)`).
- The system must include a **human-readable subject line** with the host name and a friendly metric label (e.g. `CPU usage critical on Ultron`) — never just the metric ID.
- The system must include the **elapsed time since the rule started breaching** when known, otherwise a UTC + local timestamp.
- The system must render distinct templates for **fire** vs **resolve** events — fire = red/yellow/blue circle + "ALERT FIRED"; resolve = green ✓ + "RESOLVED" + duration the alert was active.
- The system must, for **systemd service** alerts, include the service name, current state, active-since timestamp, and the last 3 lines of the service journal (truncated to keep the message under Telegram's 4096-char cap).
- The system must, for **Docker container** alerts, include the container name, image, current state, exit code (if exited), and the last 3 lines of `docker logs --tail 3`.
- The system must, for **disk / RAM / CPU / temperature** alerts, include a short trend hint (current vs 5-minute-ago value) when the metrics ring buffer has the prior sample.
- The system must include a footer line with a **deep link** back to `/alerts` (or the relevant page on the dashboard) — using `ULTRON_PUBLIC_URL` if set, falling back to `http://<pi-host>:<port>` from existing config.
- The system must group multiple simultaneous fires of the same rule within a 60-second window into a single chat row (storm protection) — second and subsequent fires append to the original message via Telegram's `editMessageText`.
- The system must escape Markdown special characters in user-controlled fields (service names, container names, journal output) so a service named `foo_bar` does not break Markdown parsing.
- The system must keep the same delivery contract for the existing `Test Telegram` button — sending a sample alert that previews the new format end-to-end.
- The system must mirror the same body content (with HTML formatting) in the Email notifier so notification surfaces stay consistent — layout-only changes; no new behaviour beyond what Telegram gains.

## Success Criteria
- Given a CPU alert fires at 92% with threshold 80%, when the Telegram message arrives, then it shows: `🔴 CPU usage critical on Ultron — 92% (threshold > 80%) — for 1m20s`, plus a 5-min trend line and a `/alerts` deep link.
- Given a systemd service alert fires for `nginx.service` failed, when the Telegram message arrives, then it includes the service name, current state, active-since timestamp, and the last 3 journal lines (truncated to ≤600 chars).
- Given a Docker container exits non-zero, when the Telegram message arrives, then it includes the container name, image, exit code, and the last 3 lines of `docker logs --tail 3`.
- Given an alert resolves, when the resolve message arrives, then it uses a distinct green ✓ template and shows how long the alert was active (e.g. "active for 4m12s").
- Given the same rule fires 3 times in 60 seconds, when the messages arrive, then the user sees one chat row with an incremented "(3 fires)" counter — not 3 separate notifications.
- Given a service is named `foo_bar` (or `_underscore_chat_id`), when the Telegram message renders, then Markdown does not break.
- Given the user clicks "Test Telegram" in settings, when the test message arrives, then it demonstrates the full new format using a synthetic CPU alert sample.
- No regressions on FR-005 (Telegram delivery) or FR-006 (Email delivery) — existing TestTelegram_* and TestEmail_* tests continue to pass.

## Out of Scope
- Changes to the alert engine, alert-rule schema, or alert evaluation logic.
- Telegram **inline buttons** (acknowledge / mute from chat) — adds bot-side state machine, defer to a follow-up feature.
- Daily digest email format (FR-006 mention) — defer.
- New languages / i18n — message text is English-only.
- Settings page changes (covered by `settings-revamp`).
- Changes to the SQLite schema or stored config shapes.
- Changes to Pironman 5 / Tailscale / WAN-monitor surfaces.
- Adding a Slack / Discord / SMS notifier (out of scope; one channel pair at a time).
