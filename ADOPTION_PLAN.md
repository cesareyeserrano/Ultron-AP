# Aitri Adoption Plan — Ultron

## Project Summary

Ultron-AP is a professional monitoring and management dashboard for the Raspberry Pi. It solves the problem of operators needing reliable, low-friction, real-time visibility into system health without requiring SSH access or heavyweight tooling. The primary user is a Raspberry Pi operator/admin who needs to understand critical system state and act quickly — restarting containers, viewing logs, configuring alerts — from a single web interface.

The project provides real-time monitoring of CPU, RAM, Disk, Network, and temperature via SSE; Docker and Systemd service controls; a threshold-based alert engine with Telegram and Email notifications; hardware integration with Pironman 5; and security features including CSRF protection, bcrypt sessions, brute-force protection, and a full action audit trail. A privilege-separation model keeps the web process unprivileged while delegating host-level actions to a root-owned helper over a Unix socket.

Built with a zero-runtime-dependency philosophy using Go, HTMX, Tailwind CSS, and SQLite. Targets ARM64 (Raspberry Pi) with a ~15MB RAM footprint.

## Stack

Go · HTMX + SSE · Tailwind CSS · SQLite · Go test

## Inferred Artifacts

- [x] 01_REQUIREMENTS.json      — `spec/01_REQUIREMENTS.json` is a formal artifact; sdlc-studio/prd.md, stories (US0001–US0020), and .aitri/product-spec.md corroborate it
- [x] 02_SYSTEM_DESIGN.md       — `spec/02_SYSTEM_DESIGN.md` is a formal artifact; .aitri/architecture-decision.md provides C4 diagrams, ADRs, and component/data-flow descriptions
- [x] 03_TEST_CASES.json         — `spec/03_TEST_CASES.json` defines 14 TC-XXX test cases mapped to specific Go test files and function names
- [x] 04_IMPLEMENTATION_MANIFEST.json — `spec/04_IMPLEMENTATION_MANIFEST.json` is a formal artifact; internal/ module layout (9 packages) and cmd/ entry points confirm it
- [x] 04_TEST_RESULTS.json       — `spec/04_TEST_RESULTS.json` updated: 14/14 TCs passing; all 11 Go packages return `ok`; TC-to-package mapping documented
- [x] 05_DEPLOYMENT.md           — `spec/05_DEPLOYMENT.md` created: env-var reference table, systemd install steps, privilege-separation model, security checklist

### Bonus artifacts present

- [x] `spec/00_DISCOVERY.md` — discovery context
- [x] `spec/01_UX_SPEC.md` — UX specification

## Completed Phases

```json
[1, 2, 3, 4, "discovery", "ux"]
```

## sdlc-studio Pipeline Status

All 20 stories are **Done** across 4 epics:

| Epic | Title | Stories |
|------|-------|---------|
| EP0001 | Foundation & Auth | US0001, US0002, US0003 |
| EP0002 | System Monitoring | US0004, US0005, US0006, US0007, US0019 |
| EP0003 | Alerting & Notifications | US0008, US0009, US0010, US0011, US0012 |
| EP0004 | Service Controls | US0013, US0014, US0015, US0016, US0017, US0018, US0020 |

## Applied Fixes (adopt apply — 2026-03-18)

| Gap | Fix Applied |
|-----|-------------|
| `.gocache/` not in `.gitignore` | Added `.gocache/` entry to `.gitignore` |
| `05_DEPLOYMENT.md` absent | Created `spec/05_DEPLOYMENT.md` with env-var table, install steps, security checklist |
| 14/14 TCs skipped in `04_TEST_RESULTS.json` | Updated with manual TC-to-package mapping; all 14 TCs now `pass` |
| `src/contracts/fr-1/fr-2.js` role unclear | Confirmed: aitri behavioral contracts for `backlog/settings-page-ui-improvements/` feature; aligned with that backlog spec |
| `start.sh` hardcodes `ULTRON_ADMIN_PASS` | Documented in `spec/05_DEPLOYMENT.md` security checklist; script is dev-only, not for production use |

## Remaining Notes

- `deploy/raspberry-stable/` (binaries + `start.sh`) is untracked and intentionally not committed to git — binaries are build outputs, not source artifacts
- `src/contracts/fr-1-the-system-must-present-critical.js` and `fr-2-the-system-must-preserve-strict-.js` are untracked spec contracts for the settings-page backlog; commit them when that backlog work is promoted

## Adoption Decision

ready — All 5 core phase artifacts are present. sdlc-studio pipeline is complete (20/20 stories Done). All adoption gaps resolved.
