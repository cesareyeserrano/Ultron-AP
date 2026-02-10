# PL0002: User Authentication - Implementation Plan

> **Status:** Complete
> **Story:** [US0002: User Authentication](../stories/US0002-authentication.md)
> **Epic:** [EP0001: Foundation & Authentication](../epics/EP0001-foundation-and-auth.md)
> **Created:** 2026-02-09
> **Language:** Go

## Overview

Implement single-user authentication for Ultron-AP: login page, bcrypt password verification, session management with HttpOnly cookies, auth middleware protecting all routes, brute force protection (5 attempts / 15 min lockout), CSRF tokens, and logout. Initial admin user created from env vars on first boot.

## Acceptance Criteria Summary

| AC | Name | Description |
|----|------|-------------|
| AC1 | Login page renders | Unauthenticated users redirected to /login with form |
| AC2 | Successful login | POST /login with correct creds sets session cookie, redirects to / |
| AC3 | Failed login | Wrong creds show generic error, no info leak |
| AC4 | Brute force protection | 5 failures locks IP for 15 minutes |
| AC5 | Session expiration | Expired sessions cleaned up, user redirected to /login |
| AC6 | Logout | Session destroyed, cookie cleared, redirect to /login |
| AC7 | Auth middleware | All routes except /login and /health require auth |

---

## Technical Context

### Language & Framework
- **Primary Language:** Go 1.25
- **Framework:** net/http (standard library ServeMux with method routing)
- **Test Framework:** Go testing + testify
- **Database:** modernc.org/sqlite (pure Go)
- **Crypto:** golang.org/x/crypto/bcrypt

### Existing Patterns
- **Server struct:** Holds `*http.Server`, `*config.Config`, `*database.DB`; handlers are receiver methods
- **Route registration:** `mux.HandleFunc("GET /path", s.handler)` in `registerRoutes()`
- **DB operations:** Methods on `*database.DB` (embedded `*sql.DB`)
- **Config loading:** `config.Load()` reads `ULTRON_*` env vars with defaults
- **Templates:** `web/embed.go` with `embed.FS` for templates and static
- **Schema:** User and Session tables already exist in `sqlite.go`

### Design Decisions

**Middleware pattern:** Wrap `http.Handler` — `func (s *Server) requireAuth(next http.Handler) http.Handler`

**Brute force tracking:** In-memory `map[string]*loginAttempt` with `sync.Mutex`. Not persisted — resets on server restart. This is acceptable for single-user/single-instance.

**CSRF strategy:** Per-session token stored in Session table. Rendered as hidden field in forms, validated on all POST requests.

**Template rendering:** Use `html/template` with `embed.FS`. Login page is the first template; establishes the pattern for all future pages.

**Admin bootstrap:** On server start, check if User table is empty. If so, create admin from `ULTRON_ADMIN_USER` / `ULTRON_ADMIN_PASS` env vars with bcrypt hash.

---

## Recommended Approach

**Strategy:** TDD
**Rationale:** US0002 has 7 clear ACs, 10 edge cases, and is security-critical (auth, brute force, CSRF). TDD ensures every security behavior is verified before implementation. API contracts are well-defined (POST /login, POST /logout, middleware redirects).

### Test Priority
1. Auth middleware (protects routes, allows /health and /login)
2. Login success/failure with bcrypt
3. Brute force lockout and expiry
4. Session creation, validation, expiration, cleanup
5. CSRF token generation and validation
6. Logout and cookie clearing

---

## Implementation Tasks

| # | Task | File | Depends On | Status |
|---|------|------|------------|--------|
| 1 | Add bcrypt dependency | `go.mod` | - | [ ] |
| 2 | Extend Config with auth fields | `internal/config/config.go` | - | [ ] |
| 3 | Add DB methods for auth (user CRUD, session CRUD) | `internal/database/auth.go` | - | [ ] |
| 4 | Implement brute force tracker | `internal/auth/bruteforce.go` | - | [ ] |
| 5 | Implement CSRF token helper | `internal/auth/csrf.go` | - | [ ] |
| 6 | Implement auth middleware | `internal/server/middleware.go` | 3 | [ ] |
| 7 | Implement login/logout handlers | `internal/server/handlers_auth.go` | 2, 3, 4, 5 | [ ] |
| 8 | Create login template | `web/templates/login.html` | - | [ ] |
| 9 | Bootstrap admin user on startup | `cmd/ultron-ap/main.go` | 2, 3 | [ ] |
| 10 | Wire middleware and routes | `internal/server/server.go` | 6, 7 | [ ] |
| 11 | Write tests | `internal/database/auth_test.go`, `internal/auth/*_test.go`, `internal/server/middleware_test.go`, `internal/server/handlers_auth_test.go` | 3-10 | [ ] |

