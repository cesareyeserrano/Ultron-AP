#!/usr/bin/env bash
# Compare the vulnerabilities govulncheck found REACHABLE from our code against
# the reviewed allowlist.
#
# Fails on:
#   1. a reachable vulnerability that is not on the allowlist  (something new)
#   2. an allowlist entry past its review-by date              (accepted too long)
#   3. an allowlist entry no longer reported                   (stale — delete it)
#
# (3) is the point: an allowlist that is never pruned stops being a decision and
# becomes noise everyone scrolls past.
set -euo pipefail

scan="${1:?usage: check-vulns.sh <govulncheck.json> <allowlist>}"
allowlist="${2:?usage: check-vulns.sh <govulncheck.json> <allowlist>}"

# A finding whose trace has a function is one govulncheck traced into our own
# call graph. Findings without one are "in a module we require but never call".
mapfile -t reachable < <(
  jq -rs '[.[] | select(.finding) | .finding
          | select(.trace[0].function != null) | .osv] | unique | .[]' "$scan"
)

mapfile -t allowed < <(grep -oE '^GO-[0-9]{4}-[0-9]+' "$allowlist" || true)

fail=0
today=$(date -u +%Y-%m-%d)

echo "Reachable: ${reachable[*]:-none}"
echo "Allowed:   ${allowed[*]:-none}"
echo

for id in "${reachable[@]:-}"; do
  [[ -z "$id" ]] && continue
  if ! printf '%s\n' "${allowed[@]:-}" | grep -qx "$id"; then
    echo "::error::$id is reachable from our code and is NOT on the allowlist."
    echo "         Fix it, or add it to $allowlist with a justification and a review-by date."
    fail=1
  fi
done

for id in "${allowed[@]:-}"; do
  [[ -z "$id" ]] && continue

  if ! printf '%s\n' "${reachable[@]:-}" | grep -qx "$id"; then
    echo "::error::$id is on the allowlist but is no longer reachable — the exemption is stale."
    echo "         Remove its line from $allowlist."
    fail=1
    continue
  fi

  review_by=$(grep -E "^$id" "$allowlist" | grep -oE 'review-by=[0-9-]+' | cut -d= -f2)
  if [[ -n "$review_by" && "$today" > "$review_by" ]]; then
    echo "::error::$id was accepted until $review_by (today is $today). Re-check whether a fix exists."
    echo "         If it is still unfixable, extend review-by with a fresh justification."
    fail=1
  fi
done

if [[ $fail -eq 0 ]]; then
  echo "✅ Security gate: no unreviewed reachable vulnerabilities."
fi
exit $fail
