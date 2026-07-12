# Adversarial code review — follow-up / continuation notes

Branch: `claude/code-review-adversarial-22o5na`

This file captures the work that was **intentionally left for the local project**
(needs a browser, is a protocol/design tradeoff, or is low-value polish), plus
the maintenance notes you need to continue. Everything else from the review was
implemented, tested, and pushed (13 commits, `go vet` clean, full suite green).

Delete this file once the items below are triaged into Aitri (`aitri bug add` /
`aitri feature init`) or resolved.

---

## 1. Pending: extract the settings.html inline `<script>` (remainder of CSS7)

**What.** `settings.html` still carries a ~339-line inline `<script>` block
(currently lines **191–529** of `web/templates/settings.html`). The six config
`<section>`s were already extracted into `partials/settings-*.html`; this script
is the last big chunk keeping the file large.

**Why it wasn't done here.** The block is pure JS (no `{{ }}` interpolation), so
it *can* move to `web/static/js/settings.js` referenced with
`<script src="/static/js/settings.js?v={{.AssetVersion}}">`. That would be a
clean ~339-line reduction and a CSP win. **But** the script runs inside the
`hx-boost`-swapped body, so moving it changes when/how it executes and binds on
navigation. The test suite only asserts rendered **HTML strings**, so it cannot
catch a runtime binding regression — this needs a real browser.

**How to do it locally (with a browser in the loop):**
1. Move lines 191–529 (the content between `<script>`/`</script>`, exclusive) of
   `web/templates/settings.html` into a new `web/static/js/settings.js`.
2. Replace the inline block in `settings.html` with:
   `<script src="/static/js/settings.js?v={{.AssetVersion}}" defer></script>`
   — but note: `<head>`-style loading won't work here because the script lives
   in the hx-boosted body. Check whether it should be `defer`, or wrapped in an
   idempotent init guard like the widgets in `web/static/js/widgets/*.js`
   (`if (window.__settingsBound) return; window.__settingsBound = true;`), since
   hx-boost re-inserts the body on every navigation.
3. `@source "../static/js/**/*.js"` is already in `input.css`, so any Tailwind
   classes the script toggles are already scanned. `AssetVersion` already hashes
   the whole `static/` tree, so cache-busting is automatic.
