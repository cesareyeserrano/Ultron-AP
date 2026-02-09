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
  config/               # Configuration loading and validation
  database/             # SQLite initialization and schema
  server/               # HTTP server, routing, handlers
web/
  templates/            # Go HTML templates (HTMX)
  static/               # CSS, JS, static assets
deploy/                 # Systemd unit file
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
- [ ] User authentication (bcrypt + sessions)
- [ ] Dark mode UI layout (HTMX + Tailwind)
- [ ] System metrics collector (CPU, RAM, disk, temp)
- [ ] Docker container monitoring
- [ ] Systemd service monitoring
- [ ] Real-time dashboard with SSE
- [ ] Alert engine with configurable thresholds
- [ ] Telegram notifications
- [ ] Email notifications
- [ ] Service controls (start/stop/restart)
- [ ] Action audit trail

## License

MIT
