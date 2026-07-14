// Package logfilter sanitises and bounds log output that the privileged
// ultron-helper ships back to the unprivileged web process. The helper
// runs journalctl/dmesg/ps on behalf of the panel, and the output of
// those commands can carry env-style secrets (TOKEN=…, PASSWORD=…),
// Authorization bearer tokens, JWTs, and arbitrarily large blobs.
// Anything reaching the web process can reach the browser, so the
// boundary needs both a redaction pass and a hard byte cap.
package logfilter

import (
	"bytes"
	"fmt"
	"regexp"
)

// Policy selects which redaction patterns apply.
type Policy int

const (
	// PolicyNone applies the size cap only. Suitable for ps output
	// where columns are explicitly chosen (pid, comm, %cpu, %mem) and
	// argv is never included, and for dmesg where kernel ring-buffer
	// content is mostly device-level metadata.
	PolicyNone Policy = iota

	// PolicyJournal applies env/secret/token redaction patterns plus
	// the size cap. Suitable for journalctl output of arbitrary
	// services whose logs may print environment values, configured
	// secrets, or upstream Authorization headers.
	PolicyJournal
)

// MaxBytes is the default response cap. journalctl with 500 lines of
// long messages can exceed several hundred KB; 256 KiB is roomy enough
// for real diagnostic value while still bounded for the IPC channel.
const MaxBytes = 256 * 1024

// Filter applies the redaction policy and then caps output to maxBytes,
// keeping the tail (most recent log lines) and prepending a truncation
// marker. maxBytes <= 0 means "use the package default".
func Filter(input []byte, policy Policy, maxBytes int) []byte {
	if maxBytes <= 0 {
		maxBytes = MaxBytes
	}

	out := input

	// M7: bound the redaction work. Redaction makes several full regex passes,
	// each allocating a copy, so running it over a multi-megabyte payload
	// spikes memory before the cap ever applies. Pre-trim very large inputs to
	// the tail on a line boundary first (we keep the tail anyway), so the regex
	// passes only ever see ~maxBytes. Cutting on '\n' avoids splitting a secret
	// mid-line at the trim point. preDropped remembers the bytes removed here so
	// the truncation marker below still reflects them.
	var preDropped int
	if policy == PolicyJournal && len(out) > 2*maxBytes {
		preDropped = len(out) - maxBytes
		out = out[len(out)-maxBytes:]
		if i := bytes.IndexByte(out, '\n'); i >= 0 && i+1 < len(out) {
			preDropped += i + 1
			out = out[i+1:]
		}
	}
	if policy == PolicyJournal {
		out = redactJournal(out)
	}

	// Fast path: nothing pre-trimmed and already under the cap.
	if preDropped == 0 && len(out) <= maxBytes {
		return out
	}

	// Keep the tail within maxBytes and prepend a marker reporting the total
	// bytes dropped from the start — both the pre-trim and any final overflow.
	const markerTemplate = "... [truncated %d bytes from start]\n"
	dropped := preDropped
	tail := out
	// Budget the marker with an upper-bound digit count so keep never has to
	// grow after we format the real count.
	marker := fmt.Sprintf(markerTemplate, dropped+len(out))
	keep := maxBytes - len(marker)
	if keep < 0 {
		keep = 0
	}
	if len(tail) > keep {
		dropped += len(tail) - keep
		tail = tail[len(tail)-keep:]
	}
	marker = fmt.Sprintf(markerTemplate, dropped)
	result := make([]byte, 0, len(marker)+len(tail))
	result = append(result, marker...)
	result = append(result, tail...)
	return result
}

// Redaction patterns. Order matters slightly — JWT before bearer so
// the JWT pattern matches the full eyJ…eyJ…sig form before bearer's
// looser \S+ would consume it.

var (
	// JWTs: header.payload.signature, all base64url. Anchored on the
	// distinctive "eyJ" header prefix so we don't mis-redact every
	// dotted identifier.
	jwtRe = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`)

	// "Authorization: Bearer xyz" / "bearer xyz" — case-insensitive on
	// the Bearer keyword; the token is anything that isn't whitespace.
	bearerRe = regexp.MustCompile(`(?i)\bbearer\s+\S+`)

	// key=value or key: value where key smells like a secret. The value
	// capture accepts a double/single-quoted string (so a secret containing
	// spaces is redacted whole — M6) or an unquoted non-whitespace run.
	// The leading [\w.\-]* matters: \b(token) does NOT match "bot_token=" —
	// there is no word boundary between "_" and "token", so a Telegram
	// bot_token, an api_token or a service_password sailed through unredacted.
	// Consuming any prefix run makes those cases match too (AC-081-003).
	envRe = regexp.MustCompile(`(?i)\b[\w.\-]*(?:token|secret|password|passwd|passphrase|credentials?|api[_\-]?key|apikey|access[_\-]?key|auth(?:orization)?)[\w.\-]*\s*[:=]\s*("[^"]*"|'[^']*'|\S+)`)

	// Password inside a URL/connection string: scheme://user:PASSWORD@host.
	// Redacts the password segment while keeping scheme, user and host.
	connStrRe = regexp.MustCompile(`(?i)([a-z][a-z0-9+.\-]*://[^:/@\s]*:)[^@/\s]+(@)`)
)

func redactJournal(input []byte) []byte {
	out := jwtRe.ReplaceAll(input, []byte("[REDACTED-JWT]"))
	out = bearerRe.ReplaceAll(out, []byte("Bearer [REDACTED]"))
	out = connStrRe.ReplaceAll(out, []byte("${1}[REDACTED]${2}"))
	out = envRe.ReplaceAllFunc(out, func(match []byte) []byte {
		// Keep the key portion visible (first run up to ':' or '='),
		// replace the value with [REDACTED] so logs remain useful for
		// debugging "is X being read at all?" but never expose the
		// actual secret.
		for i, b := range match {
			if b == '=' || b == ':' {
				prefix := match[:i+1]
				return append(append([]byte{}, prefix...), []byte(" [REDACTED]")...)
			}
		}
		return []byte("[REDACTED]")
	})
	return out
}
