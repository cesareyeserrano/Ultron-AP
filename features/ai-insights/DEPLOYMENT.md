# Deployment — AI Insights feature

This feature ships **inside the existing `ultron-ap` binary** — there is no new
process, no new service, and no container. Deployment is identical to the project's
standard path (see the repo-root `DEPLOYMENT.md`); this note covers only what is
specific to the AI feature.

## Deployment model

**Single native Go binary under systemd on the Raspberry Pi (ARM64).** No Dockerfile,
no docker-compose — the AI provider is an external HTTPS endpoint, not a bundled
service (matches `02_SYSTEM_DESIGN.md → Deployment Architecture` and the project's
prior decision to stay off containers).

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| Go 1.25.11+ | Build machine only — not on the Pi |
| `ULTRON_SECRET_KEY` set on the Pi | Already provisioned. Required to encrypt the AI API key at rest (same key that protects Telegram/SMTP secrets). If unset, the AI key cannot be saved. |
| An OpenAI-compatible LLM endpoint | Any provider (Ollama Cloud, OpenAI, GLM, vLLM, LM Studio…). Configured in the panel, **not** as an env var. |

No new environment variables are introduced by this feature.

## Build & deploy

Same as the standard flow:

```bash
# On the build machine (Mac)
make build-arm          # cross-compiles ultron-ap + ultron-helper for linux/arm64

# Transfer + install on the Pi (or via the project's deploy steps)
scp bin/ultron-ap-linux-arm64 pi@raspberry:/tmp/ultron-ap.new
ssh pi@raspberry '
  sudo systemctl stop ultron-ap
  sudo cp /opt/ultron-ap/ultron-ap /opt/ultron-ap/ultron-ap.prev   # rollback copy
  sudo install -m 0755 /tmp/ultron-ap.new /opt/ultron-ap/ultron-ap
  sudo systemctl start ultron-ap
'
```

The database table `ai_settings` is created automatically on first start (additive
`CREATE TABLE IF NOT EXISTS`; no manual migration, existing tables untouched).

## Post-deploy configuration (operator, in the panel)

1. Open **Settings → AI Assistant**.
2. Enter the **Endpoint URL**, **Model**, and **API key**, toggle **Enable AI** on.
3. (Optional) Toggle **Send explanation to Telegram on alert**.
4. Click **Test connection** to verify reachability, then **Save**.

With AI left disabled (no key), the panel behaves exactly as before (FR-019).

## Health checks

```bash
curl -s http://localhost:8080/health     # {"status":"ok"} — unaffected by AI state
curl -s http://localhost:8080/version    # confirm the deployed commit
```

`/health` returns 200 even when the AI provider is misconfigured or unreachable
(NFR-007) — the AI subsystem is fail-closed and isolated.

## Rollback

```bash
sudo systemctl stop ultron-ap
sudo cp /opt/ultron-ap/ultron-ap.prev /opt/ultron-ap/ultron-ap
sudo systemctl start ultron-ap
curl -s http://localhost:8080/health
```

The `ai_settings` table is additive and harmless to a prior binary (it simply
ignores the table), so a binary rollback needs no DB change.

## CI

The feature's tests live in the root module's `internal/...` packages and run on
every push/PR via `.github/workflows/ci.yml` (`go vet`, `go test -race`, arm64
build) and `security-gate.yml` (`go test`, `govulncheck`). No feature-specific
workflow is added.
