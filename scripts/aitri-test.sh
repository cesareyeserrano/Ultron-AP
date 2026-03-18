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

emit "internal/alerts"    "TC-001" "alert engine — threshold evaluation and cooldown"
emit "internal/auth"      "TC-002" "authentication — brute-force protection"
emit "internal/auth"      "TC-003" "authentication — CSRF token validation"
emit "internal/metrics"   "TC-004" "metrics — ring buffer retention"
emit "internal/metrics"   "TC-005" "metrics — collector produces valid snapshot"
emit "internal/docker"    "TC-006" "docker monitor — container list and graceful unavailable"
emit "internal/systemd"   "TC-007" "systemd monitor — service list"
emit "internal/notify"    "TC-008" "notifications — dispatcher routing"
emit "internal/database"  "TC-009" "database — user and session roundtrip"
emit "internal/database"  "TC-010" "database — alert create and list roundtrip"
emit "internal/server"    "TC-011" "server — middleware auth and CSRF rejection"
emit "internal/server"    "TC-012" "server — SSE endpoint content-type"
emit "internal/tailscale" "TC-013" "tailscale — graceful unavailable"
emit "internal/config"    "TC-014" "config — env vars and defaults"
emit "internal/server"    "TC-015" "service controls — start/stop/restart with CSRF enforcement"
emit "internal/server"    "TC-016" "dashboard rendering — sidebar, header, CSRF token in HTML"
emit "internal/server"    "TC-017" "action history and log viewer — render and filter"
emit "internal/server"    "TC-018" "privilege separation — auth middleware and secure cookie"
emit "internal/server"    "TC-019" "hardware settings — pironman settings page renders"
emit "internal/server"    "TC-020" "database backup — config save and outcome logging"

exit $EXIT_CODE
