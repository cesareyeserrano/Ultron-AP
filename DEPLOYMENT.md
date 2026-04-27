# Deployment — Ultron-AP

Target platform: **Raspberry Pi (ARM64)** — Raspberry Pi OS 64-bit or Debian ARM64.
Runtime model: two systemd services — unprivileged web process (`ultron-ap`) and root-owned privileged helper (`ultron-helper`).

---

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| Go 1.25+ | Build machine only — not needed on the Pi |
| Raspberry Pi OS 64-bit / Debian ARM64 | Tested on RPi 4/5 |
| `systemd` | Ships with Raspberry Pi OS |
| `tailscale` CLI | Optional — required for VPN panel (FR-014) |
| `pironman5` CLI | Optional — required for Pironman 5 panel (FR-013) |

---

## 1. Build

Cross-compile from any host with Go installed:

```bash
make build-arm
# Outputs:
#   bin/ultron-ap-linux-arm64       — web server
#   bin/ultron-helper-linux-arm64   — privileged helper
```

CSS is compiled separately and committed to the repo — no Tailwind CLI needed on the Pi.

---

## 2. Transfer to the Pi

```bash
scp bin/ultron-ap-linux-arm64     pi@raspberry:/tmp/ultron-ap
scp bin/ultron-helper-linux-arm64 pi@raspberry:/tmp/ultron-helper
```

---

## 3. Install

Run the following on the Pi:

```bash
# Service user
sudo useradd --system --no-create-home --shell /usr/sbin/nologin ultron

# Deploy binaries
sudo install -d /opt/ultron-ap
sudo install -m 0755 /tmp/ultron-ap     /opt/ultron-ap/ultron-ap
sudo install -m 0755 /tmp/ultron-helper /opt/ultron-ap/ultron-helper
sudo chown root:root /opt/ultron-ap/ultron-helper   # helper must be root-owned

# Data and config directories
sudo install -d -o ultron -g ultron -m 0750 /var/lib/ultron-ap
sudo install -d -o root   -g root   -m 0755 /etc/ultron-ap
```

---

## 4. Configure

Generate a bcrypt-hashed password (do this on any machine with Python):

```bash
python3 -c "import bcrypt; print(bcrypt.hashpw(b'your-password', bcrypt.gensalt(10)).decode())"
```

Create the environment file on the Pi:

```bash
sudo tee /etc/ultron-ap/ultron-ap.env <<'EOF'
ULTRON_PORT=8080
ULTRON_DB_PATH=/var/lib/ultron-ap/ultron.db
ULTRON_ADMIN_PASS=<bcrypt-hash-here>
EOF

sudo chmod 0600 /etc/ultron-ap/ultron-ap.env
sudo chown root:root /etc/ultron-ap/ultron-ap.env
```

Optional — add notification variables to the same file:

```bash
# Telegram (FR-005)
ULTRON_TELEGRAM_TOKEN=<bot-token>
ULTRON_TELEGRAM_CHAT_ID=<chat-id>

# Email (FR-006)
ULTRON_SMTP_HOST=smtp.example.com
ULTRON_SMTP_PORT=587
ULTRON_SMTP_USER=alerts@example.com
ULTRON_SMTP_PASS=<smtp-password>
ULTRON_SMTP_FROM=alerts@example.com
```

---

## 5. Install systemd units and sudoers

```bash
sudo install -m 0644 deploy/ultron-ap.service     /etc/systemd/system/
sudo install -m 0644 deploy/ultron-helper.service /etc/systemd/system/
sudo install -m 0440 deploy/ultron-ap.sudoers     /etc/sudoers.d/ultron-ap
sudo visudo -c    # verify sudoers syntax — fix before proceeding if error

sudo systemctl daemon-reload
sudo systemctl enable --now ultron-helper.service
sudo systemctl enable --now ultron-ap.service
```

---

## 6. Health Check

```bash
curl -s http://localhost:8080/health
# Expected: {"status":"ok"}
```

---

## 7. Logs