### Parallel Execution Groups

| Group | Tasks | Prerequisite |
|-------|-------|--------------|
| A | 1, 2, 3, 4, 5, 8 | None (independent) |
| B | 6 | Task 3 (DB methods) |
| C | 7 | Tasks 2, 3, 4, 5 |
| D | 9, 10 | Tasks 2, 3, 6, 7 |
| E | 11 | All above (TDD: tests written first per phase) |

---

## Implementation Phases

### Phase 1: Dependencies & Configuration
**Goal:** Add bcrypt dependency, extend Config with admin credentials and session TTL

- [ ] `go get golang.org/x/crypto/bcrypt`
- [ ] Add fields to Config: AdminUser, AdminPass, SessionTTL
- [ ] Read `ULTRON_ADMIN_USER` (default: "admin"), `ULTRON_ADMIN_PASS` (required), `ULTRON_SESSION_TTL` (default: "24h")
- [ ] Validate: AdminPass must be set, SessionTTL must parse as duration

**Files:**
- `go.mod` — New dependency
- `internal/config/config.go` — Extended Config struct

### Phase 2: Database Auth Methods
**Goal:** CRUD operations for users and sessions

- [ ] Create `internal/database/auth.go` with methods on `*DB`:
  - `CreateUser(username, passwordHash string) error`
  - `GetUserByUsername(username string) (*User, error)` — returns User struct
  - `UserCount() (int, error)` — for bootstrap check
  - `CreateSession(session *Session) error`
  - `GetSession(token string) (*Session, error)`
  - `DeleteSession(token string) error`
  - `DeleteExpiredSessions() (int64, error)`
- [ ] Define User and Session structs in same file
- [ ] All queries use parameterized statements (SQL injection prevention)

**Files:**
- `internal/database/auth.go` — Auth DB methods and model structs

### Phase 3: Auth Utilities
**Goal:** Brute force tracker and CSRF token helpers

- [ ] Create `internal/auth/bruteforce.go`:
  - `Tracker` struct with `map[string]*attempt` and `sync.Mutex`
  - `RecordFailure(ip string)` — increment counter, set first-failure timestamp
  - `IsLocked(ip string) bool` — true if >=5 failures within 15 minutes
  - `Reset(ip string)` — clear on successful login
  - `Cleanup()` — remove expired entries (call periodically or on check)
- [ ] Create `internal/auth/csrf.go`:
  - `GenerateToken() (string, error)` — 32 bytes crypto/rand, hex encoded
  - `ValidateToken(session, submitted string) bool` — constant-time compare

**Files:**
- `internal/auth/bruteforce.go` — Brute force protection
- `internal/auth/csrf.go` — CSRF token utilities

### Phase 4: Login Template
**Goal:** HTML login page with form

- [ ] Create `web/templates/login.html`:
  - Form with username, password, hidden CSRF token field
  - Error message area (shown on failed login / lockout)
  - Minimal styling (will be replaced in US0003 with Tailwind)
- [ ] Set up template parsing from embedded FS in server

**Files:**
- `web/templates/login.html` — Login form template

### Phase 5: Auth Middleware
**Goal:** Middleware that protects routes, redirects unauthenticated users

- [ ] Create `internal/server/middleware.go`:
  - `requireAuth(next http.Handler) http.Handler` — method on Server
  - Read session cookie, validate against DB
  - If valid: set user info in request context, call next
  - If invalid/missing: redirect to /login (HTML) or 401 (API, based on Accept header)
  - Exempt paths: /login, /health, /static/
- [ ] Clean up expired sessions on each check (lightweight)

**Files:**
- `internal/server/middleware.go` — Auth middleware

### Phase 6: Login & Logout Handlers
**Goal:** Handle authentication flow

- [ ] Create `internal/server/handlers_auth.go`:
  - `handleLoginPage(w, r)` — GET /login: render template with CSRF token
  - `handleLogin(w, r)` — POST /login:
    1. Check brute force lockout
    2. Validate CSRF token
    3. Get user from DB by username
    4. Compare bcrypt hash
    5. On success: create session, set cookie (HttpOnly, SameSite=Strict), redirect /
    6. On failure: record attempt, re-render with error
  - `handleLogout(w, r)` — POST /logout:
    1. Delete session from DB
    2. Clear cookie (Max-Age=0)
    3. Redirect to /login

**Files:**
- `internal/server/handlers_auth.go` — Login/logout handlers

### Phase 7: Admin Bootstrap & Wiring
**Goal:** Create admin user on first boot, wire middleware and new routes

- [ ] In `cmd/ultron-ap/main.go` after DB init:
  - Check `db.UserCount()` — if 0, create admin from config env vars
  - Hash password with bcrypt cost 10
  - Call `db.CreateUser()`
