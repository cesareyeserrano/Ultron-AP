# US0001: Project Scaffolding & Go Server

> **Status:** In Progress
> **Epic:** [EP0001: Foundation & Authentication](../epics/EP0001-foundation-and-auth.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** the Go server to start as a single binary on a configurable port
**So that** I have a lightweight, self-contained service running on my Raspberry Pi

## Context

### Persona Reference
**Admin** - Raspberry Pi owner who needs a lightweight admin panel
[Full persona details](../personas.md#admin)

### Background
This is the foundational story. It sets up the Go project structure, HTTP server, configuration loading, and SQLite database initialization. Everything else builds on this.

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Performance | < 30MB RAM total | Server idle must use < 10MB |
| PRD | Performance | Binario ARM < 20MB | Minimal dependencies, pure Go SQLite |
| PRD | Architecture | Single binary | Embed templates, CSS, static assets |

---

## Acceptance Criteria

### AC1: Server starts and listens on configurable port
- **Given** the binary is compiled for linux/arm64
- **When** I run `./ultron-ap`
- **Then** the server starts listening on port 8080 (default)
- **And** logs "Server started on :8080" to stdout

### AC2: Port is configurable via environment variable
- **Given** `ULTRON_PORT=9090` is set
- **When** I run `./ultron-ap`
- **Then** the server starts listening on port 9090

### AC3: Configuration loads from env vars
- **Given** env vars `ULTRON_PORT`, `ULTRON_DB_PATH`, `ULTRON_LOG_LEVEL` are set
- **When** the server starts
- **Then** all configuration values are loaded correctly
- **And** missing optional vars use defaults (port=8080, db=/var/lib/ultron-ap/ultron.db, log=info)

### AC4: SQLite database initializes on first run
- **Given** no database file exists at the configured path
- **When** the server starts for the first time
- **Then** SQLite database is created with all required tables (User, Session, Alert, AlertConfig, ActionLog)
- **And** the directory is created if it doesn't exist

### AC5: Health check endpoint responds
- **Given** the server is running
- **When** I GET /health
- **Then** I receive 200 OK with `{"status": "ok"}`
- **And** no authentication is required

---

## Scope

### In Scope
- Go project structure (cmd/ultron-ap/, internal/, web/)
- HTTP server with basic router
- Configuration loading (env vars with defaults)
- SQLite initialization with schema migration
- /health endpoint
- Graceful shutdown on SIGINT/SIGTERM
- Embed directive for static assets
- Systemd unit file template

### Out of Scope
- Authentication (US0002)
- UI layout (US0004)
- Any monitoring features

---

## Technical Notes

- Use `net/http` standard library (no external router framework needed for this scope)
- Use `modernc.org/sqlite` for pure Go SQLite (no CGo needed for ARM cross-compile)
- Use `embed.FS` for embedding templates and static assets
- Project structure:
  ```
  cmd/ultron-ap/main.go
  internal/config/config.go
  internal/database/sqlite.go
  internal/server/server.go
  web/templates/
  web/static/
  ```

### Data Requirements
- SQLite schema for tables: User, Session, Alert, AlertConfig, ActionLog

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Port already in use | Log error "Port 8080 already in use" and exit with code 1 |
| Invalid port number (e.g. 99999) | Log error "Invalid port: 99999" and exit with code 1 |
| DB directory not writable | Log error with path and permission details, exit with code 1 |
| DB file corrupted | Log error, suggest backup and recreate |
| SIGINT received | Graceful shutdown: close DB, drain connections, exit 0 |
| SIGTERM received | Same as SIGINT |
| ULTRON_LOG_LEVEL invalid value | Default to "info", log warning |

---

## Test Scenarios

- [ ] Server starts on default port 8080
- [ ] Server starts on custom port via ULTRON_PORT
- [ ] Config loads all env vars with correct defaults
- [ ] SQLite DB created on first run
- [ ] SQLite tables exist after initialization
- [ ] /health returns 200 with correct JSON
- [ ] Server handles graceful shutdown
- [ ] Invalid port exits with error
- [ ] Missing DB directory gets created

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| None | - | - | - |

### External Dependencies

| Dependency | Type | Status |
|------------|------|--------|
| Go 1.21+ | Runtime | Available |
| modernc.org/sqlite | Library | Available |

---

## Estimation

**Story Points:** 5
**Complexity:** Medium

---

## Open Questions

- [ ] Use gorilla/mux or chi router, or stick with net/http ServeMux? - Owner: Dev

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0001 |
