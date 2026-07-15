#!/usr/bin/env bash
# security-gate.sh — re-checks the aitri audit security findings (2026-07-14).
# Exit 0 = the audited posture holds; non-zero = a finding regressed.
# Declared as a quality_gate so `verify` re-checks security on every cycle.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2
fail=0; note(){ echo "FAIL: $*"; fail=1; }

# Secrets: no secret-like file tracked; no committed Telegram-token shape
git ls-files | grep -Ei '\.(env|db|pem|key)$|credentials\.json' && note "secret-like file tracked"
git grep -nE '[0-9]{8,10}:AA[A-Za-z0-9_-]{30,}' -- ':!*.md' ':!*.example' && note "possible Telegram token committed"

# Root-helper invariants still present (crown jewel)
grep -q 'serviceNameRe = regexp.MustCompile' cmd/ultron-helper/main.go || note "serviceNameRe allow-list missing"

# No new Sprintf-built SQL beyond the known VACUUM INTO
grep -rnE 'Exec\(|Query\(' internal/database/*.go 2>/dev/null | grep -i sprintf \
  | grep -v 'VACUUM INTO' | grep -v _test.go | grep . && note "new Sprintf-built SQL"

# RQ-SEC-002: sudoers must not ship a wildcard privilege entry
[ -f deploy/ultron-ap.sudoers ] && grep -qE 'pironman5 \*' deploy/ultron-ap.sudoers && note "sudoers ships pironman5 wildcard (RQ-SEC-002)"

# RQ-SEC-004: no .DS_Store embedded in the binary
[ -f web/static/.DS_Store ] && note ".DS_Store present in embedded web/static (RQ-SEC-004)"

# RQ-SEC-005: shipped client JS must not leak Aitri trace IDs
git grep -lnE '(FR|BG|AC|TC)-[0-9]+' -- 'web/static/js/**' >/dev/null 2>&1 && note "Aitri trace IDs in client JS (RQ-SEC-005)"

exit $fail
