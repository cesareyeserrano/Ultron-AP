# Deployment — network-alerts

`network-alerts` ships inside the existing Ultron-AP binary. The canonical production target remains Raspberry Pi ARM64 with the parent `deploy/` systemd units. The Docker files in this feature directory are auxiliary packaging artifacts for isolated validation; they do not replace the systemd deployment path.

## Environment Variables

| Name | Type | Required | Example | Notes |
|---|---|---:|---|---|
| `ULTRON_PORT` | integer | optional | `8080` | HTTP listen port. |
| `ULTRON_DB_PATH` | path | optional | `/var/lib/ultron-ap/ultron.db` | SQLite database path. |
| `ULTRON_ADMIN_PASS` | bcrypt hash | required for first boot | `$2a$10$...` | Parent admin password hash. |
| `ULTRON_SECRET_KEY` | string | recommended | `replace-with-32-byte-random-secret` | Parent session/secret key. |
| `ULTRON_NET_TARGETS` | CSV | optional | `gateway,1.1.1.1=1.1.1.1,8.8.8.8=8.8.8.8,dns:dns=1.1.1.1/cloudflare.com` | Network targets used by probes and alert target validation. |

No new feature-specific environment variables are required.

## Production Deploy — Raspberry Pi systemd

1. Build the ARM64 binaries from the repo root:

```bash
make build-arm
```

2. Install using the parent deployment procedure in [DEPLOYMENT.md](../../DEPLOYMENT.md). Keep both services enabled:

```bash
sudo systemctl enable --now ultron-helper.service
sudo systemctl enable --now ultron-ap.service
```

3. Confirm the application starts and migrations complete:

```bash
journalctl -u ultron-ap -n 100 --no-pager
```

Expected signal: no migration errors and `Alert engine started`.

4. Health check:

```bash
curl -s http://localhost:8080/health
```

Expected response:

```json
{"status":"ok"}
```

## Local Compose Validation

Compose is optional and not the Raspberry Pi production path.

```bash
cp features/network-alerts/.env.example features/network-alerts/.env
docker compose -f features/network-alerts/docker-compose.yml --env-file features/network-alerts/.env up --build
```

Health check:

```bash
docker compose -f features/network-alerts/docker-compose.yml ps
curl -s http://localhost:8080/health
```

## Rollback

1. Stop the service:

```bash
sudo systemctl stop ultron-ap.service
```

2. Restore the previous binary:

```bash
sudo install -m 0755 /opt/ultron-ap/releases/previous/ultron-ap /opt/ultron-ap/ultron-ap
```

3. Start and verify:

```bash
sudo systemctl start ultron-ap.service
curl -s http://localhost:8080/health
```

The `AlertConfig.target` and `AlertConfig.sustained_duration` columns are forward-compatible with older binaries because existing readers use explicit column lists and ignore extra columns.

## Operational Checks

- Confirm network alert rules can be created under `/settings#settings-alerts`.
- Confirm logs include structured network evaluation lines:

```bash
journalctl -u ultron-ap --since "10 min ago" | grep 'metric=latency'
```

- Confirm disabled rules do not emit alerts by toggling a rule off and observing no new `Alert` row for that metric.

## Verification Evidence

Feature verification command:

```bash
aitri feature verify-run network-alerts
```

Current result: `24/24` tests passing, `0` failed, `0` skipped.
