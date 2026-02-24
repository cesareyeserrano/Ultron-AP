# Ultron-AP

> **v1.0.0-stable** | Lightweight Raspberry Pi Admin Panel

Ultron-AP is a professional, high-performance monitoring and management dashboard designed specifically for the Raspberry Pi. It provides real-time visibility into system health, Docker containers, and Systemd services through a sleek, resource-efficient interface.

Built with a **zero-runtime-dependency** philosophy using Go, HTMX, and Tailwind CSS.

---

## ✨ Key Features

- **Real-Time Monitoring:** Instant visibility into CPU, RAM, Disk, Network, and CPU Temperature via Server-Sent Events (SSE).
- **Service Controls:** Start, Stop, and Restart Docker containers and Systemd services directly from the web.
- **On-Demand Logs:** View the last 100 lines of logs for any Docker container or core system service without SSH.
- **Smart Alerting:** Configurable threshold-based rules with real-time notifications via **Telegram** and Email.
- **Hardware Integration:** Native support for **Pironman 5** (RGB, Fan modes, and OLED configuration).
- **Security First:** Built-in CSRF protection, secure sessions (bcrypt), brute-force protection, and a full action audit trail.
- **Privilege Separation:** Web process runs unprivileged with `NoNewPrivileges=true`; host-level actions are executed by a root-owned local helper over Unix socket.
- **Resource Optimized:** Consumes ~15MB RAM and minimal CPU, making it ideal for background operation on any Pi model.

---

## 🚀 Quick Start

### Build from source
```bash
make build
./bin/ultron-ap
```

### Deploy to Raspberry Pi (ARM64)
1. **Cross-compile:**
   ```bash
   make build-arm
   ```
2. **Transfer:** Copy `bin/ultron-ap-linux-arm64` to your Pi.
3. **Configure:** Set the following environment variables:
   - `ULTRON_ADMIN_PASS`: Initial admin password (required on first run).
   - `ULTRON_PORT`: Default is `8080`.
   - `ULTRON_DB_PATH`: Path to SQLite database.
   - `ULTRON_HELPER_SOCKET`: Unix socket path for privileged helper (default `/run/ultron-helper.sock`).
   - `ULTRON_HELPER_TIMEOUT`: Helper RPC timeout (default `5s`).

---

## 🛠️ Tech Stack

- **Backend:** Go 1.25+ (Standard library + minimal dependencies)
- **Frontend:** HTMX (Interactivity), Tailwind CSS (Styling)
- **Real-time:** SSE (Server-Sent Events)
- **Storage:** SQLite (WAL mode enabled for high concurrency)
- **Integration:** Docker SDK, Systemd D-Bus/CLI, Pironman5 CLI

---

## 📁 Project Structure

```text
cmd/ultron-ap/          # Entry point
internal/
  alerts/               # Alert engine & rule evaluation
  auth/                 # Security, sessions & brute-force
  database/             # SQLite schema & persistence
  docker/               # Container management
  metrics/              # System resource collectors
  notify/               # Telegram & Email dispatch
  server/               # HTTP core & SSE broker
  systemd/              # OS service controls
web/
  templates/            # HTMX-powered HTML templates
  static/               # Optimized assets (CSS/JS)
```

---

## 📜 License

MIT License. Developed by [Cesar Reyes Serrano](https://github.com/cesareyeserrano).
