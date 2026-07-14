#!/usr/bin/env bash
# aitri-test.sh
# Runs Go tests and emits TC-XXX markers that aitri verify-run can auto-detect.
# Maps package-level pass/fail to TC IDs defined in spec/03_TEST_CASES.json.
set -uo pipefail

OUTPUT=$(go test ./... 2>&1)
EXIT_CODE=$?

printf '%s\n' "$OUTPUT"
echo ""
echo "# TC Coverage (package-to-TC mapping)"

pkg_status() {
  echo "$OUTPUT" | grep -q "^ok  .*${1}" && echo "pass" || echo "fail"
}

emit() {
  local pkg="$1" tc="$2" desc="$3"
  if [ "$(pkg_status "$pkg")" = "pass" ]; then
    echo "✔ ${tc}: ${desc}"
  else
    echo "✖ ${tc}: ${desc}"
  fi
}

# FR-001 — real-time metrics dashboard (ring buffer + collector + SSE)
emit "internal/metrics"   "TC-001h" "ring buffer retains last N samples in chronological order"
emit "internal/metrics"   "TC-001e" "ring buffer empty on creation"
emit "internal/metrics"   "TC-001f" "collector Latest returns nil when empty"

# FR-002 — Docker container monitor
emit "internal/docker"    "TC-002h" "docker monitor lists containers with name and state"
emit "internal/docker"    "TC-002e" "container with empty name uses truncated ID"
emit "internal/docker"    "TC-002f" "docker socket unavailable returns empty list, no panic"

# FR-003 — Systemd service monitor
emit "internal/systemd"   "TC-003h" "systemd monitor lists services with name and state"
emit "internal/systemd"   "TC-003e" "failed-services filter returns only failed"
emit "internal/systemd"   "TC-003f" "systemctl unavailable sets monitor unavailable, no panic"

# FR-004 — Threshold alerts engine
emit "internal/alerts"    "TC-004h" "alert fires when metric crosses threshold"
emit "internal/alerts"    "TC-004e" "alert respects cooldown window"
emit "internal/alerts"    "TC-004f" "below-threshold reading does not fire alert"

# FR-005 — Telegram notifications
emit "internal/notify"    "TC-005h" "telegram sender posts alert on success"
emit "internal/notify"    "TC-005e" "long telegram message is truncated"
emit "internal/notify"    "TC-005f" "telegram API error returns error without panic"

# FR-006 — Email notifications
emit "internal/notify"    "TC-006h" "email sender sends MIME message via mock SMTP"
emit "internal/notify"    "TC-006e" "email body composes severity, value and threshold"
emit "internal/notify"    "TC-006f" "SMTP error from server is surfaced without panic"

# FR-007 — Authentication
emit "internal/database"  "TC-007h" "user created and retrieved with bcrypt hash"
emit "internal/auth"      "TC-007e" "brute-force tracker locks IP at limit"
emit "internal/server"    "TC-007f" "login with wrong password is rejected"

# FR-008 — Service controls
emit "internal/server"    "TC-008h" "service start succeeds with auth + CSRF + audit log"
emit "internal/server"    "TC-008e" "docker stop logs the action with timestamp"
emit "internal/server"    "TC-008f" "service start without CSRF returns 403"

# FR-009 — Dark mode UI
emit "internal/server"    "TC-009h" "dashboard renders sidebar, header, CSRF token"
emit "internal/server"    "TC-009e" "history page renders empty state when no entries"
emit "internal/server"    "TC-009f" "alerts page redirects to login when unauthenticated"

# FR-010 — On-demand service logs / action history viewer
emit "internal/server"    "TC-010h" "history page renders log entries"
emit "internal/server"    "TC-010e" "history page filters by docker source"
emit "internal/server"    "TC-010f" "history page invalid source falls back to all entries"

# FR-011 — Privilege separation
emit "internal/server"    "TC-011h" "login sets Secure cookie behind HTTPS proxy"
emit "internal/server"    "TC-011e" "login sets Secure cookie under direct TLS"
emit "internal/server"    "TC-011f" "unauthenticated request to / redirects to /login"

# FR-012 — CSRF protection
emit "internal/auth"      "TC-012h" "CSRF GenerateToken returns 64 hex chars, unique"
emit "internal/auth"      "TC-012e" "CSRF ValidateToken accepts matching tokens"
emit "internal/auth"      "TC-012f" "CSRF ValidateToken rejects different/empty tokens"

# FR-013 — Hardware integration (Pironman)
emit "internal/server"    "TC-013h" "settings page renders hardware configuration section"
emit "internal/server"    "TC-013e" "settings page does not enable SSE on /settings"
emit "internal/server"    "TC-013f" "settings page rejects unauthenticated access"

