# Pi Live Validation Report (Ultron)

Date: 2026-02-24
Host: `192.168.1.29` (`Ultron`)
Scope: post-remediation runtime verification while user performs manual UI checks.

## Summary
- Service status: PASS
- Health endpoint: PASS (`200` + `{"status":"ok"}`)
- Security posture: PASS (`NoNewPrivileges=yes` in both units)
- Privileged boundary: PASS (root helper socket active at `/run/ultron-helper.sock`, owned `root:ultron`)
- Runtime logs: PASS after hotfix (no new `sudo` execution errors, no new helper timeout errors in observation window)

## Commands and Results
1. Baseline host/runtime
- `date -Iseconds; hostname; uname -a`
- Result: host online, Linux `6.12.62+rpt-rpi-2712`, arm64.

2. Service and hardening status
- `systemctl is-active ultron-helper ultron-ap`
- `systemctl show ultron-helper -p NoNewPrivileges`
- `systemctl show ultron-ap -p NoNewPrivileges`
- `ls -l /run/ultron-helper.sock`
- Result: both `active`; both `NoNewPrivileges=yes`; socket exists `srw-rw---- root ultron`.

3. API health
- `curl http://127.0.0.1:8080/health`
- Result: HTTP `200`, body `{"status":"ok"}`.

4. Journal audit
- `journalctl -u ultron-ap -n 120`
- `journalctl -u ultron-helper -n 80`
- Result: one runtime issue detected during test:
  - `hardware: apply config: pironman5 apply via helper: decode helper response: ... i/o timeout`

## Incident Found and Remediation Applied
Issue:
- IPC timeout in hardware apply path due client socket deadline being fixed at helper default timeout instead of request context deadline.

Root cause:
- `internal/privileged/client.go` set connection deadline from `Client.timeout` even when caller context allowed longer operation.

Fix implemented:
- `internal/privileged/client.go`: effective timeout now derived from context deadline when present.
- `cmd/ultron-helper/main.go`: per-connection deadline increased to 90s.
- Rebuild/redeploy of `ultron-ap` and `ultron-helper` binaries on Pi.

Post-fix validation:
- Services restarted cleanly and remained `active`.
- `/health` remained `200`.
- Live monitoring (`journalctl -f` on both units for ~90s) showed no new errors.

## Notes
- The earlier `sudo`/`NoNewPrivileges` failures are historical (pre-helper design) and were not reproduced after current deployment.
- Manual end-to-end UI behavior still depends on active interactive testing (in progress by operator).
