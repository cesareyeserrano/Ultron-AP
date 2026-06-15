# Deployment — sustained-alert-window-fix

## Summary
This feature is an internal correctness fix to the alert engine's sustained-breach
window (`internal/alerts/engine.go`). **There is no deployment-surface change**: no
new binary, service, port, environment variable, database migration, or config.
It ships through the existing Raspberry Pi systemd + static-binary path (the project's
declared deployment model — **not** Docker).

## Build & deploy (existing path, unchanged)
```sh
# 1. Build the ARM64 binaries (also builds the privileged helper)
make build-arm

# 2. Verify the local build differs from what's running on the Pi
make deploy-verify

# 3. Copy the new binary to the Pi and restart the existing service
#    (same procedure as any prior release — see the repository's main
#    deploy/ docs; the unit file and helper socket are unchanged)
sudo systemctl restart ultron-ap
```

## Health check
- Process liveness: `GET /health` returns 200 when the web process is alive
  (unchanged by this feature).
- Functional check: configure (or keep) a host metric rule with a sustained
  duration (e.g. CPU > X% for 60s). After the fix, the rule fires once the
  breach has genuinely persisted for the configured duration even when metric
  sampling is not perfectly interval-aligned. Before the fix it would silently
  never fire under sampling jitter.

## Rollback
The change is a single binary. To roll back, redeploy the previous
`ultron-ap-linux-arm64` binary and `systemctl restart ultron-ap`. No data,
schema, or config migration is involved, so rollback is immediate and lossless.
The only behavioral effect of rolling back is reverting to the old (buggy)
sustained-window logic, where sustained alert rules may not fire under jitter.

## CI
`.github/workflows/security-gate.yml` already runs `go test ./...` (including the
new `internal/alerts/sustained_window_aitri_test.go` cases) and `govulncheck`
on every push to `main` and on pull requests — satisfying the CI/CD requirement
(NFR-010). No new workflow is added.
