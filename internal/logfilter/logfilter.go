// Package logfilter sanitises and bounds log output that the privileged
// ultron-helper ships back to the unprivileged web process. The helper
// runs journalctl/dmesg/ps on behalf of the panel, and the output of
// those commands can carry env-style secrets (TOKEN=…, PASSWORD=…),
// Authorization bearer tokens, JWTs, and arbitrarily large blobs.
// Anything reaching the web process can reach the browser, so the
// boundary needs both a redaction pass and a hard byte cap.
package logfilter

import (
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
	if policy == PolicyJournal {
		out = redactJournal(out)
	}

	if len(out) <= maxBytes {
		return out
	}

	// Keep the tail. Reserve a budget for the marker so the final
	// length is <= maxBytes.
	const markerTemplate = "... [truncated %d bytes from start]\n"
	dropped := len(out) - maxBytes
	marker := fmt.Sprintf(markerTemplate, dropped)
	keep := maxBytes - len(marker)
	if keep < 0 {
		keep = 0
	}
	tail := out[len(out)-keep:]
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

	// key=value or key: value where key smells like a secret. The
	// trailing capture is a non-quoted, non-whitespace run; common
	// log layouts use this form. We deliberately match neither
	// surrounding quotes nor commas in the value to avoid eating
	// structured-log delimiters.
	envRe = regexp.MustCompile(`(?i)\b(token|secret|password|passwd|api[_\-]?key|apikey|access[_\-]?key|auth(?:orization)?)\b\s*[:=]\s*\S+`)
)

func redactJournal(input []byte) []byte {
	out := jwtRe.ReplaceAll(input, []byte("[REDACTED-JWT]"))
	out = bearerRe.ReplaceAll(out, []byte("Bearer [REDACTED]"))
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
