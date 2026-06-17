// Module: ai/redact
// Purpose: Strip secret values and secret-shaped tokens from any text before it
//          enters a prompt sent to the external provider or an AI log line
//          (FR-023, NFR-005).
// Dependencies: regexp, strings.
package ai

import (
	"regexp"
	"strings"
)

const redactedMark = "[REDACTED]"

// secretShaped matches common secret token shapes that may appear in raw logs even
// when not present in the DB secrets list: JWTs, Bearer tokens, and long opaque
// hex/base64 runs. Kept deliberately conservative to avoid mangling normal text.
var secretShaped = regexp.MustCompile(
	`(?i)(bearer\s+[A-Za-z0-9._\-]{8,})` + // Bearer <token>
		`|(eyJ[A-Za-z0-9._\-]{10,})` + // JWT
		`|([A-Za-z0-9_\-]{40,})`, // long opaque token (40+ chars)
)

// Scrub removes every non-empty value in secrets (exact match) and then any
// secret-shaped token from text, replacing each with [REDACTED].
//
// @aitri-trace FR-ID: FR-023, US-ID: US-023, AC-ID: AC-023-1h, TC-ID: TC-AI-023h
func Scrub(text string, secrets []string) string {
	for _, s := range secrets {
		if strings.TrimSpace(s) == "" {
			continue
		}
		text = strings.ReplaceAll(text, s, redactedMark)
	}
	return secretShaped.ReplaceAllString(text, redactedMark)
}

// containsAny reports whether text contains any of the given non-empty needles —
// a helper used by tests asserting no secret leaked.
func containsAny(text string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(text, n) {
			return true
		}
	}
	return false
}
