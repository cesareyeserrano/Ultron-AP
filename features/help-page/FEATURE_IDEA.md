## Feature
A new /help page that documents every Ultron metric, state, and insights-engine verdict in two parallel voices: technical (what the metric measures, source, thresholds, code path) and plain (what it means for the user in everyday words). Verdict cards on the dashboard deep-link via anchor IDs (e.g. /help#verdict-thermal-throttling) to the matching entry, closing the loop on the insights-engine's `links` field.

## Problem / Why
The insights-engine (BL-017) emits verdicts like "Thermal throttling probable" with recommendations, but a non-technical operator still has no in-product explanation of what "thermal throttling" actually means or how the threshold was chosen. Today's only references are buried in source comments and external docs. Ultron needs an authoritative, in-product glossary so any verdict, badge, or metric on the dashboard can be one click from a definition.

## Target Users
Existing Ultron operators. Same dashboard, same auth, same nav — adds one new left-nav item ("Help") and a new route /help. No new user type.

## New Behavior
- The system must serve a /help page (parent FR-007 auth) listing every glossary entry, grouped by category (System metrics, Network probes, Services & containers, VPN, Insights verdicts).
- Each entry has: stable anchor id, title, two-voice body (technical + plain), optional threshold table, optional source-code path link.
- Each insights-engine bundled rule's `links` field must reference a real anchor on /help (validated at startup — if a rule references a missing anchor, log a warning and continue; rule still loads).
- The dashboard's Operational Indicators verdict cards must add a "Learn more" link pointing at the rule's first valid anchor on /help.
- Glossary content lives as embedded JSON in the binary (go:embed), versioned with the code. No SQLite, no admin UI in v1.
- Bootstrap content covers: CPU/RAM/temp/disk thresholds, service & container states (active/failed/restarting), Docker container health, gateway/cloudflare/DNS probe semantics, WAN state machine (up/unknown/down), jitter/loss/RTT, Tailscale VPN peers, and one entry per insights-engine bundled rule (10).
- Page must support in-page client-side filter (typed search) — no JS framework, just a plain input that hides non-matching entries via CSS or a tiny inline script.

## Success Criteria
- Given the operator clicks "Learn more" on a thermal-throttling verdict card, when the browser navigates, then /help opens scrolled to the matching anchor with the entry visible without further scrolling on a 1280×800 viewport.
- Given the operator types "cpu" in the page filter, then only entries whose title or body mentions "cpu" remain visible within 100 ms.
- Given a developer adds a new bundled rule with `links: ["#verdict-foo"]` and `verdict-foo` is NOT defined in the glossary, when the binary starts, then a WARN log is emitted (the rule still loads — links are advisory, not required).
- The bootstrap glossary covers ≥30 entries spanning all parent FR domains plus the 10 insights-engine rules.

## Out of Scope
- Operator-authored glossary entries (admin UI) — v2.
- Multi-language localization — content is English-only in v1.
- Markdown editor / live preview — entries are JSON literals committed with code.
- Server-side full-text search index — client-side filter is sufficient at v1 scale (≤100 entries).
- Changelog / version history of glossary edits — git history is the record.
- /help/<id> deep-page routes — every entry lives on the same /help page anchored.