```bash
journalctl -u ultron-ap     -f   # web process
journalctl -u ultron-helper -f   # privileged helper
```

---

## 8. Upgrade

```bash
# On build machine
make build-arm
scp bin/ultron-ap-linux-arm64     pi@raspberry:/tmp/ultron-ap
scp bin/ultron-helper-linux-arm64 pi@raspberry:/tmp/ultron-helper

# On the Pi
sudo systemctl stop ultron-ap ultron-helper

# Keep a rollback copy
sudo cp /opt/ultron-ap/ultron-ap     /opt/ultron-ap/ultron-ap.prev
sudo cp /opt/ultron-ap/ultron-helper /opt/ultron-ap/ultron-helper.prev

sudo install -m 0755 /tmp/ultron-ap     /opt/ultron-ap/ultron-ap
sudo install -m 0755 /tmp/ultron-helper /opt/ultron-ap/ultron-helper
sudo chown root:root /opt/ultron-ap/ultron-helper

sudo systemctl start ultron-helper ultron-ap
curl -s http://localhost:8080/health
```

---

## 9. Rollback

```bash
sudo systemctl stop ultron-ap ultron-helper

sudo cp /opt/ultron-ap/ultron-ap.prev     /opt/ultron-ap/ultron-ap
sudo cp /opt/ultron-ap/ultron-helper.prev /opt/ultron-ap/ultron-helper
sudo chown root:root /opt/ultron-ap/ultron-helper

sudo systemctl start ultron-helper ultron-ap
curl -s http://localhost:8080/health
```

### Database rollback

```bash
sudo systemctl stop ultron-ap
sudo cp /var/lib/ultron-ap/ultron.db /var/lib/ultron-ap/ultron.db.$(date +%Y%m%d-%H%M%S)
sudo cp /path/to/backup.db /var/lib/ultron-ap/ultron.db
sudo chown ultron:ultron /var/lib/ultron-ap/ultron.db
sudo systemctl start ultron-ap
```

---

## 10. Environment Variable Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ULTRON_ADMIN_PASS` | **Yes** | — | bcrypt-hashed admin password |
| `ULTRON_PORT` | No | `8080` | HTTP listen port |
| `ULTRON_DB_PATH` | No | `./ultron.db` | SQLite database path |
| `ULTRON_HELPER_SOCKET` | No | `/run/ultron-helper.sock` | Helper Unix socket path |
| `ULTRON_TELEGRAM_TOKEN` | No | — | Telegram bot token (FR-005) |
| `ULTRON_TELEGRAM_CHAT_ID` | No | — | Telegram chat ID (FR-005) |
| `ULTRON_SMTP_HOST` | No | — | SMTP hostname (FR-006) |
| `ULTRON_SMTP_PORT` | No | `587` | SMTP port (FR-006) |
| `ULTRON_SMTP_USER` | No | — | SMTP username (FR-006) |
| `ULTRON_SMTP_PASS` | No | — | SMTP password (FR-006) |
| `ULTRON_SMTP_FROM` | No | — | Sender address (FR-006) |
| `ULTRON_BACKUP_KEY` | Conditional | — | AES-256 key for encrypted backups; required when `BackupConfig.encrypt_enabled=1` and `encryption_key_ref="env:ULTRON_BACKUP_KEY"` |

---

## 11. Security Checklist

- [ ] `ULTRON_ADMIN_PASS` is a bcrypt hash — never plaintext
- [ ] `/etc/ultron-ap/ultron-ap.env` is `chmod 0600`, owned `root:root`
- [ ] `ultron-helper` binary is owned `root:root`, mode `0755`
- [ ] Port `8080` is firewalled or accessible only from trusted network
- [ ] HTTPS terminated at reverse proxy (nginx/Caddy) if exposed externally
- [ ] `deploy/raspberry-stable/start.sh` is **not** used in production

---

## 12. CI

`.github/workflows/security-gate.yml` runs on every push and pull request to `main`:
- `go test ./...` — full unit + integration suite
- `govulncheck ./...` — dependency vulnerability scan
