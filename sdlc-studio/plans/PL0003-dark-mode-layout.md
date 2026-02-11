# PL0003: Dark Mode UI Layout - Implementation Plan

> **Status:** Complete
> **Story:** [US0003: Dark Mode UI Layout](../stories/US0003-dark-mode-layout.md)
> **Epic:** [EP0001: Foundation & Authentication](../epics/EP0001-foundation-and-auth.md)
> **Created:** 2026-02-09
> **Language:** Go + HTML/CSS/JS

## Overview

Implement the visual shell for Ultron-AP: a dark mode layout with collapsible sidebar navigation, responsive breakpoints, and a header with logout and uptime indicator. Uses Go `html/template` with layout inheritance (base + page), Tailwind CSS built at compile time and embedded as a static asset, and Lucide icons (SVG subset). HTMX added for SPA-like page transitions via `hx-boost`. This story is purely visual — no backend data beyond what auth already provides.

## Acceptance Criteria Summary

| AC | Name | Description |
|----|------|-------------|
| AC1 | Dark theme renders | Dark background (#1a1a2e), light text (#e0e0e0), blue/cyan accents (#00d4ff) |
| AC2 | Sidebar navigation | Shows Dashboard, Docker, Services, Alerts, Settings with icons; current page highlighted |
| AC3 | Sidebar collapsible | Collapse to icon-only, main content expands, state saved to localStorage |
| AC4 | Responsive layout | Mobile: sidebar hidden (hamburger); Tablet: sidebar collapsed; Desktop: sidebar expanded |
| AC5 | Header with user info | Logo/name, logout button, system uptime indicator |

---

## Technical Context

### Language & Framework
- **Primary Language:** Go 1.25
- **Framework:** net/http (standard library ServeMux)
- **Templating:** html/template with embedded FS
- **CSS:** Tailwind CSS (standalone CLI, built at compile time)
- **Icons:** Lucide (inline SVG subset)
- **Interactivity:** HTMX for navigation boosting, vanilla JS for sidebar toggle
- **Test Framework:** Go testing + testify (server tests), manual verification (visual)

### Existing Patterns
- **Server struct:** Holds `*http.Server`, `*config.Config`, `*database.DB`, `bruteForce`, `templates fs.FS`
- **Template rendering:** `template.ParseFS(s.templates, "templates/login.html")` in handlers_auth.go
- **Route registration:** `mux.HandleFunc("GET /path", s.handler)` / `mux.Handle("GET /", s.requireAuth(...))` in registerRoutes()
- **Embed FS:** `web/embed.go` with `//go:embed templates/*` and `//go:embed static/*`
- **Static serving:** `mux.Handle("GET /static/", ...)` already wired
- **Login template:** Standalone HTML with inline `<style>`, dark theme colors already matching (#1a1a2e, #e0e0e0, #e94560)
- **Dashboard placeholder:** Inline HTML string in `handleDashboardPlaceholder` — to be replaced

### Design Decisions

**Template inheritance pattern:** Go's `html/template` doesn't have native inheritance. Use `template.ParseFS` with multiple files and `{{define "content"}}` / `{{template "content" .}}` blocks. Base layout (`base.html`) defines the shell; each page defines a `content` block.

**Tailwind CSS approach:** Use the standalone Tailwind CLI (`tailwindcss`) to build CSS at development time. Output to `web/static/css/app.css` and embed via `embed.FS`. No Node.js required — the standalone binary works directly. Include a `make css` target.

**Icons:** Embed a small set of Lucide SVG icons as Go template partials (`templates/icons/*.svg`), or inline them directly in the sidebar template. This avoids CDN dependency and keeps the binary self-contained.

**HTMX:** Include `htmx.min.js` in `web/static/js/`. Add `hx-boost="true"` to navigation links for SPA-like transitions without full page reloads.

**Uptime:** Use Go's `time.Now()` at server start, compute uptime in the handler, pass to template. Simple — no new packages needed.

---

## Recommended Approach

**Strategy:** Test-After
**Rationale:** US0003 is UI-heavy with 5 ACs that are all visual/behavioral (dark theme colors, sidebar toggle, responsive breakpoints). TDD is poorly suited for template rendering and CSS validation. Tests will verify template parsing, handler responses, and uptime calculation after implementation. Visual verification is the primary validation method.

### Test Priority
1. Base template parses without error
2. Dashboard handler returns 200 with expected HTML structure
3. Uptime calculation is correct
4. All page handlers render within the base layout
5. Static assets served correctly (CSS, JS)

---

## Implementation Tasks

| # | Task | File | Depends On | Status |
|---|------|------|------------|--------|
| 1 | Install Tailwind CSS standalone CLI | `Makefile` | - | [ ] |
| 2 | Create Tailwind config with dark palette | `tailwind.config.js` | - | [ ] |
| 3 | Create Tailwind input CSS | `web/css/input.css` | - | [ ] |
| 4 | Create base layout template | `web/templates/base.html` | - | [ ] |
| 5 | Create sidebar partial | `web/templates/partials/sidebar.html` | 4 | [ ] |
| 6 | Create header partial | `web/templates/partials/header.html` | 4 | [ ] |
| 7 | Create dashboard page template | `web/templates/dashboard.html` | 4 | [ ] |
| 8 | Update login template to match design system | `web/templates/login.html` | 3 | [ ] |
| 9 | Add sidebar toggle JS | `web/static/js/sidebar.js` | - | [ ] |
| 10 | Download and embed HTMX | `web/static/js/htmx.min.js` | - | [ ] |
| 11 | Add uptime tracking to Server | `internal/server/server.go` | - | [ ] |
| 12 | Create template renderer with layout support | `internal/server/render.go` | 4, 5, 6 | [ ] |
| 13 | Replace dashboard placeholder handler | `internal/server/handlers.go` | 7, 12 | [ ] |
| 14 | Add Makefile targets (css, css-watch) | `Makefile` | 1, 2, 3 | [ ] |
| 15 | Build CSS and verify size < 50KB | `web/static/css/app.css` | 14 | [ ] |
| 16 | Write tests | `internal/server/*_test.go` | All above | [ ] |

### Parallel Execution Groups

| Group | Tasks | Prerequisite |
|-------|-------|--------------|
| A | 1, 2, 3, 9, 10, 11 | None (independent) |
| B | 4, 5, 6, 7, 8 | Task 3 (CSS classes reference) |
| C | 12, 13 | Tasks 4, 5, 6, 7, 11 |
| D | 14, 15 | Tasks 1, 2, 3, 4-8 |
| E | 16 | All above |

---

## Implementation Phases

### Phase 1: Tailwind CSS Setup
**Goal:** Install Tailwind standalone CLI, configure dark palette, create build pipeline

- [ ] Download `tailwindcss` standalone CLI for macOS ARM64
- [ ] Create `tailwind.config.js` with custom dark palette colors (#1a1a2e background, #16213e sidebar, #0f3460 card, #e0e0e0 text, #00d4ff accent, #e94560 danger)
- [ ] Create `web/css/input.css` with `@tailwind` directives and custom base styles
- [ ] Add `make css` target: `./tailwindcss -i web/css/input.css -o web/static/css/app.css --minify`
- [ ] Add `make css-watch` target for development
- [ ] Add `tailwindcss` binary to `.gitignore`

**Files:**
- `tailwind.config.js` — Tailwind configuration
- `web/css/input.css` — Tailwind input file
- `Makefile` — New css targets
- `.gitignore` — Exclude tailwindcss binary

### Phase 2: HTMX & JavaScript
**Goal:** Add HTMX and sidebar toggle scripts

- [ ] Download `htmx.min.js` (v2.x) to `web/static/js/htmx.min.js`
- [ ] Create `web/static/js/sidebar.js` for sidebar toggle logic:
  - Toggle `collapsed` class on sidebar element
  - Save state to localStorage
  - Read state on page load
  - Handle mobile hamburger menu

**Files:**
- `web/static/js/htmx.min.js` — HTMX library
- `web/static/js/sidebar.js` — Sidebar toggle logic

### Phase 3: Base Layout & Partials
**Goal:** Create the template shell that all pages use

- [ ] Create `web/templates/base.html` — Main layout with:
  - HTML head (meta, title, CSS link, HTMX script)
  - Sidebar include
  - Header include
  - `{{block "content" .}}{{end}}` for page content
  - Sidebar JS script
- [ ] Create `web/templates/partials/sidebar.html`:
  - Nav items: Dashboard (/), Docker (/docker), Services (/services), Alerts (/alerts), Settings (/settings)
  - Lucide SVG icons inline for each item
  - `hx-boost="true"` on links
  - Active page highlighting via template data
  - Collapse/expand button
  - Responsive classes: hidden on mobile, collapsible on tablet/desktop
- [ ] Create `web/templates/partials/header.html`:
  - Ultron-AP logo/name (left)
  - System uptime display (center/right)
  - Logout button as POST form (right)
  - Mobile hamburger button (visible < 768px)

**Files:**
- `web/templates/base.html` — Base layout
- `web/templates/partials/sidebar.html` — Sidebar navigation
- `web/templates/partials/header.html` — Top header bar

### Phase 4: Page Templates
**Goal:** Create dashboard page and update login

- [ ] Create `web/templates/dashboard.html`:
  - Extends base layout via `{{define "content"}}`
  - Shows "Dashboard" heading
  - Placeholder text: "Metrics and monitoring coming soon"
  - Placeholder cards for CPU, Memory, Disk, Network (empty shells for US0004-US0007)
- [ ] Update `web/templates/login.html`:
  - Use Tailwind classes instead of inline styles
  - Keep same color scheme and form structure
  - Ensure it does NOT use base layout (standalone page)

**Files:**
- `web/templates/dashboard.html` — Dashboard page
- `web/templates/login.html` — Updated login page

### Phase 5: Server-Side Rendering
**Goal:** Template renderer with layout support, uptime tracking, updated handlers

- [ ] Add `startedAt time.Time` field to Server struct
- [ ] Set `startedAt: time.Now()` in `New()` constructor
- [ ] Create `internal/server/render.go`:
  - `type PageData` struct with fields: Title, ActivePage, Uptime, Username, CSRFToken, Content (interface{})
  - `func (s *Server) render(w, r, tmplName string, data interface{})` method
  - Parses base.html + partials + page template from embedded FS
  - Computes uptime from `s.startedAt`
  - Extracts username from session context
- [ ] Update `handleDashboardPlaceholder` → `handleDashboard` in `handlers.go`:
  - Use `s.render()` with dashboard template
  - Pass uptime, active page = "dashboard"
- [ ] Update `registerRoutes` to use `handleDashboard`

**Files:**
- `internal/server/server.go` — Add startedAt field
- `internal/server/render.go` — Template rendering helper
- `internal/server/handlers.go` — Updated dashboard handler

### Phase 6: CSS Build & Size Verification
**Goal:** Build final CSS, verify < 50KB constraint

- [ ] Run `make css` to generate purged, minified CSS
- [ ] Verify `web/static/css/app.css` < 50KB
- [ ] If over 50KB, reduce Tailwind config (remove unused utilities)

**Files:**
- `web/static/css/app.css` — Built CSS output

### Phase 7: Testing
**Goal:** Verify all acceptance criteria with tests and manual verification

- [ ] Write test: base template parses without errors
- [ ] Write test: dashboard handler returns 200 with correct Content-Type
- [ ] Write test: dashboard response contains expected HTML elements (sidebar, header, nav items)
- [ ] Write test: uptime formatting is correct
- [ ] Write test: static CSS file is served
- [ ] Verify CSS file size < 50KB (assertion in test)
- [ ] Manual: verify dark theme renders correctly in browser
- [ ] Manual: verify sidebar collapse/expand
- [ ] Manual: verify responsive breakpoints
- [ ] Manual: verify logout button works from header

**Files:**
- `internal/server/render_test.go` — Template rendering tests
- `internal/server/handlers_test.go` — Updated handler tests

### Phase 8: Verification
**Goal:** Verify all acceptance criteria

| AC | Verification Method | File Evidence | Status |
|----|---------------------|---------------|--------|
| AC1 | Test dark theme CSS classes in templates, manual visual check | `base.html`, `app.css` | Pending |
| AC2 | Test sidebar nav items in HTML response | `sidebar.html`, handler tests | Pending |
| AC3 | Test sidebar JS localStorage logic, manual toggle check | `sidebar.js`, manual | Pending |
| AC4 | Test responsive CSS classes in templates, manual resize check | `base.html`, `app.css` | Pending |
| AC5 | Test header contains logo, logout, uptime; manual visual check | `header.html`, handler tests | Pending |

---

## Edge Case Handling

| # | Edge Case (from Story) | Handling Strategy | Phase |
|---|------------------------|-------------------|-------|
| 1 | localStorage not available | Wrap localStorage access in try/catch; default to expanded sidebar | Phase 2 |
| 2 | Very long page name in sidebar | Use Tailwind `truncate` class (text-overflow: ellipsis) on nav text | Phase 3 |
| 3 | JavaScript disabled | Base layout uses expanded sidebar as default state; collapse requires JS but layout is functional without it | Phase 3 |
| 4 | Screen width < 320px | Use `min-w-0` and `overflow-x-hidden` on body; content scrolls vertically | Phase 3 |
| 5 | Extremely long username in header | Use Tailwind `truncate` with `max-w-[120px]` on username display | Phase 3 |

**Coverage:** 5/5 edge cases handled

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Tailwind CSS standalone CLI not available for ARM64 macOS | Medium | Fall back to npx/npm if needed; or use x86 via Rosetta |
| CSS > 50KB after purge | Low | Tailwind purge is aggressive; custom palette is small; monitor size |
| Go template inheritance complexity | Low | Use `ParseFS` with named blocks; pattern is well-established |
| HTMX adds JS weight | Low | htmx.min.js is ~14KB gzipped; acceptable for SPA-like UX |
| Sidebar JS conflicts with HTMX page transitions | Medium | Use `htmx:afterSwap` event to reinitialize sidebar state after navigation |

---

## Definition of Done

- [ ] All 5 acceptance criteria implemented
- [ ] Dark theme with correct colors (#1a1a2e, #e0e0e0, #00d4ff)
- [ ] Sidebar with 5 nav items and Lucide icons
- [ ] Sidebar collapse/expand with localStorage persistence
- [ ] Responsive: mobile (hidden), tablet (collapsed), desktop (expanded)
- [ ] Header with logo, uptime, logout
- [ ] CSS < 50KB after Tailwind purge
- [ ] Unit tests written and passing
- [ ] No linting errors (go fmt, go vet)
- [ ] `go test ./...` passes
- [ ] Manual visual verification in browser

---

## Notes

- Login page remains standalone (no base layout) — it's shown before authentication
- The sidebar nav items (Docker, Services, Alerts, Settings) will point to placeholder routes that return the dashboard layout with a "Coming soon" content block. These routes will be implemented in their respective stories.
- HTMX `hx-boost` means navigation clicks replace only the body content, giving SPA-like speed while keeping server-side rendering.
- The uptime indicator shows time since server start, formatted as "Xd Xh Xm" or "Xh Xm" for shorter durations.
- Tailwind standalone CLI is preferred over Node.js to keep the build toolchain minimal (Go + Tailwind binary only).
