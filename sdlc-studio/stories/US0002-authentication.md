# US0002: User Authentication

> **Status:** Done
> **Epic:** [EP0001: Foundation & Authentication](../epics/EP0001-foundation-and-auth.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** to protect the panel with login credentials
**So that** only I can access the dashboard and control my services

## Context

### Persona Reference
**Admin** - Raspberry Pi owner, single user
[Full persona details](../personas.md#admin)

### Background
Single-user authentication. One admin account, configurable credentials, secure session management with brute force protection.

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Security | bcrypt cost 10+ | Password hash verification may take ~100ms on ARM |
| PRD | Security | HttpOnly, SameSite=Strict cookies | Session cookie flags mandatory |
| PRD | Security | CSRF protection | All POST forms need CSRF token |

---

## Acceptance Criteria

### AC1: Login page renders
- **Given** I am not authenticated
- **When** I navigate to any route (e.g., /)
- **Then** I am redirected to /login
- **And** the login page shows a form with username and password fields

### AC2: Successful login
- **Given** the admin credentials are configured (ULTRON_ADMIN_USER=admin, ULTRON_ADMIN_PASS=secret)
- **When** I POST /login with username=admin and password=secret
- **Then** a session cookie is set with flags HttpOnly, SameSite=Strict
- **And** I am redirected to / (dashboard)

### AC3: Failed login
- **Given** I enter wrong credentials
- **When** I POST /login with username=admin and password=wrong
- **Then** I see an error message "Invalid username or password"
- **And** the error message does NOT reveal whether the username exists
- **And** no session cookie is set

### AC4: Brute force protection
- **Given** I have failed login 5 times in a row
- **When** I attempt a 6th login (even with correct credentials)
- **Then** I see "Too many login attempts. Try again in 15 minutes."
- **And** no login is accepted until the cooldown expires

### AC5: Session expiration
- **Given** I logged in 24 hours ago (default TTL)
- **When** I navigate to any authenticated route
- **Then** I am redirected to /login
- **And** the expired session is cleaned up from the database

### AC6: Logout
- **Given** I am logged in
- **When** I click the logout button in the header
- **Then** my session is destroyed in the database
- **And** the session cookie is cleared
- **And** I am redirected to /login

### AC7: Auth middleware protects all routes
- **Given** I am not authenticated
- **When** I try to access / or /api/metrics or any route except /login and /health
- **Then** I receive a redirect to /login (HTML) or 401 (API)

---

## Scope

### In Scope
- Login page (form with username/password)
- Login POST handler with bcrypt verification
- Session creation and cookie management
- Auth middleware for route protection
- Brute force protection (5 attempts, 15 min lockout)
- Logout handler
- Initial admin user creation on first boot (from env vars)
- CSRF token generation and validation

### Out of Scope
- Multiple users
- Password reset
- OAuth/SSO
- Remember me / persistent login
- API key authentication

---

## Technical Notes

- Use `golang.org/x/crypto/bcrypt` for password hashing (cost factor 10)
- Session token: crypto/rand generated, 32 bytes, hex encoded
- Store session in SQLite `Session` table with expiry
- CSRF: generate per-session token, validate on all POST requests
- Brute force: track failed attempts by IP in memory (map with mutex), reset on success

### API Contracts

```
POST /login
Content-Type: application/x-www-form-urlencoded
Body: username=admin&password=secret&csrf_token=xxx

Success: 302 -> /
  Set-Cookie: session=<token>; HttpOnly; SameSite=Strict; Path=/

Failure: 200 (re-render login page with error)

POST /logout
  302 -> /login
  Set-Cookie: session=; Max-Age=0
```

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| Empty username submitted | "Invalid username or password" (same as wrong creds) |
| Empty password submitted | "Invalid username or password" |
| SQL injection in username | Parameterized queries prevent injection |
| XSS in username field | HTML escaped in template output |
| Session cookie tampered | Session not found in DB, redirect to login |
| Concurrent login from 2 devices | Both sessions valid independently |
| Server restart | Sessions persist in SQLite, existing sessions still valid |
| CSRF token missing | 403 Forbidden |
| CSRF token invalid | 403 Forbidden |
| Lockout expires | Next login attempt is accepted normally |

---

## Test Scenarios

- [ ] Login page renders with form fields
- [ ] Successful login sets cookie and redirects
- [ ] Failed login shows error without leaking user existence
- [ ] Brute force lockout after 5 failures
- [ ] Lockout expires after 15 minutes
- [ ] Session cookie has correct flags (HttpOnly, SameSite)
- [ ] Expired session redirects to login
- [ ] Logout destroys session and clears cookie
- [ ] Unauthenticated request to / redirects to /login
- [ ] /health is accessible without auth
- [ ] CSRF token validated on POST
- [ ] Initial admin created from env vars on first boot

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0001](US0001-project-scaffolding.md) | Infrastructure | Server, DB, router | Draft |

---

## Estimation

**Story Points:** 8
**Complexity:** High

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0001 |
