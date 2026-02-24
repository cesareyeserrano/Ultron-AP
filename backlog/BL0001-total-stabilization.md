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
- [x] Surface and track automated backup failures in scheduler loop.
  - Current gap: `startBackupJob()` calls `performAutomatedBackup()` without handling returned error.
  - Done when:
    - Backup loop logs structured success/failure outcome per run.
    - Failure reason is visible in action history or explicit persisted signal.
  - Evidence:
    - `internal/server/server.go`: `recordBackupOutcome(...)` in scheduler loop.
    - `internal/server/server_test.go`: `TestRecordBackupOutcome_LogsAction`.

- [x] Add regression tests for backup retention in error paths.
  - Current gap: retention behavior is now hardened but lacks direct test coverage for "Telegram disabled/failure" branches.
  - Done when:
    - Tests prove local backup retention still runs when Telegram send is skipped or fails.
  - Evidence:
    - `internal/server/server_test.go`: `TestPerformAutomatedBackup_RetentionRunsOnTelegramError`.

### P1
- [x] Add unit tests for Pironman parser compatibility mode (`bool` vs `"on"/"off"`).
  - Current gap: `parseBoolOrString` has no direct tests.
  - Done when:
    - Table-driven tests cover `true`, `false`, `"on"`, `"off"`, `"1"`, malformed payloads.
  - Evidence:
    - `internal/pironman/controls_test.go`: `TestParseBoolOrString`.

- [x] Add tests for hardware apply payload semantics.
  - Current gap: no targeted tests for checkbox-off behavior (`rgb_enable`/`oled_enable` absent in form).
  - Done when:
    - Handler tests confirm unchecked toggles are interpreted as `false`.
    - CSRF rejection/acceptance paths for hardware endpoint are covered.
  - Evidence:
    - `internal/server/handlers_hardware_test.go`.

- [x] Add tests for session cookie policy under HTTPS proxy mode.
  - Current gap: new `Secure` behavior via `TLS` or `X-Forwarded-Proto` is not directly asserted.
  - Done when:
    - Tests verify cookie flags for HTTP and HTTPS/proxied requests.
  - Evidence:
    - `internal/server/handlers_auth_test.go`: `TestLogin_SetsSecureCookie_WhenForwardedProtoHTTPS`, `TestLogin_SetsSecureCookie_WhenTLS`.

### P2
- [x] Add bounded cleanup strategy for brute-force tracker memory.
  - Current gap: map entries are only pruned for IPs rechecked; stale addresses can accumulate over long runtimes.
  - Done when:
    - Background cleanup or bounded map policy exists.
    - Load test or unit test validates bounded growth.
  - Evidence:
    - Brute-force cleanup path implemented and exercised by tests in `internal/auth/bruteforce_test.go`.

- [x] Add defense-in-depth CSRF origin checks for state-changing endpoints.
  - Current gap: token validation exists, but Origin/Referer validation is not enforced.
  - Done when:
    - Unsafe methods verify same-origin (with reverse-proxy-aware rules).
    - Tests cover allow/deny cases.
  - Evidence:
    - `internal/server/handlers_settings.go`: `isSameOriginRequest(...)`.
    - `internal/server/handlers_settings_test.go`: origin allow/deny coverage.

## Exit Criteria for "Total Stabilization"
- [x] `go test ./...` green.
- [x] New stabilization tests merged for all P0/P1 items.
- [x] No open P0 items.
- [x] Residual P2 items explicitly accepted or scheduled.
