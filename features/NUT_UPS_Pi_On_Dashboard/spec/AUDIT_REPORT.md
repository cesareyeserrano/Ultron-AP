# AUDIT_REPORT — NUT_UPS_Pi_On_Dashboard

### Requirements Coverage

Independent re-derivation of every client-expressed need (REQ doc §1–§7 + the two
owner chat decisions) traced to the generated FRs/NFRs/no_go_zone. The coverage_map
in 01_REQUIREMENTS.json was NOT trusted; it was diffed against this trace afterward.

**[GAP-1]** `PARTIAL` — Read-only display of the safe-shutdown config must include **NUT's battery shutdown-trigger threshold**, not only the two delays.
- Source: the seed brief (FEATURE_IDEA.md → `01_REQUIREMENTS.json#original_brief`), New Behavior:
  _"mostrar (solo lectura) la configuración de apagado seguro del UPS/NUT: los delays del UPS (`ups.delay.shutdown`, `ups.delay.start`) **y el umbral de batería con que NUT dispara el apagado**, claramente etiquetados como 'gestionado por NUT'."_
  (Reaffirmed as an explicit owner chat decision: display of `ups.delay.shutdown`, `ups.delay.start`, **and NUT's battery trigger threshold**, labelled "gestionado por NUT".)
- Status: PARTIAL. FR-023 covers the two delays (AC-023-001 renders `ups.delay.shutdown` / `ups.delay.start`; AC-023-003 handles a missing variable). The **third named element — the battery threshold at which NUT triggers shutdown — appears only in FR-023's prose**; it has no acceptance criterion and, more importantly, no data source: it is NOT among the available NUT variables listed in `constraints` (`ups.status, ups.load, input.voltage, input.frequency, output.voltage, battery.voltage, ups.beeper.status, ups.delay.shutdown, ups.delay.start, ups.type`). That trigger lives in `upsmon.conf` / the UPS's `LB` flag, not in `LIST VAR`. As written, the design can render the delays but has no committed path to surface the shutdown-trigger threshold, so this sub-capability is at real risk of being silently dropped in implementation (or would require file access that tensions RS-2 / NFR-017).
- Action: re-open Phase 1 to either (a) add an acceptance criterion + data-source decision for the NUT shutdown-trigger threshold under FR-023 (e.g. surface it from a config-read path, or explicitly display "NUT dispara al recibir LB" when no numeric threshold is published), OR (b) record an explicit out-of-scope decision narrowing FR-023 to the two published delays and removing the threshold from the FR-023 description so the promise matches the deliverable.

---

**Minor observation (not a formal gap):** RF-2 lists **"Estado del beeper"** as a card element. FR-017's *description* includes beeper state, so the need is covered at the requirement level, but FR-017's acceptance_criteria omit any beeper assertion (AC-017 covers status mapping, load/input/battery-voltage units, estimado label, 'Sin datos', SSE, mobile, escaping — no beeper). Requirement-covered; AC-level test coverage for beeper display is thin. Flagged for awareness, not counted as an UNCOVERED/PARTIAL requirements gap.

---

#### What was traced (completeness evidence)

Every distinct need, with its disposition:

| # | Need (source) | Disposition |
|---|---|---|
| 1 | Read UPS + show + alert; scope = monitoring/history/alerts, not replace shutdown (§1) | Feature-wide + no_go_zone |
| 2 | Battery % must be estimated, interpolate 21.0–27.4 V, always "estimado" (§2, RF-2) | FR-018 ✓ |
| 3 | Runtime minutes cannot be shown honestly → excluded (§2) | no_go_zone ✓ (out of scope) |
| 4 | Estimate is orientative, always labelled (§2) | FR-018 ✓ |
| 5 | Native NUT client, TCP 127.0.0.1:3493, LIST/GET VAR, pure Go, no `exec upsc` (RF-1) | FR-016 ✓ |
| 6 | Dedicated Ultron read-only NUT user, not `homeassistant` (RF-1/RS-1) | NFR-019 ✓ |
| 7 | Poll every 10 s (configurable), backoff reconnect, unreachable is a valid state (RF-1) | FR-016 + FR-024 ✓ |
| 8 | Card: translated status OL/OB/LB/OL CHRG/RB/OFF/BYPASS/ALARM (RF-2) | FR-017 (AC-017-001 all 8) ✓ |
| 9 | Card: load %, input voltage, battery voltage + estimated % (RF-2) | FR-017 ✓ |
| 10 | Card: beeper state (RF-2) | FR-017 desc ✓ (AC thin — see note) |
| 11 | Card: explicit "Sin datos" when unreachable (RF-2) | FR-017 (AC-017-003) ✓ |
| 12 | Live update via existing SSE channel (RF-2) | FR-017 (AC-017-005) ✓ |
| 13 | Persist samples in SQLite `ups_samples`, reuse internal/metrics; series input.voltage/battery.voltage/ups.load/status (RF-3) | FR-019 ✓ |
| 14 | Retention 30 d + auto purge (RF-3) | FR-019 + FR-024 ✓ |
| 15 | 24 h / 7 d charts (RF-3) | FR-019 ✓ |
| 16 | Outage event log: OB opens, OL closes, start/end/duration (RF-3) | FR-020 ✓ |
| 17 | Alert: OB → Warning immediate (RF-4) | FR-021 AC-021-001 ✓ |
| 18 | Alert: LB → Crítico immediate (RF-4) | FR-021 AC-021-005 ✓ |
| 19 | Alert: OL return → Info with outage duration (RF-4) | FR-021 AC-021-002 ✓ |
| 20 | Alert: battery.voltage near 21.0 V → Crítico (RF-4) | FR-021 acceptance_criteria ✓ |
| 21 | Alert: input.voltage outside ~100–140 V → Warning debounced (RF-4) | FR-021 AC-021-003 ✓ |
| 22 | Alert: RB → Warning once/day (RF-4) | FR-021 AC-021-004 ✓ |
| 23 | Alert: UPS unreachable > 2 min → Warning once (RF-4) | FR-021 AC-021-006 ✓ |
| 24 | All alerts debounced/deduplicated (RF-4) | FR-021 ✓ |
| 25 | Insights: outage count/week, battery degradation from resting-voltage drop (RF-5) | FR-022 ✓ |
| 26 | RF-6 beeper/battery-test via upscmd — optional P2 | no_go_zone ✓ (out of scope) |
| 27 | Prohibited: any shutdown.* / load.off command (RF-6) | NFR-018 ✓ |
| 28 | RS-1 dedicated read-only credential, password in env, encrypted at rest, never in repo | NFR-019 ✓ |
| 29 | RS-2 no new privileges, no ultron-helper | NFR-017 ✓ |
| 30 | RS-3 clean degradation, gated by ULTRON_UPS_ENABLED | NFR-016 + FR-024 ✓ |
| 31 | RS-4 escape all NUT input before render (no innerHTML XSS) | NFR-019 ✓ |
| 32 | RS-5 tests: NUT parser + state mapping vs simulated upsd, in CI | NFR-021 ✓ |
| 33 | §5 out of scope (replace shutdown / runtime minutes / multi-UPS / HA duplication) | no_go_zone ✓ |
| 34 | §6 acceptance criteria (15 s Telegram, restore w/ duration, Sin datos, estimado, go test) | FR-017/018/020/021 + NFR-021 ✓ |
| 35 | Chat decision: read-only shutdown config — `ups.delay.shutdown` + `ups.delay.start` | FR-023 ✓ |
| 36 | Chat decision: read-only shutdown config — **NUT battery trigger threshold** | FR-023 **PARTIAL → GAP-1** |
| 37 | Chat decision: 4 configurable params (alert thresholds, poll interval, battery range, retention) | FR-024 ✓ |

**Verdict:** 37 needs traced — 34 covered by an FR/NFR, 6 of those correctly disposed to no_go_zone (out of scope), 1 PARTIAL (GAP-1), 0 fully UNCOVERED. No wrongly-excluded out-of-scope items and no hollow coverage-map entries found; the agent's coverage_map matched my independent trace except it recorded the shutdown-config display as fully covered by FR-023 without surfacing the battery-trigger-threshold sub-gap.

---

### Gap Resolution (post-audit, 2026-07-15)

- **GAP-1 (PARTIAL, FR-023) — RESOLVED.** Owner decision: instead of reading the trigger from `upsmon.conf` (which would break RS-2/NFR-017 — no new privileges), the shutdown-config card now displays the **configured low-battery cutoff of 21.0 V** as the "punto de apagado — gestionado por NUT", sourced from the same configured bound as FR-018/FR-024. Added to FR-023 description + acceptance_criteria and to US-023 as **AC-023-004**. The `upsmon.conf` trigger threshold stays out of scope.
- **Minor note (beeper display) — RESOLVED.** Added beeper state to FR-017 acceptance_criteria and US-017 as **AC-017-005** (`ups.beeper.status`).

Re-ran `aitri feature complete NUT_UPS_Pi_On_Dashboard 1` — gate passes.
