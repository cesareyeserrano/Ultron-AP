# Help Page — Deployment

## Deployment model

This feature is **not a standalone service**. It is a Go package added under
`internal/help/*` (with a tiny shared-types peer at `internal/help/contract/`),
one new HTTP handler in `internal/server/handlers_help.go`, plus three small
template / partial / sidebar edits and a public accessor on the existing
insights service. Its lifecycle is the parent's lifecycle: same binary, same
`systemd` unit, same SQLite file, same HTTP server, same auth middleware,
same backup pipeline.

There is no Dockerfile, no `docker-compose.yml`, and no `.env.example` for
this feature. The parent project (`/opt/ultron-ap` on the Pi) defines those
surfaces — see `deploy/ultron-ap.service`, `deploy/ultron-helper.service`,
and `deploy/ultron-ap.sudoers` at the repo root. A previous project-level
Phase 5 attempt added container artefacts and was rejected (see project
`spec/01_REQUIREMENTS.json` rejection history, 2026-03-18): the deployment
target is **Raspberry Pi via systemd**, not Docker. The insights-engine
feature established the same precedent for its Phase 5; this feature
follows it.

## Prerequisites (host)

- Raspberry Pi 4/5 (linux/arm64) running the existing Ultron-AP installation.
- `go 1.25.7` on the build host (cross-compilation via `make build-arm` or
  `GOOS=linux GOARCH=arm64 go build ./cmd/ultron-ap`).
- No new kernel sysctls, no new capabilities, no new helper allow-list
  entries.
- No new database tables, no migrations. Glossary content is bundled into
  the binary via `go:embed` (NFR-023). The parent backup pipeline is
  unaffected.

## Architectural boundary (NFR-026)

`internal/help/` MUST NOT import `internal/insights`, `internal/alerts`, or
`internal/notify`. The boundary is enforced by:

1. The shared-types peer package `internal/help/contract/` (zero behaviour,
   plain data only) which both `internal/insights` and `internal/help`
   import. Neither imports the other directly.
2. The unit test `TestTC_HP_NFR_026h` shells out to `go list -deps
   ./internal/help/...` and fails the suite if any forbidden dependency
   appears.
3. The unit test `TestTC_HP_NFR_026e` parses every production .go file
   under `internal/help/` and fails if any import path contains
   `/internal/alerts`, `/internal/notify`, `telegram`, or `smtp`.

A future contributor who adds a forbidden import will see the test fail
before the change can land.

## Build

```sh
make build-arm                 # cross-compile to bin/ultron-ap (linux/arm64)
# OR
GOOS=linux GOARCH=arm64 go build -o bin/ultron-ap ./cmd/ultron-ap
```

Verification that `go:embed` worked — quoted-printable hex of the embedded
glossary's first bytes should appear in the binary:

```sh
strings bin/ultron-ap | grep -F '"version": 1' | head -3
```

## Deploy

```sh
scp bin/ultron-ap pi@<host>:/tmp/ultron-ap
ssh pi@<host> 'sudo install -m 0755 /tmp/ultron-ap /opt/ultron-ap/ultron-ap'
ssh pi@<host> 'sudo systemctl restart ultron-ap.service'
```

## Smoke test (post-deploy)

```sh
# 1. Service is up.
sudo systemctl status ultron-ap.service

# 2. Boot log line confirms glossary loaded with the expected entry count.
journalctl -u ultron-ap -n 200 --no-pager | grep glossary-loaded
# expected: ... event=glossary-loaded entries=33  (or higher)

# 3. Authenticated GET /help returns 200 and contains the five category ids.
curl -sfk -b session=$(<your-session-cookie) https://<host>/help \
  | grep -cF 'data-category="' \
# expected: 5

# 4. The Help nav item is rendered on the dashboard.
curl -sfk -b session=$(<your-session-cookie) https://<host>/ \
  | grep -F 'href="/help"'
# expected: one match.

# 5. Verdict cards' Learn-more anchor (only fires when a verdict is active).
curl -sfk -b session=$(<your-session-cookie) https://<host>/insights/fragment \
  | grep -F 'Learn more'
# expected: one or more matches when a rule has fired with a valid #anchor.
```

## Rollback

```sh
# Roll back to the last shipped binary (adjust to your retention scheme).
ssh pi@<host> 'sudo cp /opt/ultron-ap/ultron-ap.previous /opt/ultron-ap/ultron-ap'
ssh pi@<host> 'sudo systemctl restart ultron-ap.service'
```

The feature has no migrations and no on-disk state. Rolling back a binary is
sufficient — the previous version simply has no `/help` route and no Help
nav item.

## Health checks

The parent `/health` endpoint is unchanged. The help-page renderer is
intentionally **not** part of the readiness probe — its failure must not
flip the overall service health (NFR-022 — the page is informational, not
operational).

A failure of the embedded-glossary parse (which can only happen if the
binary is corrupt) is reported in two places:

1. The boot log: `event=glossary-entry-rejected ...` lines name the
   offending entry and reason.
2. `GET /help` returns 200 with a stub message ("Help unavailable — see
   server logs"). All other Ultron features remain operational.

## Observability

Three new structured log events to watch for:

| Event                          | Severity | Meaning                                                                       |
| ------------------------------ | -------- | ----------------------------------------------------------------------------- |
| `event=glossary-loaded`        | INFO     | One per boot. Carries `entries=N` so you know how many entries are in scope.  |
| `event=glossary-entry-rejected`| WARN     | One per malformed entry. Names `id` (or `index`) and `reason`.                |
| `event=duplicate-entry-id`     | WARN     | Two entries shared the same `id`. First-wins; second is logged then dropped.  |
| `event=insights-link-missing`  | WARN     | A rule's `links` references a fragment with no matching glossary entry. The  rule still loads. |

Log retention follows the parent journald configuration; no new rotation
policy.

## Performance budgets (NFR-022)

| Surface                                | Budget         | Verified by                                  |
| -------------------------------------- | -------------- | -------------------------------------------- |
| `GET /help` server-side render p99     | < 500 ms       | `TestTC_HP_NFR_022h` — observed ≈ 1 ms p99 in CI |
| Filter keystroke → visual update p99   | < 100 ms       | Static reasoning over the inline JS (≤ 30 lines, single `String.indexOf` per entry, ≤ 100 entries) — see Phase 4 manifest for browser-stack debt |
| XHR/fetch after initial GET            | 0              | Filter is fully client-side; no `fetch`, no `htmx-*` in the rendered fragment |

## Configuration

Zero new environment variables. Zero new flags. Zero new config-file keys.
The feature is fully wired by adding ~6 lines to `cmd/ultron-ap/main.go`
(see Phase 4 manifest, files_modified).
