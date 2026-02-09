# PL0001: Project Scaffolding & Go Server - Implementation Plan

> **Status:** Complete
> **Story:** [US0001: Project Scaffolding & Go Server](../stories/US0001-project-scaffolding.md)
> **Epic:** [EP0001: Foundation & Authentication](../epics/EP0001-foundation-and-auth.md)
> **Created:** 2026-02-09
> **Language:** Go

## Overview

Set up the foundational Go project: directory structure, HTTP server, configuration loading from environment variables, SQLite database initialization with schema, health check endpoint, graceful shutdown, and embedded static assets. This is the base everything else builds upon.

## Acceptance Criteria Summary

| AC | Name | Description |
|----|------|-------------|
| AC1 | Server starts on configurable port | Binary starts, listens on port 8080 default, logs startup |
| AC2 | Port configurable via env var | ULTRON_PORT overrides default |
| AC3 | Config loads from env vars | All env vars loaded with correct defaults |
| AC4 | SQLite initializes on first run | DB created with all tables, directory auto-created |
| AC5 | Health check endpoint | GET /health returns 200 {"status":"ok"} without auth |

---

## Technical Context

### Language & Framework
- **Primary Language:** Go 1.22+
- **Framework:** net/http (standard library, Go 1.22 ServeMux with method routing)
- **Test Framework:** Go testing package + testify (assertions)
- **Database:** modernc.org/sqlite (pure Go, no CGo)

### Design Decision: Router
**Decision:** Use Go 1.22+ `net/http.ServeMux` (not chi, not gorilla/mux)

**Rationale:**
- Go 1.22 ServeMux supports `GET /path` method-based routing natively
- Zero external dependencies for routing
- Smaller binary size
- Sufficient for Ultron-AP's needs (< 20 routes total)

### Existing Patterns
None - greenfield project.

---

## Recommended Approach

**Strategy:** Test-After
**Rationale:** This is project scaffolding — mostly boilerplate setup (directory structure, config loading, DB init). TDD adds overhead for code that is structurally simple. Tests are better written after the structure exists to validate integration.

### Test Priority
1. Config loading with defaults and overrides
2. SQLite initialization and table verification
3. /health endpoint response
4. Graceful shutdown signal handling

---

## Implementation Tasks

| # | Task | File | Depends On | Status |
|---|------|------|------------|--------|
| 1 | Initialize Go module | `go.mod` | - | [x] |
| 2 | Create config package | `internal/config/config.go` | 1 | [x] |
| 3 | Create database package with schema | `internal/database/sqlite.go` | 1 | [x] |
| 4 | Create server package | `internal/server/server.go` | 2 | [x] |
| 5 | Create main entry point | `cmd/ultron-ap/main.go` | 2, 3, 4 | [x] |
| 6 | Create embedded assets scaffold | `web/templates/.gitkeep`, `web/static/.gitkeep`, `web/embed.go` | 1 | [x] |
| 7 | Create Makefile with build targets | `Makefile` | 1 | [x] |
| 8 | Create systemd unit file | `deploy/ultron-ap.service` | - | [x] |
| 9 | Write tests | `internal/config/config_test.go`, `internal/database/sqlite_test.go`, `internal/server/server_test.go` | 2, 3, 4 | [x] |

### Parallel Execution Groups

| Group | Tasks | Prerequisite |
|-------|-------|--------------|
| A | 2, 3, 6, 7, 8 | Task 1 (go mod init) |
| B | 4 | Task 2 (config) |
| C | 5 | Tasks 2, 3, 4 |
| D | 9 | Tasks 2, 3, 4 |

---

## Implementation Phases

### Phase 1: Project Structure & Dependencies
**Goal:** Go module initialized with directory structure and dependencies

- [ ] Run `go mod init github.com/cesareyeserrano/ultron-ap`
- [ ] Add dependencies: `modernc.org/sqlite`
- [ ] Create directory structure: `cmd/ultron-ap/`, `internal/config/`, `internal/database/`, `internal/server/`, `web/templates/`, `web/static/`, `deploy/`
- [ ] Create `web/embed.go` with `embed.FS` for templates and static

**Files:**
- `go.mod` - Module definition
- `web/embed.go` - Embed directives

### Phase 2: Configuration
**Goal:** Config struct loads from env vars with defaults

- [ ] Define `Config` struct with all fields: Port, DBPath, LogLevel, AdminUser, AdminPass, SessionTTL, MetricsInterval
- [ ] Implement `Load()` function reading from environment with defaults
- [ ] Validate port range (1-65535)
- [ ] Validate log level (debug, info, warn, error)

**Files:**
- `internal/config/config.go` - Config struct and loader

### Phase 3: Database
**Goal:** SQLite opens/creates DB and runs schema migration

