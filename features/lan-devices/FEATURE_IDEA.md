## Feature

Discover and track devices on the local LAN via periodic ICMP sweep + ARP-cache read + OUI vendor lookup, surfaced in `/network`.

## Problem / Why

Ultron's network visibility today ends at the WAN edge: gateway, cloudflare, and DNS probes confirm the uplink works, but there is zero view of *what is on the LAN*. An operator running a Pi as a household admin panel cannot answer basic questions ("is the printer up?", "what device just appeared?", "is anything unfamiliar on my network?") without leaving Ultron and SSHing into the router. F1 of the 2026-Q2 roadmap closes this gap with the cheapest possible discovery method that does not require router cooperation.

## Target Users

Existing Ultron operators (home / small-office). No new user types — same admin who already uses the dashboard for system + WAN telemetry now also sees per-device LAN state in the same surface.

## New Behavior

The system must:

- Detect the local IPv4 subnet automatically from the default-route interface (no manual configuration).
- Run an ICMP sweep across the local /24 every 5 minutes, rate-limited so the burst stays under ~3 seconds (~250 packets).
- Read the ARP cache (`/proc/net/arp`) immediately after each sweep and pair responding hosts with their MAC addresses.
- Resolve each MAC's vendor from a bundled IEEE OUI prefix table (no network lookup at runtime).
- Persist each observed device with: IP, MAC, vendor, first_seen, last_seen, current online state.
- Mark a device as `offline` when it has been absent from the last sweep but present within the last N sweeps (configurable, default 3).
- Expose the device list via JSON API and render it on `/network` (table view: IP, MAC, vendor, online state, last-seen relative time).
- Stay entirely unprivileged — must use the same `ping_group_range`-enabled ICMP path as `gatewayprobe`, no `setcap`, no `ultron-helper` extension, no apt installs.

## Success Criteria

- **Given** the Pi is on `192.168.1.0/24` with at least 5 reachable devices (router, laptop, phone, printer, smart bulb), **when** the operator visits `/network` after 6 minutes of uptime, **then** the table lists at least those 5 devices with vendor populated for ≥80% of them.
- **Given** a device drops off the network, **when** 3 sweeps elapse without an ICMP response from it, **then** its row flips to `offline` and `last_seen` shows the timestamp of the last successful sweep.
- **Given** a brand-new device joins the LAN, **when** the next sweep runs, **then** its row appears with `first_seen ≈ now` and `online = true`.
- The full sweep + DB write cycle on the Pi consumes <2% CPU averaged over 5 minutes (negligible impact on existing telemetry).
- No regression in existing 45 TC pass count.

## Out of Scope

- Per-device WiFi signal strength (requires router cooperation; not feasible from a guest Ethernet host).
- Per-device bandwidth or traffic accounting (same constraint).
- mDNS / Bonsoir-derived friendly hostnames (deferred to v2 — adds an avahi dependency).
- IPv6 neighbor discovery (the household LANs Ultron targets are still IPv4-NAT in 2026; revisit once IPv6 visibility justifies the cost).
- Active fingerprinting (TCP/SYN, banner grabs) — stays passive + ICMP-only for v1.
- Per-device alerting / notifications — that is `insights-engine` (BL-017) territory; the lan-devices feed becomes one more telemetry source for it later.