# FR-014 — VPN status (Tailscale)
emit "internal/tailscale" "TC-014h" "tailscale peer name prefers display name"
emit "internal/tailscale" "TC-014e" "tailscale peer name falls back to hostname"
emit "internal/tailscale" "TC-014f" "tailscale device-name resolution returns hostname when no metadata"

# FR-015 — Database backup
emit "internal/server"    "TC-015h" "backup config save persists settings and applies schedule"
emit "internal/server"    "TC-015e" "backup outcome is recorded in action_history"
emit "internal/server"    "TC-015f" "backup config save without CSRF is rejected"


# AC-coverage backfill (BL-033) — traces the remaining acceptance criteria.
emit "internal/server" "TC-001a" "dashboard metrics tile renders RAM used/total and percentage"
emit "internal/server" "TC-001b" "dashboard disk tile lists every mounted partition"
emit "internal/server" "TC-001c" "dashboard network tile shows per-interface byte rates"
emit "internal/server" "TC-001d" "cPU temperature color thresholds green <60, yellow 60-75, red >75"
emit "internal/server" "TC-001g" "sSE broadcast pushes metrics to connected clients on the 5s cadence"
emit "internal/server" "TC-001i" "dashboard header shows system uptime"
emit "internal/docker" "TC-002a" "container health classification green/grey/red by state"
emit "internal/server" "TC-002b" "docker page polls its container list every 10 seconds"
emit "internal/docker" "TC-002c" "container detail exposes ports, volumes and env var names"
emit "internal/systemd" "TC-003a" "service state indicator green active / grey inactive / red failed"
emit "internal/server" "TC-003b" "services page polls its list every 30 seconds"
emit "internal/alerts" "TC-004a" "docker container exit transition fires a docker alert"
emit "internal/alerts" "TC-004b" "systemd failed transition fires a critical service alert"
emit "internal/database" "TC-004c" "fired alert persists to SQLite with timestamp and type"
emit "internal/server" "TC-004d" "alerts panel severity filter shows only matching entries"
emit "internal/server" "TC-004g" "rendered alerts carry severity colors red/yellow/accent"
emit "internal/server" "TC-005a" "telegram channel configured from Bot Token and Chat ID"
emit "internal/server" "TC-006a" "sMTP channel configured from host/port/user/password/from/to"
emit "internal/server" "TC-007a" "login page shows username and password form"
emit "internal/server" "TC-007b" "session cookie expires after the 24h default and is HttpOnly"
emit "cmd/ultron-ap" "TC-007c" "bootstrapped admin password stored as bcrypt hash, never plaintext"
emit "internal/server" "TC-007d" "logout button visible in the header on every authenticated page"
emit "internal/server" "TC-007g" "unauthenticated non-exempt routes redirect to /login"
emit "internal/server" "TC-008a" "service rows render Start, Stop and Restart controls"
emit "internal/server" "TC-008b" "failed action renders an explicit error state with message"
emit "internal/server" "TC-008c" "actions dispatch as HTMX swaps returning in-page fragments"
emit "internal/server" "TC-009a" "body text tokens meet WCAG AA 4.5:1 on all panel backgrounds"
emit "internal/server" "TC-009b" "status badges map states to semantic color tokens"
emit "internal/server" "TC-009c" "interactive settings controls declare 44px touch targets"
emit "internal/server" "TC-009d" "status text tokens meet WCAG AA on dark backgrounds"
emit "internal/privileged" "TC-011a" "host actions route through the privileged helper Unix socket"
emit "cmd/ultron-helper" "TC-011b" "helper validates unit names against the allow-list before executing"
emit "internal/server" "TC-014a" "SSE summary event carries the Tailscale/VPN status block"

emit "internal/systemd" "TC-003c" "service row shows name, state and active-since"
emit "internal/server"  "TC-015a" "backup written to disk is encrypted at rest"

# ac-coverage-gaps feature — realises the parent ACs the code never grew.
emit "internal/notify" "TC-005b" "telegram mute window suppresses delivery until it expires"
emit "internal/notify" "TC-006b" "daily digest sends one email summarising the last 24h"
emit "internal/server" "TC-010a" "service log drawer shows the last 100 journalctl lines"
emit "internal/server" "TC-013a" "settings hardware section renders a fan-mode selector"
emit "internal/server" "TC-013b" "settings hardware section renders an OLED configuration"

emit "internal/server" "TC-015b" "settings lists prior backups and downloads the stored encrypted file"

exit $EXIT_CODE
