# Insights Engine — Deployment

## Deployment model

This feature is **not a standalone service**. It is a Go package set added
under `internal/insights/*` plus one HTTP handler in `internal/server/`,
and an inline call from the SSE broadcast loop. Its lifecycle is the
parent's lifecycle: same binary, same `systemd` unit, same SQLite file,
same HTTP server, same auth middleware, same backup pipeline.

There is no Dockerfile, no `docker-compose.yml`, and no `.env.example` for
this feature. The parent project (`/opt/ultron-ap` on the Pi) defines those
surfaces — see `deploy/ultron-ap.service`, `deploy/ultron-helper.service`,
and `deploy/ultron-ap.sudoers` at the repo root. A previous project-level
Phase 5 attempt added container artefacts and was rejected (see project
`spec/01_REQUIREMENTS.json` rejection history, 2026-03-18): the deployment
target is **Raspberry Pi via systemd**, not Docker.

## Prerequisites (host)

- Raspberry Pi running the existing Ultron-AP installation. Pi 4/5 with
  arm64.
- `go 1.22+` on the build host. Cross-compilation via `make build-arm`.
- No new kernel sysctls, no new capabilities, no new helper allow-list
  entries. NFR-021 requires `internal/insights` to never reach into
  `internal/notify` or `internal/alerts`; the architecture enforces that
  by package boundary (see `go list -deps ./internal/insights/...`).

## Database migrations

Two new tables — `rules` and `rule_state` — are appended to the parent's
`schema` constant in `internal/database/sqlite.go`. The existing
`database.New()` schema-init path picks them up on first start after
deploy.

```sql
CREATE TABLE IF NOT EXISTS rules (
  id              TEXT    PRIMARY KEY,
  title           TEXT    NOT NULL,
  condition_json  TEXT    NOT NULL,
  severity        TEXT    NOT NULL CHECK (severity IN ('info','warn','critical')),
  verdict         TEXT    NOT NULL,
  recommendation  TEXT    NOT NULL,
  links_json      TEXT    NOT NULL DEFAULT '[]',
  enabled         INTEGER NOT NULL DEFAULT 1,
  source          TEXT    NOT NULL DEFAULT 'bundled' CHECK (source IN ('bundled','user')),
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rules_enabled_severity ON rules(enabled, severity);
CREATE TABLE IF NOT EXISTS rule_state (
  rule_id                TEXT    PRIMARY KEY,
  last_evaluated_at      INTEGER NOT NULL,
  last_value             INTEGER NOT NULL,
  last_change_at         INTEGER NOT NULL,
  transitions_in_window  INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (rule_id) REFERENCES rules(id)
);
```

On first boot after upgrade, `Service.LoadBundled()` upserts the bundled
rule set (`internal/insights/rules/bundled.json`, embedded via go:embed)
into the `rules` table with `source='bundled'`. Existing user-modified
`enabled` flags survive across restarts (FR-045 AC-002).

## Build & deploy

```sh
# On the build host (from repo root):
make build-arm                                  # → bin/ultron-ap-linux-arm64

# Test before shipping:
go test ./...

# Copy and restart on the Pi:
scp bin/ultron-ap-linux-arm64 cesareyeserrano@192.168.1.29:/tmp/ultron-ap-new
ssh cesareyeserrano@192.168.1.29 \
  'sudo cp /opt/ultron-ap/ultron-ap /opt/ultron-ap/ultron-ap.previous \
   && sudo install -m0755 /tmp/ultron-ap-new /opt/ultron-ap/ultron-ap \
   && sudo systemctl restart ultron-ap.service'
```

The helper unit (`ultron-helper.service`) does NOT need to be rebuilt or
restarted — this feature does not touch the helper allow-list grammar or
add any new privileged path.

## Configuration

This feature introduces **no new environment variables in v1**. Defaults
(per `internal/insights/insights.go`):

| Setting              | Default | Note |
|----------------------|---------|------|
| Eval cadence         | parent SSE interval (5 s) | Inherits from FR-039 / parent FR-013 |
| Hysteresis window    | 10 s | FR-046 — flap quench |
| Hysteresis threshold | 5 transitions | FR-046 |

Future configurability is out of scope for v1.

## Health checks

Inherits the parent's `/health` endpoint. The insights engine also
exposes `GET /api/insights/verdicts` returning the live verdict array.

```sh
ssh cesareyeserrano@192.168.1.29 'systemctl is-active ultron-ap \
  && curl -fsS http://127.0.0.1:8080/health \
  && curl -fsS -b "session=$ULTRON_SESSION" http://127.0.0.1:8080/api/insights/verdicts'
```

A populated array (or an empty array on a healthy idle Pi) confirms the
engine is evaluating. The dashboard's "Operational Indicators" section
will render automatically via SSE — verify by loading `/` in a browser
and watching the section populate or stay clean.

## Rollback

Forward-compatible only at the schema level — an older binary will simply
ignore the `rules` and `rule_state` tables created by the new binary.

```sh
ssh cesareyeserrano@192.168.1.29 \
  'sudo systemctl stop ultron-ap.service \
   && sudo install -m0755 /opt/ultron-ap/ultron-ap.previous /opt/ultron-ap/ultron-ap \
   && sudo systemctl start ultron-ap.service'
```

## Failure modes & runbook

| Symptom                                            | Likely cause                                       | Remedy |
|----------------------------------------------------|----------------------------------------------------|--------|
| Operational Indicators section never populates     | SSE broker not emitting `verdicts` event           | `journalctl -u ultron-ap` for `insights:` errors. SSE event only fires when the active set CHANGES — an idle Pi with zero rules firing is the expected steady state. |
| Same verdict flapping on the dashboard             | Hysteresis window misconfigured (FR-046)           | Verify `transitions_in_window` in `rule_state` table; should be ≤5 in 10 s before fire is suppressed. |
| Bundled rule fires that should not                 | Telemetry var the rule depends on is misnamed      | Compare rule's `condition_json` var names against `internal/server/sse.go::evalInsightsTick` projection. Missing vars resolve to false (FR-041 AC-002), so a misspelled var causes the rule to be permanently silent rather than firing. |
| `internal/insights` imports `notify` or `alerts`   | NFR-021 boundary violation (build-time check)      | `go list -deps ./internal/insights/... \| grep -E 'alerts\|notify'` MUST be empty. If it is not, revert the offending change — the boundary is architectural. |

## CI/CD

The parent project's existing `.github/workflows/security-gate.yml` runs
`go test ./...` and `govulncheck` on every push/PR — both apply to this
feature unchanged. No new workflow needed.

The NFR-021 boundary check (no `alerts`/`notify` imports under
`internal/insights/`) is captured by **TC-IE-010e** in the test suite,
which fails the build if violated. Re-checking in CI is therefore
automatic via the existing `go test ./...` step.