- [ ] Implement `New(dbPath string)` to open/create SQLite database
- [ ] Auto-create directory if it doesn't exist (`os.MkdirAll`)
- [ ] Define schema SQL for tables: User, Session, Alert, AlertConfig, ActionLog
- [ ] Run schema on initialization (CREATE TABLE IF NOT EXISTS)
- [ ] Implement `Close()` for clean shutdown
- [ ] Enable WAL mode for better concurrent read performance

**Files:**
- `internal/database/sqlite.go` - Database initialization and schema

### Phase 4: HTTP Server
**Goal:** Server starts, serves /health, handles graceful shutdown

- [ ] Define `Server` struct with `http.ServeMux`, config, and DB reference
- [ ] Implement `New(cfg *config.Config, db *database.DB)` constructor
- [ ] Register `GET /health` handler returning `{"status":"ok"}`
- [ ] Implement `Start()` that listens on configured port
- [ ] Implement graceful shutdown via `context.Context` and `http.Server.Shutdown`
- [ ] Log startup message "Server started on :{port}"

**Files:**
- `internal/server/server.go` - Server with router and handlers
- `internal/server/handlers.go` - Health handler (and future handlers)

### Phase 5: Main Entry Point
**Goal:** Wire everything together, handle signals

- [ ] Load config
- [ ] Initialize database
- [ ] Create server
- [ ] Set up signal handling (SIGINT, SIGTERM)
- [ ] Start server in goroutine
- [ ] Wait for signal, trigger graceful shutdown
- [ ] Close DB, exit 0

**Files:**
- `cmd/ultron-ap/main.go` - Application entry point

### Phase 6: Build & Deploy Artifacts
**Goal:** Makefile for builds, systemd unit file

- [ ] Makefile targets: `build`, `build-arm`, `test`, `clean`, `run`
- [ ] ARM cross-compile: `GOOS=linux GOARCH=arm64 go build`
- [ ] Systemd unit file with description, restart policy, env file reference

**Files:**
- `Makefile` - Build automation
- `deploy/ultron-ap.service` - Systemd unit

### Phase 7: Testing
**Goal:** Tests for config, database, and server

- [ ] Config tests: defaults, overrides, invalid port, invalid log level
- [ ] Database tests: creation, tables exist, close
- [ ] Server tests: /health endpoint, startup (use httptest)
- [ ] Use `t.TempDir()` for test database paths

**Files:**
- `internal/config/config_test.go`
- `internal/database/sqlite_test.go`
- `internal/server/server_test.go`

### Phase 8: Verification
**Goal:** Verify all acceptance criteria

| AC | Verification Method | File Evidence | Status |
|----|---------------------|---------------|--------|
| AC1 | Test server starts on default port | `server_test.go` | Passed |
| AC2 | Test with ULTRON_PORT env var | `config_test.go` | Passed |
| AC3 | Test all config defaults | `config_test.go` | Passed |
| AC4 | Test DB creation and table check | `sqlite_test.go` | Passed |
| AC5 | Test /health returns 200 | `server_test.go` | Passed |

---

## Edge Case Handling

| # | Edge Case (from Story) | Handling Strategy | Phase |
|---|------------------------|-------------------|-------|
| 1 | Port already in use | `net.Listen` returns error; log and exit(1) | Phase 4 |
| 2 | Invalid port number (99999) | Config validation rejects port outside 1-65535; exit(1) | Phase 2 |
| 3 | DB directory not writable | `os.MkdirAll` returns error; log path+perms, exit(1) | Phase 3 |
| 4 | DB file corrupted | SQLite PRAGMA integrity_check on open; log error if fails | Phase 3 |
| 5 | SIGINT received | Signal handler triggers server.Shutdown + db.Close; exit(0) | Phase 5 |
| 6 | SIGTERM received | Same handler as SIGINT | Phase 5 |
| 7 | ULTRON_LOG_LEVEL invalid | Default to "info", log warning about invalid value | Phase 2 |

**Coverage:** 7/7 edge cases handled

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| modernc.org/sqlite slow on ARM | Medium | Benchmark during Phase 3; WAL mode helps read perf |
| Go 1.22 ServeMux insufficient | Low | Pattern is simple; can migrate to chi later if needed |
| Embed increases binary size | Low | Only embed minimal scaffold files for now |

---

## Definition of Done

- [x] All acceptance criteria implemented
- [x] Unit tests written and passing (22 tests)
- [x] Edge cases handled (7/7)
- [x] Code follows Go conventions (go fmt, go vet clean)
- [x] No linting errors
- [x] Binary compiles for linux/arm64 (13MB)
- [x] `go test ./...` passes

---

## Notes

- This plan intentionally keeps the server minimal. No auth middleware, no UI templates, no monitoring — those come in US0002, US0003, and EP0002.
- The `web/` directory is scaffolded with embed but will be populated in later stories.
- Go 1.22 is assumed for enhanced ServeMux routing. If Go 1.21 is required, standard `http.HandleFunc` works fine but without method routing.
