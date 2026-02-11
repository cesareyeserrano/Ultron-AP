# US0003: Dark Mode UI Layout

> **Status:** In Progress
> **Epic:** [EP0001: Foundation & Authentication](../epics/EP0001-foundation-and-auth.md)
> **Owner:** TBD
> **Reviewer:** TBD
> **Created:** 2026-02-09

## User Story

**As a** Admin
**I want** a professional dark mode interface with sidebar navigation
**So that** the panel looks modern, is comfortable to use, and I can navigate between sections easily

## Context

### Persona Reference
**Admin** - Tech-proficient user who values professional, efficient interfaces
[Full persona details](../personas.md#admin)

### Background
Establishes the visual foundation for the entire application. Dark mode minimalista estilo Grafana/Portainer. All subsequent features render within this layout shell.

---

## Inherited Constraints

| Source | Type | Constraint | AC Implication |
|--------|------|------------|----------------|
| PRD | Performance | CSS < 50KB after purge | Tailwind purge must be aggressive |
| PRD | UI | Dark mode default | No light mode toggle needed |

---

## Acceptance Criteria

### AC1: Dark theme renders correctly
- **Given** I am logged in
- **When** the dashboard loads
- **Then** the background is dark (#1a1a2e or similar)
- **And** text is light (#e0e0e0 or similar)
- **And** accent colors are blue/cyan (#00d4ff or similar)

### AC2: Sidebar navigation
- **Given** the dashboard is loaded
- **When** I see the sidebar
- **Then** it shows navigation items: Dashboard, Docker, Services, Alerts, Settings
- **And** the current page is highlighted
- **And** each item has an icon (Lucide or Heroicons)

### AC3: Sidebar is collapsible
- **Given** the sidebar is expanded
- **When** I click the collapse button
- **Then** the sidebar collapses to icon-only mode
- **And** the main content area expands
- **And** the collapsed state is remembered (localStorage)

### AC4: Responsive layout
- **Given** I access the panel from a mobile device (< 768px)
- **When** the page loads
- **Then** the sidebar is hidden by default (hamburger menu)
- **And** all content is readable and usable
- **And** on tablet (768-1024px) the sidebar starts collapsed

### AC5: Header with user info
- **Given** I am logged in
- **When** I see the header
- **Then** it shows the Ultron-AP logo/name
- **And** shows a logout button
- **And** shows system uptime as a subtle indicator

---

## Scope

### In Scope
- Base HTML layout template (Go html/template)
- Tailwind CSS configuration with dark palette
- Sidebar navigation component
- Header component with logout
- Responsive breakpoints (mobile, tablet, desktop)
- Lucide icons (CDN or embedded)
- Typography: monospace for metrics, sans-serif for UI
- CSS transitions for sidebar collapse

### Out of Scope
- Light mode toggle
- Theme customization
- Dashboard content (just the shell/layout)
- Graficas o metricas (EP0002)

---

## Technical Notes

- Go `html/template` with layout inheritance (base.html -> page.html)
- Tailwind CSS: build at compile time, purge unused classes, embed result
- Sidebar state: save to localStorage, read on page load
- HTMX: `hx-boost` on navigation links for SPA-like transitions
- Icons: embed Lucide icon SVGs or use a minimal subset

### Data Requirements
- None (purely visual)

---

## Edge Cases & Error Handling

| Scenario | Expected Behaviour |
|----------|-------------------|
| localStorage not available | Sidebar defaults to expanded, no error |
| Very long page name in sidebar | Text truncated with ellipsis |
| JavaScript disabled | Layout works, sidebar not collapsible (static expanded) |
| Screen width < 320px | Content scrollable, no horizontal overflow |
| Extremely long username in header | Truncated with ellipsis |

---

## Test Scenarios

- [ ] Dark theme colors render correctly
- [ ] Sidebar shows all navigation items
- [ ] Sidebar collapse/expand works
- [ ] Sidebar state persists across page loads
- [ ] Mobile layout hides sidebar by default
- [ ] Tablet layout starts with sidebar collapsed
- [ ] Desktop layout starts with sidebar expanded
- [ ] Logout button visible in header
- [ ] Navigation highlights current page
- [ ] CSS file size < 50KB after purge

---

## Dependencies

### Story Dependencies

| Story | Type | What's Needed | Status |
|-------|------|---------------|--------|
| [US0001](US0001-project-scaffolding.md) | Infrastructure | Server, templates | Draft |
| [US0002](US0002-authentication.md) | Auth | Login page, auth middleware | Draft |

---

## Estimation

**Story Points:** 5
**Complexity:** Medium

---

## Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-02-09 | Claude | Story created from EP0001 |
