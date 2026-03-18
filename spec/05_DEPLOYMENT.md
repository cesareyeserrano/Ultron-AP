# 05 — Deployment

**Project:** Ultron-AP
**Target platform:** Raspberry Pi (ARM64, Raspberry Pi OS / Debian)
**Runtime model:** Two systemd services — unprivileged web process (`ultron-ap`) + root-owned privileged helper (`ultron-helper`). The web process communicates with the helper over a Unix socket; no web handler may invoke privileged commands directly.

---

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| Raspberry Pi OS (64-bit) or Debian ARM64 | Tested on RPi 4/5 |
| Go 1.22+ (build only) | Not required on the target host |
| `systemd` | Unit files ship with the project |
| `tailscale` (optional) | Required only for VPN dashboard panel |
| `pironman5` CLI (optional) | Required only for Pironman 5 hardware panel |

---

## Environment Variables

All configuration is loaded via environment variables at startup. The systemd unit reads them from `/etc/ultron-ap/ultron-ap.env`.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ULTRON_PORT` | No | `8080` | HTTP listen port |
| `ULTRON_DB_PATH` | No | `./ultron.db` | Absolute path to SQLite database file |
| `ULTRON_ADMIN_PASS` | **Yes** | — | Bcrypt-hashed admin password (see Password Setup below) |
| `ULTRON_HELPER_SOCKET` | No | `/run/ultron-helper.sock` | Unix socket path for privileged helper |
| `ULTRON_SMTP_HOST` | No | — | SMTP host for email notifications |
| `ULTRON_SMTP_PORT` | No | `587` | SMTP port |
| `ULTRON_SMTP_USER` | No | — | SMTP username |
| `ULTRON_SMTP_PASS` | No | — | SMTP password |
| `ULTRON_SMTP_FROM` | No | — | Sender address for email alerts |
| `ULTRON_TELEGRAM_TOKEN` | No | — | Telegram bot token |
| `ULTRON_TELEGRAM_CHAT_ID` | No | — | Telegram chat ID |

### Password Setup

`ULTRON_ADMIN_PASS` must be a bcrypt hash, **never a plaintext password**. Generate with:

```bash
htpasswd -bnBC 10 "" <your-password> | tr -d ':\n'
# or
python3 -c "import bcrypt; print(bcrypt.hashpw(b'<your-password>', bcrypt.gensalt(10)).decode())"
```

> **Warning:** `deploy/raspberry-stable/start.sh` is a development convenience script that sets `ULTRON_ADMIN_PASS="admin123"`. This is not suitable for production use. Use the systemd unit with a proper env file instead.

---

## Build

Cross-compile for ARM64 from any host:

```bash
make build-arm64
# Output: bin/ultron-ap-linux-arm64  (web server)
#         bin/ultron-helper-linux-arm64  (privileged helper)
```

Or use the native host build:

```bash
make build
```

---

## Installation

### 1. Create the service user

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin ultron
```

### 2. Deploy the binaries

```bash
sudo install -d /opt/ultron-ap
sudo install -m 0755 bin/ultron-ap-linux-arm64      /opt/ultron-ap/ultron-ap
sudo install -m 0755 bin/ultron-helper-linux-arm64  /opt/ultron-ap/ultron-helper
sudo chown root:root /opt/ultron-ap/ultron-helper
```

### 3. Prepare the data directory

```bash
sudo install -d -o ultron -g ultron -m 0750 /var/lib/ultron-ap
sudo install -d -o root   -g root   -m 0755 /etc/ultron-ap
```

### 4. Create the environment file

```bash
sudo tee /etc/ultron-ap/ultron-ap.env <<'EOF'
ULTRON_PORT=8080
ULTRON_DB_PATH=/var/lib/ultron-ap/ultron.db
ULTRON_ADMIN_PASS=<bcrypt-hash-here>
EOF
sudo chmod 0600 /etc/ultron-ap/ultron-ap.env
sudo chown root:root /etc/ultron-ap/ultron-ap.env
```

### 5. Install the systemd units

```bash
sudo install -m 0644 deploy/ultron-ap.service     /etc/systemd/system/
sudo install -m 0644 deploy/ultron-helper.service /etc/systemd/system/
sudo systemctl daemon-reload
```

### 6. Install the sudoers profile (if not using the helper socket)

```bash
sudo install -m 0440 deploy/ultron-ap.sudoers /etc/sudoers.d/ultron-ap
sudo visudo -c  # verify no syntax errors
```

### 7. Enable and start

```bash
sudo systemctl enable --now ultron-helper.service
sudo systemctl enable --now ultron-ap.service
sudo systemctl status ultron-ap ultron-helper
```

---

## Privilege Separation

| Component | User | Capabilities | Communication |
|-----------|------|-------------|---------------|
| `ultron-ap` | `ultron` (unprivileged) | `NoNewPrivileges`, `ProtectSystem=full` | Listens on HTTP; talks to helper via Unix socket |
| `ultron-helper` | `root` | Restricted via `ProtectHome`, `ProtectKernelTunables`, etc. | Receives commands over `/run/ultron-helper.sock` |

The helper socket group is set to `ultron` so only the web service process can connect. No web handler may call privileged commands directly — all host-level actions are proxied through the helper.

---

## Health Check

```bash
curl -s http://localhost:8080/health
# {"status":"ok"}
```

---

## Logs

```bash
journalctl -u ultron-ap     -f   # web process
journalctl -u ultron-helper -f   # privileged helper
```

---

## Security Checklist (pre-production)

- [ ] `ULTRON_ADMIN_PASS` is a bcrypt hash, not plaintext
- [ ] `/etc/ultron-ap/ultron-ap.env` is mode `0600`, owned by `root`
- [ ] `ultron-helper` binary is owned `root:root`, mode `0755`
- [ ] Firewall restricts port `8080` to trusted network (or reverse-proxy with TLS)
- [ ] HTTPS is terminated at a reverse proxy (nginx/Caddy) in front of Ultron
- [ ] `deploy/raspberry-stable/start.sh` is **not** used in production

---

## Raspberry Pi Stable Release

Pre-compiled binaries for the current stable deployment target are available under `deploy/raspberry-stable/` (not tracked in version control). To produce them:

```bash
make build-arm64
cp bin/ultron-ap-linux-arm64 deploy/raspberry-stable/ultron-ap
```

The `start.sh` convenience script in that directory is for ad-hoc development restarts only.
