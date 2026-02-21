# Ultron-AP

Lightweight admin panel for Raspberry Pi. Monitor Docker containers, Systemd services, and system metrics from a single dashboard — no SSH needed.

Built with **Go**, **HTMX**, **Tailwind CSS**, and **SQLite**. Runs as a single binary under 15MB, consuming less than 30MB of RAM.

## Features

- **System Metrics** — CPU, RAM, disk, network, temperature in real time via SSE
- **Docker Monitoring** — Container status, resource usage, health checks
- **Systemd Monitoring** — Service status, start/stop/restart controls
- **Alert System** — Configurable thresholds with Telegram and email notifications
- **Service Controls** — Start, stop, restart containers and services from the dashboard
- **Dark Mode UI** — Minimal, responsive interface optimized for low-resource devices
- **Single Binary** — No runtime dependencies, embed everything, deploy anywhere

## Quick Start

### Prerequisites

- Go 1.22+

### Build & Run

```bash
# Clone
git clone https://github.com/Cesareyeserrano/Ultron-AP.git
cd Ultron-AP

# Build
make build

# Run with defaults (port 8080, SQLite at /var/lib/ultron-ap/ultron.db)
./bin/ultron-ap

# Or configure via environment variables
ULTRON_PORT=9090 ULTRON_DB_PATH=./ultron.db ULTRON_LOG_LEVEL=debug ./bin/ultron-ap
```

### Cross-compile for Raspberry Pi

```bash
make build-arm
# Output: bin/ultron-ap-linux-arm64
```

Copy the binary to your Pi and run it. That's it.

### Deploy as a Service

```bash
# Copy binary
sudo cp bin/ultron-ap-linux-arm64 /opt/ultron-ap/ultron-ap

# Copy and enable service
sudo cp deploy/ultron-ap.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ultron-ap
```

Create an environment file at `/etc/ultron-ap/ultron-ap.env`:

```env
ULTRON_PORT=8080
ULTRON_DB_PATH=/var/lib/ultron-ap/ultron.db
ULTRON_LOG_LEVEL=info
```

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `ULTRON_PORT` | `8080` | HTTP server port |
| `ULTRON_DB_PATH` | `/var/lib/ultron-ap/ultron.db` | SQLite database path |
| `ULTRON_LOG_LEVEL` | `info` | Log level: debug, info, warn, error |

## API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check — returns `{"status": "ok"}` |

More endpoints coming as features are implemented.

## Project Structure

```
cmd/ultron-ap/          # Application entry point
internal/
  alerts/               # Alert engine and evaluation rules
  auth/                 # Authentication, sessions, and brute-force protection
  config/               # Configuration loading
  database/             # SQLite storage and audit logging
  docker/               # Docker Engine integration and controls
  metrics/              # System resource collection (CPU, RAM, etc.)
  notify/               # Telegram and Email notification dispatching
  pironman/             # Hardware controls (Pironman5)
  server/               # HTTP server, handlers, and SSE broker
  systemd/              # OS service monitoring and controls
  tailscale/            # VPN status integration
web/
  templates/            # Go HTML templates (HTMX)
  static/               # CSS, JS, and compiled Tailwind assets
```

## Development

```bash
make test       # Run all tests
make fmt        # Format code
make vet        # Run go vet
make run        # Build and run locally
```

## Roadmap

- [x] Project scaffolding & health endpoint
- [x] User authentication (bcrypt + sessions)
- [x] Dark mode UI layout (HTMX + Tailwind)
- [x] System metrics collector (CPU, RAM, disk, temp)
- [x] Docker container monitoring
- [x] Systemd service monitoring
- [x] Real-time dashboard with SSE
- [x] Alert engine with configurable thresholds
- [x] Telegram notifications
- [x] Email notifications
- [x] Service controls (start/stop/restart)
- [x] Action audit trail
- [x] Hardware integration (Pironman5)
- [x] Performance tuning (configurable intervals)

## License

MIT