4. **Validate in a browser** (this is the part CI can't do): load `/settings`,
   then navigate away and back via the sidebar (exercises hx-boost re-init), and
   confirm every interaction still works:
   - the segmented/stepper/toggle/chip-preset widgets,
   - each form's save + the "saving…" busy state + the success/error toast,
   - the encryption-key `✓/✗` probe badge,
   - the destructive System Controls confirm/countdown flow,
   - the settings-section anchor/scroll behaviour.
5. Rebuild `app.css` is **not** needed for this (no CSS change).

**Aitri classification.** This changes no behaviour if done correctly, so it is a
"minor change" — but because it touches a functional surface and needs manual
verification, doing it under `aitri feature`/a proper verify pass is the safer
call.

---

## 2. Deferred-by-design review findings (not implemented, with rationale)

These were left out on purpose. Each line notes what a fix would look like if you
decide it's worth it.

- **M1 — authenticated blind SSRF via the email "Test" button.**
  `smtp_host`/`smtp_port` come straight from settings and are dialled with the
  result surfaced to the user. *Not fixed* because blocking private/loopback
  ranges would break legitimate internal SMTP relays (common on a Pi), and this
  is a single-admin tool. If you want defence anyway: return a generic
  "test failed" to the client while logging the detailed error server-side, so
  the reachability oracle is closed without blocking internal hosts.
  Sink: `internal/server/handlers_notifications.go:164`; dial:
  `internal/notify/email.go:112`.

- **B9 — ICMP reply "authentication" is a fixed public constant.**
  `gatewayprobe`/`sweep` accept replies whose payload equals `"ultron-ap"` /
  `"ultron-ap-sweep"`, spoofable by any LAN host. *Not fixed* because a real fix
  is a protocol change (per-probe random nonce echoed back and verified). Files:
  `internal/network/gatewayprobe/gatewayprobe.go`,
  `internal/network/landevices/sweep/icmp_transport.go`.

- **B10 — Tailscale peer names leave the package unmarked as untrusted.**
  `HostName`/`DisplayName`/`LoginName` from `tailscale status --json` flow to the
  UI. Currently safe because all templates use `html/template` (auto-escaped);
  this is a documentation/defence-in-depth note only. File:
  `internal/tailscale/status.go`.

- **B15 — secret masking reveals the last 4 chars** of `bot_token`/`smtp_password`
  in the settings placeholders. Standard practice; left as-is. File:
  `internal/server/handlers_settings.go` (`maskNotifConfig`).

- **CSS5 — JS↔class coupling.** `setAlertsFeedback` in `web/templates/base.html`
  toggles 6 literal Tailwind classes; a purge/rename could silently break it.
  Low value; would be cleaner as `.feedback-error`/`.feedback-idle` state classes
  in `@layer components`.

- **F1 — "weekly/biweekly ignores weekday": NOT a bug.** `BackupConfig` has no
  weekday field, so weekly == every 7 days at HH:MM by design. No action.

### Minor observations from the self-review (not bugs)
- **F5 checkbox edge:** `EncryptEnabled`/`Enabled` checkboxes still reset to false
  on a POST that omits them (standard HTML checkbox semantics; unchanged from
  `main`). Fully preserving them on a partial POST would need a hidden companion
  field per checkbox. `internal/server/handlers_performance.go`.
- **B1 dead code:** the auth middleware's `time.Now().After(session.ExpiresAt)` +
  `DeleteSession` branch is now unreachable (GetSession filters expired), but is
  kept as belt-and-braces. Harmless. `internal/server/middleware.go`.

---

## 3. Local maintenance notes

### Rebuilding `web/static/css/app.css`
`app.css` is a **committed build artifact**; edit `web/css/input.css` then rebuild.
- Normal path (repo's standalone binary): `make css`
  (`Makefile` calls `./tailwindcss -i web/css/input.css -o web/static/css/app.css --minify`).
- If the `./tailwindcss` standalone binary isn't present, the CLI moved to
  `@tailwindcss/cli` in v4. What was used on this branch:
  ```
  npm install --no-save --prefix /tmp/twbuild tailwindcss@4.1.18 @tailwindcss/cli@4.1.18
  ln -sfn /tmp/twbuild/node_modules ./node_modules   # resolver needs node_modules reachable from web/css
  /tmp/twbuild/node_modules/.bin/tailwindcss -i "$PWD/web/css/input.css" -o web/static/css/app.css --minify
  rm -f ./node_modules
  ```
  A rebuild of an unchanged `input.css` is byte-identical to the committed
  artifact (verified), so the toolchain is deterministic.
- CI idea: fail the build if `make css` produces a diff, so the artifact can
  never drift from the source.

### `.gitignore`
`node_modules` is **not** in `.gitignore`. If you rebuild CSS locally with npm,
add it so a stray `node_modules` can't be committed.

### Asset cache-busting
`?v=` on CSS/JS/icon links is now a content hash of the whole `static/` tree
(`computeAssetVersion` in `internal/server/server.go`), injected as
`{{.AssetVersion}}`. Editing any static asset auto-invalidates its cache — no
more hand-bumped version strings.

---

## 4. Aitri reconcile — commit → suggested registration

Each commit is a self-contained theme. Suggested mapping when you reconcile:

| Commit theme | Suggested Aitri entry |
|---|---|
| systemctl arg-injection / telegram token / brute-force UPSERT | `bug add` × 3 (security, high) |
| backup path TOCTOU / stream upload / logfilter / probe / session TTL | `bug add` × several (security, med) |
| lan-device bound / net underflow / docker pool / FD leak / startup panics | `bug add` × several (robustness) |
| toast XSS / !important / @source JS | `bug add` (XSS) + feature/minor (CSS) |
| partial-save / alert errors / hash-versioned CSS | feature/minor |
| session-in-context (D2) / backup snapshot (D1) | refactor → `aitri normalize --resolve` |
| a11y (reduced-motion / focus / scrollbar) | feature/minor |
| B-tier hardening (session expiry / weak-key / proxy / header+log inj / temp) | `bug add` × several (low) |
| device-count off lock / hash all assets | refactor + minor |
| HX-Trigger helper / dedupe handlers / shared shadow | refactor |
| self-review fixes (truncation marker / connstr redaction / sidebar version) | `bug add` (low) |
| settings.html section split | refactor → `aitri normalize --resolve` |
