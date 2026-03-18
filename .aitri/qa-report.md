# QA Report: normalization
Date: 2026-03-04T17:05:45-05:00

## Results
- AC-1: PASS — evidence: `curl -sS -o /tmp/ultron-qa-login.out -w '%{http_code}' -b /tmp/ultron-qa-normalization.cookies -c /tmp/ultron-qa-normalization.cookies -X POST http://127.0.0.1:8091/login --data 'username=admin&password=admin123&csrf_token=<token>'` => `303`; `curl -sS -b /tmp/ultron-qa-normalization.cookies http://127.0.0.1:8091/` => contains `Live telemetry healthy` and `System`; same dashboard response contains `href="/alerts"`, and `curl -sS -o /tmp/ultron-qa-alerts.out -w '%{http_code}' -b /tmp/ultron-qa-normalization.cookies http://127.0.0.1:8091/alerts` => `200` (corrective module reachable in <=2 clicks).
- AC-2: PASS — evidence: `curl -sS -o /tmp/ultron-qa-reject.out -w '%{http_code}' -X POST http://127.0.0.1:8091/api/system/restart --data 'csrf_token=bogus'` => `401`; authenticated `curl -sS -b /tmp/ultron-qa-normalization.cookies http://127.0.0.1:8091/history` => includes audit entries `security`, `auth_reject`, target `/api/system/restart`, details `missing session cookie`.

## Summary
Total: 2 | Passed: 2 | Failed: 0
Decision: PASS