- [ ] Update `internal/server/server.go`:
  - Add brute force tracker to Server struct
  - Update `registerRoutes()` with new routes:
    - `GET /login` → `handleLoginPage`
    - `POST /login` → `handleLogin`
    - `POST /logout` → `handleLogout`
  - Wrap protected routes with `requireAuth` middleware
  - Serve `/static/` from embedded FS

**Files:**
- `cmd/ultron-ap/main.go` — Admin bootstrap
- `internal/server/server.go` — Updated routing and middleware wiring

### Phase 8: Testing
**Goal:** Comprehensive tests for all auth components

- [ ] `internal/config/config_test.go` — Tests for new auth config fields
- [ ] `internal/database/auth_test.go` — User CRUD, session CRUD, expired cleanup
- [ ] `internal/auth/bruteforce_test.go` — Lockout after 5, expiry after 15m, reset on success
- [ ] `internal/auth/csrf_test.go` — Token generation, validation, constant-time compare
- [ ] `internal/server/middleware_test.go` — Protected routes redirect, /health and /login exempt
- [ ] `internal/server/handlers_auth_test.go` — Login success/failure, logout, CSRF validation, brute force integration

**Files:**
- `internal/config/config_test.go` (updated)
- `internal/database/auth_test.go`
- `internal/auth/bruteforce_test.go`
- `internal/auth/csrf_test.go`
- `internal/server/middleware_test.go`
- `internal/server/handlers_auth_test.go`

### Phase 9: Verification
**Goal:** Verify all acceptance criteria

| AC | Verification Method | File Evidence | Status |
|----|---------------------|---------------|--------|
| AC1 | Test GET / redirects to /login | `middleware_test.go` | Pending |
| AC2 | Test POST /login with correct creds | `handlers_auth_test.go` | Pending |
| AC3 | Test POST /login with wrong creds | `handlers_auth_test.go` | Pending |
| AC4 | Test lockout after 5 failures | `bruteforce_test.go`, `handlers_auth_test.go` | Pending |
| AC5 | Test expired session redirect | `middleware_test.go` | Pending |
| AC6 | Test POST /logout clears session | `handlers_auth_test.go` | Pending |
| AC7 | Test middleware on various routes | `middleware_test.go` | Pending |

---

## Edge Case Handling

| # | Edge Case (from Story) | Handling Strategy | Phase |
|---|------------------------|-------------------|-------|
| 1 | Empty username submitted | bcrypt compare fails; show generic "Invalid username or password" | Phase 6 |
| 2 | Empty password submitted | Same as empty username — generic error | Phase 6 |
| 3 | SQL injection in username | All DB queries use parameterized statements (`?` placeholders) | Phase 2 |
| 4 | XSS in username field | `html/template` auto-escapes all output | Phase 4 |
| 5 | Session cookie tampered | Token not found in DB → redirect to /login | Phase 5 |
| 6 | Concurrent login from 2 devices | Each login creates independent session; both valid | Phase 6 |
| 7 | Server restart | Sessions persisted in SQLite; survive restart | Phase 2 |
| 8 | CSRF token missing | POST handler returns 403 Forbidden | Phase 6 |
| 9 | CSRF token invalid | POST handler returns 403 Forbidden | Phase 6 |
| 10 | Lockout expires | Tracker checks timestamp; allows login after 15 min | Phase 3 |

**Coverage:** 10/10 edge cases handled

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| bcrypt slow on ARM (~100ms cost 10) | Low | Acceptable for single-user; cost 10 is minimum secure |
| In-memory brute force tracker lost on restart | Low | Acceptable trade-off; attacker gets fresh 5 attempts |
| Session fixation attack | Medium | Generate new session token on each login |
| Timing side-channel on username check | Low | Always run bcrypt even if user not found (constant time) |

---

## Definition of Done

- [ ] All 7 acceptance criteria implemented
- [ ] Unit tests written and passing
- [ ] 10/10 edge cases handled
- [ ] Code follows Go conventions (go fmt, go vet clean)
- [ ] No linting errors
- [ ] bcrypt cost factor >= 10
- [ ] Session cookies have HttpOnly + SameSite=Strict flags
- [ ] CSRF validated on all POST endpoints
- [ ] `go test ./...` passes

---

## Notes

- The login page will have minimal styling in this story. US0003 (Dark Mode Layout) will add Tailwind and proper design.
- ULTRON_ADMIN_PASS is **required** — the server should refuse to start without it.
- Session cleanup of expired entries happens lazily during middleware checks. A background goroutine for periodic cleanup can be added later if needed.
- The brute force tracker keys on IP address extracted from `r.RemoteAddr`. Behind a reverse proxy, this should be updated to check `X-Forwarded-For` (future consideration).
