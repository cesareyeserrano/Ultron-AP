# Ultron-AP — Audit Report

## Security — consolidated review record (2026-07-14)

The security posture was reviewed in **three independent passes** with different scopes and lenses. This document consolidates all three. Threat model throughout: a single-admin, LAN/Tailscale, plain-HTTP-by-design tool with privilege separation. The threats that matter — an **unauthenticated LAN device** reaching data or actions, the **root-helper boundary** (privilege escalation), **secrets leaking**, and **supply chain**.

| # | Pass | Tool | Scope | Result |
|---|---|---|---|---|
| 1 | Branch-change security review | Claude `/security-review` | Only the diff of this branch (the session's new code) | **0 findings** — the changes introduced no vulnerability |
| 2 | Adversarial security audit | `aitri audit security` | Whole project — static (code, git history, deps, deploy) **and** runtime (the deployed Pi) | **6 P2** (2 MEDIUM, 4 LOW) + 1 accepted dependency risk |
| 3 | Secret-exposure hunt | manual, on owner's prompt | Repo tree + full git history + Pi env file + journal + rendered pages | **0 exposed secrets** |

The three do not contradict — they layer. Pass 1 answered "did we break anything?" (no). Pass 2 answered "what does the whole deployed product expose?" (six minor items). Pass 3 answered "is any real key exposed anywhere?" (no). The 6 findings are all **hardening / defence-in-depth** (nothing is broken behaviour), filed as backlog items **BL-035..BL-040**.

### Pass 1 — Branch-change security review (`/security-review`) — 0 findings

Reviewed only the diff of `claude/code-review-adversarial-22o5na` (the session's new features and fixes). Traced every new data flow from user input to sensitive sink. Verified clean: path traversal in the backup download and log drawer, command/option injection into the helper's `journalctl`, the widened secret-redaction regex (strictly more inclusive — a fix, not a bypass), SQL in the new `mute`/`digest`/`hardware` layers (all parameterised), HTML/header injection in the digest email (every attacker-influenceable field `html.EscapeString`d), authorization + CSRF on the four new routes, and template auto-escaping in the new partials. **No vulnerability introduced by the branch.**

### Pass 3 — Secret-exposure hunt — 0 exposed secrets

Prompted by the owner's suspicion that a key was exposed somewhere neither prior pass looked. Checked, all clean:
- **Working tree** — no Telegram-token shapes, no PEM blocks, no non-placeholder secret assignments.
- **Full git history** (388 commits) — pickaxe on secret env vars + Telegram-token regex over all blobs; no `.env`/`.db`/`.pem`/`credentials.json` ever committed (even later-deleted).
- **Aitri spec JSON + feature `.env.example`** — only empty templates / placeholders (`change-me-…`).
- **Test files** — only obvious dummies (`abc123`, `123:abc`).
- **systemd units in `deploy/`** — the secret is referenced via `EnvironmentFile=-/etc/ultron-ap/ultron-ap.env`, **not** inlined as `Environment=`.
- **The running Pi** — `/etc/ultron-ap/ultron-ap.env` is `-rw------- root root` (not readable without privilege); the live unit has no inline key; the journal shows no key leak.
- **Rendered pages** — the backup `encryption_key_ref` field renders a *reference* (`env:NAME`), not key material; `maskNotifConfig` masks `bot_token`/`smtp_password` to the last 4 chars before they reach the browser.
- **High-entropy sweep** — no unexplained 32+ char hex/base64 blobs in code, env, or scripts.

Note: a throwaway dev key (`ULTRON_SECRET_KEY=0123…`) was typed into local dev-server commands this session — it is not the production key, is worthless outside the local test box, and is not committed anywhere. (Known minor, deferred: `maskNotifConfig` reveals the last 4 chars of masked secrets — BG-015-class, unchanged.)

### Pass 2 — Adversarial security audit (`aitri audit security`)

Defensive, passive, non-destructive review of **both** surfaces: static (source, repo history, dependencies, build/deploy config) and runtime (the deployed panel at `http://192.168.1.29:8080`).

### Surfaces covered / not reached

- **Static — covered:** git history secret scan (388 commits, pickaxe on secret env vars + Telegram-token regex over all blobs); `.gitignore`; `go.mod`; the full route table + `middleware.go`; `cmd/ultron-helper/main.go` (the root boundary); `internal/database/{secrets,backup_path,sqlite}.go` + a SQL sweep over all of `internal/database`; `backup_crypto.go`; `handlers_{auth,backup_files,service_logs,csp}.go`; `config.go`; `logfilter.go`; `deploy/*.service` + `deploy/ultron-ap.sudoers`; `web/templates/**` + `web/static/js/**` info-disclosure grep.
- **Runtime — covered:** auth gating on pages and `/api/*`, security headers, cookie flags, `/static/` behavior, path-normalization negatives, error-path verbosity, `/version` + `/health`, what ships to the browser.
- **NOT reached:** `govulncheck` was not run live (not installed on the audit host); dependency status is reported from `go.mod` inspection + the known-accepted docker advisories. No builds or tests were executed against production.

### Verdict

**0 P0 · 0 P1 · 6 P2** (2 MEDIUM, 4 LOW) + 1 accepted-risk dependency observation.

This is a **hardened, privilege-separated codebase**. Every crown-jewel boundary was verified solid with cited evidence: the root helper IPC (SO_PEERCRED fail-closed, allow-listed unit names, `--` argument guards, no shell), SQL (fully parameterised), backup crypto (streaming AES-GCM, `crypto/rand`, AAD binding chunk index + final flag against reorder/truncation), session/auth (bcrypt, HttpOnly/SameSite cookies, per-IP brute-force lockout, CSRF on every mutation), path handling (basename + prefix + EvalSymlinks containment), and secret redaction. No secret was ever committed to history. The six findings are all **hardening / defence-in-depth** — none is broken behaviour, which is why they are backlog items, not bugs.

---

### RQ-SEC-001 — CSP ships Report-Only by default, and `script-src` allows `'unsafe-inline'`  · **MEDIUM · P2**

- **CWE-693** Protection-Mechanism Failure. Confirmed both statically and at runtime.
- **Evidence:** `internal/server/middleware.go:96-101` sets `Content-Security-Policy-Report-Only` (advisory — enforces nothing) unless `ULTRON_CSP_ENFORCE=1` (`config.go:149-152`, default off). Live: every response carries `Content-Security-Policy-Report-Only: … script-src 'self' 'unsafe-inline' …`. Even once enforced, `'unsafe-inline'` in `script-src` neuters its XSS value.
- **Attack scenario:** the default deployment would not block a reflected/stored XSS (none found — templates use `html/template` auto-escaping) because the policy is not enforced; an injected inline `<script>` would run even after the flip. Defence-in-depth: no attacker without a separate XSS primitive.
- **Fix / AC:** after the reporting soak, default `CSPEnforce=true` and drop `'unsafe-inline'` from `script-src` (nonce/hash the few inline scripts in `base.html`). Responses then carry enforcing `Content-Security-Policy` with no `'unsafe-inline'` in `script-src`.

### RQ-SEC-002 — `deploy/ultron-ap.sudoers` is a parallel root path that bypasses the helper; `pironman5 *` grants arbitrary root args  · **MEDIUM · P2**

- **CWE-250** Execution with Unnecessary Privileges / **CWE-88** argument injection. Verified by reading the file.
- **Evidence:** `deploy/ultron-ap.sudoers` grants user `ultron` `NOPASSWD` for pinned `systemctl`/`shutdown`/`journalctl` (fine) **and `Cmnd_Alias ULTRON_PIRONMAN = /usr/local/bin/pironman5 *`** — a trailing wildcard = any arguments to a root binary. The Go code never invokes `sudo` (the only reference, `internal/systemd/controls.go:97`, is an error-string check); all privileged actions go through the validated root helper over the socket. Sharpening the point: the panel does **not** actuate the Pironman hardware at all (that was deliberately left out of the hardware feature), so this entry grants root-arbitrary-args to a binary the panel never calls.
- **Attack scenario:** if the unprivileged web process were ever compromised (RCE) **and** this sudoers file is installed, the attacker runs `sudo pironman5 <arbitrary args>` and `sudo shutdown`, sidestepping every guard in `cmd/ultron-helper`. The whole point of the helper split is defeated. Mitigated by: install is a manual step (not wired into the Makefile/deploy), so the file is currently dormant.
- **Fix / AC:** delete `deploy/ultron-ap.sudoers` and any install step (the helper is the sole privilege channel); if Pironman control is ever needed, add a `pironman` action to the helper with an allow-listed argument set. No sudoers file ships a wildcard entry.

### RQ-SEC-003 — Backup encryption key has no length/entropy floor; bare SHA-256 as KDF  · **LOW · P2**

- **CWE-326** Inadequate Encryption Strength / **CWE-916** weak password hash. Verified by reading the code.
- **Evidence:** `internal/server/backup_crypto.go:49-63` — `backupKeyFromRef` derives the AES-256 key as `sha256.Sum256(raw)` with **no salt and no minimum-length check**. Contrast `internal/database/secrets.go:33-46`, which warns when `ULTRON_SECRET_KEY < 16` chars ("SHA-256 does not add entropy"). The backup key gets no equivalent guard — yet backups are uploaded to Telegram when a remote destination is configured.
- **Attack scenario:** an operator sets a short/guessable `ULTRON_BACKUP_KEY`; an attacker who obtains a `.db.enc` (e.g. from the Telegram chat) brute-forces it offline — SHA-256 is fast and unsalted. Requires a weak operator key + access to the ciphertext.
- **Fix / AC:** emit the same `< 16 char` startup/save warning for the backup key as for `ULTRON_SECRET_KEY`; ideally use a salted KDF (scrypt/argon2, salt in the file header). A short backup key produces a visible warning.

### RQ-SEC-004 — `/static/` serves a directory listing (Go autoindex), and an `.DS_Store` is embedded in the binary  · **LOW · P2**

- **CWE-548** Information Exposure Through Directory Listing. Confirmed at runtime; `.DS_Store` verified locally.
- **Evidence:** `internal/server/server.go:572-573` mounts a bare `http.FileServer` over the embedded static FS; a directory with no `index.html` gets Go's default autoindex. Live: `GET /static/` and `GET /static/js/` (both **unauthenticated** — static must be reachable to render `/login`) return the classic `<pre><a href=…>` listing enumerating every file, including a committed **`.DS_Store`**. Note: `.DS_Store` is in `.gitignore` (so not tracked), but `//go:embed static/*` reads the **filesystem**, not git — so it ships in the binary anyway.
- **Attack scenario:** an unauthenticated LAN device gets a free, complete map of the client-side attack surface (every widget/script filename) without guessing. Low impact, but it hands recon for free. Path-traversal negatives passed (`/static/../server.go` → 404, no source leak).
- **Fix / AC:** wrap the FileServer to return 404 on directory paths (an FS that denies non-file requests), and delete `web/static/.DS_Store` before build. `GET /static/` and `GET /static/js/` return 404/403 with no enumeration; a known file still serves 200; no `.DS_Store` in the binary.

### RQ-SEC-005 — Internal Aitri traceability IDs (FR-/BG-/AC-) ship in client HTML/JS comments  · **LOW · P2**

- **CWE-200** Information Exposure. Confirmed both statically and at runtime.
- **Evidence:** e.g. `web/templates/base.html:14` (`BG-038`), `web/templates/partials/settings-backup.html:94` (`FR-068`), `web/static/js/sidebar.js:3` (`BG-076`), `web/static/js/settings.js:2` (`FR-065`), `web/static/js/widgets/service-logs.js:1` (`FR-081`). No secrets, keys, or internal endpoints leak — only requirement/bug identifiers. The only `token` in `/login` is the legitimate per-request `csrf_token`.
- **Attack scenario:** an unauthenticated visitor to `/login` can read the internal SDLC issue taxonomy — a minor reconnaissance/social-engineering aid. No control depends on these being secret.
- **Fix / AC:** strip `FR-/BG-/AC-/TC-` references from client-shipped comments at build time (or a CI grep gate). Grep of all `/static/js/**` + shipped HTML returns zero such matches.

### RQ-SEC-006 — Unauthenticated `/version` discloses build commit + Go toolchain  · **LOW · P2**

- **CWE-200** Information Exposure. Confirmed both statically and at runtime.
- **Evidence:** `internal/server/handlers.go:21` `handleVersion` (public, `server.go:563`) returns `{"version","commit","go":runtime.Version()}` with no auth. Live: `{"commit":"a127f30d4ba4","go":"go1.25.11","version":"v1.0.0"}`.
- **Attack scenario:** an unauthenticated LAN/Tailscale peer fingerprints the exact build + Go toolchain to target known CVEs. Low impact on a single-admin box; aids targeting only. `/health` stays clean (`{"status":"ok"}`).
- **Fix / AC:** move `/version` behind `requireAuth`, or drop `commit`/`go` for anonymous callers. `/version` returns detailed build info only to an authenticated session.

### Observation — Dependency: `docker/docker v27.5.1+incompatible` (known accepted risk)

The CI `govulncheck` gate is red on **GO-2026-4887 / GO-2026-4883** (docker/docker v27), pending a v28 SDK bump; the stdlib side is cleared by Go 1.25.11. No *new* vulnerable pin was found by inspection (`golang.org/x/net v0.53.0`, `golang.org/x/crypto v0.50.0` current). `govulncheck` could not be run live. Treat as accepted risk, not a new finding.

### Categories verified clean (evidence, not assumption)

- **Secrets in repo/history** — no real credential ever committed; pickaxe hits are placeholders (`change-me-to-a-long-random-string`). `.gitignore` excludes `*.env`, `*.db*`, `credentials.json`, `node_modules/`, `bin/`.
- **Root-helper boundary** — SO_PEERCRED fail-closed on missing allowlist; `serviceNameRe` anchored to an alphanumeric first char; `--` before every unit name; `exec.CommandContext` with fixed argv (no shell); bounded timeouts + process-group kill; log sources allow-listed.
- **SQL injection** — every query parameterised; the sole `fmt.Sprintf` into SQL is `VACUUM INTO '%s'` (SQLite forbids bind params there) with a server-generated, validated, single-quote-escaped path — not attacker-controllable.
- **Path traversal (backup download / log drawer)** — bare-basename + `ultron-` prefix + EvalSymlinks containment (trailing separator closes the sibling-prefix bypass); unit names validated against the same allow-list the helper uses.
- **AuthN/Z** — every mutating/data route behind `requireAuth`; public set is exactly `/health`, `/version`, `/login`, `/api/csp-report`, `/static/`. Cookie `HttpOnly` + `SameSite=Lax` + conditional `Secure`; bcrypt DefaultCost; constant-time dummy hash on unknown user; CSRF on all mutations incl. logout.
- **Crypto** — AES-GCM streaming, `crypto/rand` everywhere (no `math/rand`), per-file nonce base + counter, AAD binds chunk index + final flag.
- **Secret redaction** — helper redacts JWT/bearer/`*token`/`*secret`/`*password`/conn-string before bytes cross the socket, re-applied panel-side (BG-073/074 fix widened this correctly).
- **Runtime headers** — `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Permissions-Policy` all present incl. on 404s. Absent HSTS/Secure-cookie is **intentional and correct** for a plain-HTTP LAN tool (both are conditional on `isHTTPSRequest`). Errors return the stdlib `404 page not found` — no stack traces or framework fingerprints.

### Proposed quality gate (`scripts/security-gate.sh`)

An exit-code check the project should declare as a `quality_gate` so `verify` re-checks the fixed posture every cycle — an audit that leaves no gate behind protects exactly once.

```bash
#!/usr/bin/env bash
# Re-checks the security-audit findings. Exit 0 = pass.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2
fail=0; note(){ echo "FAIL: $*"; fail=1; }

# Secrets: no secret-like file tracked; no committed Telegram-token shape
git ls-files | grep -Ei '\.(env|db|pem|key)$|credentials\.json' && note "secret-like file tracked"
git grep -nE '[0-9]{8,10}:AA[A-Za-z0-9_-]{30,}' -- ':!*.md' ':!*.example' && note "possible Telegram token committed"

# Root-helper invariants still present
grep -q 'serviceNameRe = regexp.MustCompile' cmd/ultron-helper/main.go || note "serviceNameRe allow-list missing"

# No new Sprintf-built SQL beyond the known VACUUM INTO
grep -rnE 'Exec\(|Query\(' internal/database/*.go | grep -i sprintf | grep -v 'VACUUM INTO' | grep -v _test.go \
  | grep . && note "new Sprintf-built SQL"

# RQ-SEC-002: sudoers must not ship a wildcard privilege entry
[ -f deploy/ultron-ap.sudoers ] && grep -qE 'pironman5 \*' deploy/ultron-ap.sudoers && note "sudoers ships pironman5 wildcard"

# RQ-SEC-004: no .DS_Store embedded in the binary
[ -f web/static/.DS_Store ] && note ".DS_Store present in embedded web/static"

# RQ-SEC-005: shipped assets must not leak Aitri trace IDs
git grep -lnE '(FR|BG|AC|TC)-[0-9]+' -- 'web/static/js/**' >/dev/null 2>&1 && note "Aitri trace IDs in client JS"

exit $fail
```
