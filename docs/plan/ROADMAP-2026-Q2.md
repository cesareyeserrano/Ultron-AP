# Ultron Roadmap — 2026 Q2

> Strategic sequencing of the four next initiatives discussed
> 2026-05-05 after the network-monitoring feature shipped.
> Ordered by **leverage**, not by size. Each item below is also
> tracked as a backlog ID for `aitri backlog`.

---

## Sequencing rationale

Bundling all four initiatives into one mega-feature would block shipping
for 4-6 weeks with high scope-creep risk. The sequence below lets each
phase ship independently in 1-2 weeks; you can stop between any two
without losing value already delivered.

The pivotal call: **F2 (insights-engine) is the highest-leverage piece**.
Built first, every later module gains diagnostic capability for free by
adding rules; `/help` becomes the central documentation surface; `F4`
later exposes rule thresholds as proper settings — everything composes.
Doing UI rework (F3/F4) before F2 just rearranges cards while the deeper
problem ("Ultron tells you *what* is happening, not *what it means*")
stays intact.

---

## F1 — `lan-devices` *(BL-016)*

Discover and track devices on the LAN via 5-min ICMP sweep over the
local /24 + `ip neigh` parsing + OUI vendor lookup. Persisted in
SQLite with online/offline + last-seen. UI lives at `/network`
(replaces the WAN-events-only stub left after BG-016).

- **Scope v1:** IP, MAC, vendor, online state, last seen.
- **Out of scope v1:** per-device WiFi signal (router cooperation
  required, not feasible from a guest host on Ethernet); per-device
  bandwidth (same constraint); mDNS hostname enrichment (v2).
- **Cost:** ~250 ARP packets in 3s every 5 min, no apt-installs (uses
  the unprivileged ICMP socket already enabled via `ping_group_range`,
  same path as `gatewayprobe`).
- **Estimated effort:** 1-2 days end-to-end.
- **When to start:** next.

---

## F2 — `insights-engine` *(BL-017)* + `help-page` *(BL-018)*

Declarative rules engine that turns telemetry into verdicts.

**Rule shape:**
```
condition (over telemetry vars)  →  verdict + severity + recommendation
```

**Examples:**
| Condition | Verdict | Severity |
|---|---|---|
| `cpu ≥ 90 ∧ temp ≥ 75` | Thermal throttling probable, kill heavy loads | critical |
| `gateway=ok ∧ cloudflare=fail` | LAN ok, ISP / WAN-link down | critical |
| `ram ≥ 90 ∧ swap > 0` | Memory pressure, inspect processes | warning |
| `services.failed > 0` | One or more systemd units failed | critical |
| `wan.flapping_5min ≥ 3` | Unstable WAN, possible ISP issue | warning |

**Surface:** verdicts render in the dashboard's existing "Operational
Indicators" section and stream over SSE. Each verdict deep-links to its
definition in `/help`.

**`/help` (BL-018):** glossary + rule documentation in **two voices**
— *technical* (what the metric measures, thresholds, source path) and
*non-technical* (what it means in plain language). Bootstrap content:
gateway/cloudflare/dns probe semantics, WAN state machine,
jitter/loss/RTT, CPU/RAM/temp thresholds, services/containers health,
VPN peers.

- **Estimated effort:** 1-2 weeks for the engine + 3-5 days for `/help`.
- **When to start:** after F1 ships.

---

## F3 — `ui-audit` *(BL-019)*

**Analysis task, not build.** Walk every page (dashboard, network,
services, docker, alerts, history, logs, settings, login) and produce a
prioritized punch-list of:

1. Layout inconsistencies
2. Card disposition / visual hierarchy issues
3. Duplicated information across pages
4. Missing pickers / poor input ergonomics (especially settings)
5. Responsive breakpoints

**Out of scope:** visual redesign — design tokens stay as-is.

**Output:** `backlog/ui-audit-2026-Q2.md` — markdown punch-list, no code.
The list feeds concrete tickets into F4 and any focused UI cleanups.

- **Estimated effort:** 2-3 days.
- **When to start:** after F2 ships (so the audit can also evaluate
  verdict surfacing).

---

## F4 — `settings-revamp` *(BL-020)*

Extends the existing `backlog/settings-page-ui-improvements/`
seed. Settings is the weakest UI today: raw inputs, no pickers, minimal
validation, weak hierarchy.

**Adds:**
- Typed input pickers (time, intervals, thresholds, color-coded severity
  selectors).
- Client + server validation with inline error states.
- Sectioned navigation with search/filter.
- First-class exposure of `insights-engine` (F2) rule thresholds as
  proper settings — closes the loop with F2.

**Sequenced AFTER `ui-audit`** so concrete prioritization comes from
the audit's punch-list.

- **Estimated effort:** 2-3 weeks.
- **When to start:** after F3 punch-list lands.

---

## Open bugs in flight (not features, but planned)

- **BG-017** — Recurring `SQLITE_BUSY` on net sample inserts
  (~10/min sustained on the Pi). Pre-existing; surfaced during
  BG-016 deploy on 2026-05-05. Likely net-probe writer competing
  with alert-engine for the SQLite handle. Investigate with
  PRAGMA `busy_timeout` / WAL mode review or a single-writer
  serialization layer.

---

## Decision log

- **2026-05-05** — Sequence agreed: F1 → F2 → F3 → F4. User
  confirmed "documentar como backlog primero, normalizar lo desplegado,
  luego features con plan claro". This document is the result.
- **2026-05-05** — F1 implementation choice: option 2 (Go-native ICMP
  sweep, no apt-installs). Rejected option 3 (`arp-scan` + `avahi`)
  because it would require a new privileged endpoint in `ultron-helper`
  for raw sockets — cost not justified by the marginal gain (mostly
  mDNS-derived friendly names).
