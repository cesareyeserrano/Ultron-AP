# BL0001: Total Stabilization Backlog

> Source: Aitri session (`aitri init` + `aitri resume` + `aitri status`)
> Date: 2026-02-24
> Objective: Close remaining quality gaps after hardening pass.

## Scope
- Backend reliability
- Security hardening
- Test coverage for high-risk paths
- Operational observability

## Pending Adjustments

### P0
- [ ] Surface and track automated backup failures in scheduler loop.
  - Current gap: `startBackupJob()` calls `performAutomatedBackup()` without handling returned error.
  - Done when:
    - Backup loop logs structured success/failure outcome per run.
    - Failure reason is visible in action history or explicit persisted signal.

- [ ] Add regression tests for backup retention in error paths.
  - Current gap: retention behavior is now hardened but lacks direct test coverage for "Telegram disabled/failure" branches.
  - Done when:
    - Tests prove local backup retention still runs when Telegram send is skipped or fails.

### P1
- [ ] Add unit tests for Pironman parser compatibility mode (`bool` vs `"on"/"off"`).
  - Current gap: `parseBoolOrString` has no direct tests.
  - Done when:
    - Table-driven tests cover `true`, `false`, `"on"`, `"off"`, `"1"`, malformed payloads.

- [ ] Add tests for hardware apply payload semantics.
  - Current gap: no targeted tests for checkbox-off behavior (`rgb_enable`/`oled_enable` absent in form).
  - Done when:
    - Handler tests confirm unchecked toggles are interpreted as `false`.
    - CSRF rejection/acceptance paths for hardware endpoint are covered.

- [ ] Add tests for session cookie policy under HTTPS proxy mode.
  - Current gap: new `Secure` behavior via `TLS` or `X-Forwarded-Proto` is not directly asserted.
  - Done when:
    - Tests verify cookie flags for HTTP and HTTPS/proxied requests.

### P2
- [ ] Add bounded cleanup strategy for brute-force tracker memory.
  - Current gap: map entries are only pruned for IPs rechecked; stale addresses can accumulate over long runtimes.
  - Done when:
    - Background cleanup or bounded map policy exists.
    - Load test or unit test validates bounded growth.

- [ ] Add defense-in-depth CSRF origin checks for state-changing endpoints.
  - Current gap: token validation exists, but Origin/Referer validation is not enforced.
  - Done when:
    - Unsafe methods verify same-origin (with reverse-proxy-aware rules).
    - Tests cover allow/deny cases.

## Exit Criteria for "Total Stabilization"
- [ ] `go test ./...` green.
- [ ] New stabilization tests merged for all P0/P1 items.
- [ ] No open P0 items.
- [ ] Residual P2 items explicitly accepted or scheduled.
