package logfilter

import (
	"bytes"
	"strings"
	"testing"
)

func TestFilter_PolicyNone_PassesThroughWhenSmall(t *testing.T) {
	in := []byte("PID  COMM         %CPU %MEM\n  1 systemd        0.1 0.4\n")
	got := Filter(in, PolicyNone, 0)
	if !bytes.Equal(got, in) {
		t.Fatalf("PolicyNone must not alter content under the cap, got %q", got)
	}
}

func TestFilter_PolicyJournal_RedactsEnvStyleSecrets(t *testing.T) {
	in := []byte("Apr 23 10:00 host app[123]: starting with TOKEN=abc123 and PASSWORD=hunter2\n")
	got := Filter(in, PolicyJournal, 0)
	s := string(got)
	if strings.Contains(s, "abc123") || strings.Contains(s, "hunter2") {
		t.Fatalf("env-style secrets must be redacted, got %q", s)
	}
	// Key visibility preserved so operators can confirm "is TOKEN being read?"
	if !strings.Contains(s, "TOKEN=") || !strings.Contains(s, "PASSWORD=") {
		t.Fatalf("redaction should keep key prefix visible, got %q", s)
	}
}

func TestFilter_PolicyJournal_CaseInsensitiveKeys(t *testing.T) {
	in := []byte("token: foo\nApiKey=bar\nAccess_Key: baz\n")
	got := string(Filter(in, PolicyJournal, 0))
	for _, leaked := range []string{"foo", "bar", "baz"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("expected %q redacted, got %q", leaked, got)
		}
	}
}

func TestFilter_PolicyJournal_RedactsBearer(t *testing.T) {
	in := []byte("Authorization: Bearer eyJsupersecret.payload.sig\nfollowup line\n")
	got := string(Filter(in, PolicyJournal, 0))
	if strings.Contains(got, "supersecret") {
		t.Fatalf("bearer token must be redacted, got %q", got)
	}
}

func TestFilter_PolicyJournal_RedactsJWT(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.abc-DEF_123"
	in := []byte("issued token=" + jwt + " for sub=42\n")
	got := string(Filter(in, PolicyJournal, 0))
	if strings.Contains(got, jwt) {
		t.Fatalf("JWT must be redacted, got %q", got)
	}
	if !strings.Contains(got, "REDACTED") {
		t.Fatalf("expected REDACTED marker, got %q", got)
	}
}

func TestFilter_PolicyNone_DoesNotRedact(t *testing.T) {
	// Critical: ps output mentioning "password" in a process name must
	// not be redacted under PolicyNone. Only journals are mangled.
	in := []byte("PID COMM\n  1 password-manager\n")
	got := string(Filter(in, PolicyNone, 0))
	if !strings.Contains(got, "password-manager") {
		t.Fatalf("PolicyNone must not redact, got %q", got)
	}
}

func TestFilter_CapsAtMaxBytes(t *testing.T) {
	// Build a payload above the cap; assert the output is <= cap and
	// that the marker is present and the tail (most recent) is kept.
	const cap = 1024
	body := strings.Repeat("a", cap)
	tailMarker := "TAIL-MARKER-LINE\n"
	in := []byte(body + tailMarker)

	got := Filter(in, PolicyNone, cap)
	if len(got) > cap {
		t.Fatalf("output exceeds cap: %d > %d", len(got), cap)
	}
	if !strings.HasPrefix(string(got), "... [truncated") {
		t.Fatalf("missing truncation marker, got prefix %q", string(got)[:40])
	}
	if !strings.HasSuffix(string(got), tailMarker) {
		t.Fatalf("tail must be preserved (most recent log lines), got suffix %q", string(got)[len(got)-40:])
	}
}

func TestFilter_RedactionAppliedBeforeCap(t *testing.T) {
	// If a secret sits in the kept tail, redaction must still apply.
	// Realistic journal format: each entry is a separate line, so the
	// TOKEN= is at a word boundary.
	padding := strings.Repeat("filler line filler line filler\n", 80)
	tail := "Apr 23 10:00 host app[123]: TOKEN=topsecretvalue\n"
	in := []byte(padding + tail)
	got := string(Filter(in, PolicyJournal, 1024))
	if strings.Contains(got, "topsecretvalue") {
		t.Fatalf("secret leaked through cap, got %q", got)
	}
	if !strings.Contains(got, "TOKEN=") {
		t.Fatalf("expected key prefix preserved, got %q", got)
	}
}

func TestFilter_DefaultMaxBytes(t *testing.T) {
	// maxBytes <= 0 should fall back to package default; passthrough
	// for input smaller than the default.
	in := []byte("small payload")
	got := Filter(in, PolicyNone, 0)
	if !bytes.Equal(got, in) {
		t.Fatalf("expected passthrough under default cap, got %q", got)
	}
	if MaxBytes <= 0 {
		t.Fatalf("MaxBytes default must be positive, got %d", MaxBytes)
	}
}

func TestFilter_NoFalsePositiveOnNonSecretDottedTokens(t *testing.T) {
	// Common log content: file paths, version strings, hostnames. None
	// should be redacted under PolicyJournal.
	in := []byte("loaded /etc/ultron/config.yaml v1.2.3 at host pi.local\n")
	got := string(Filter(in, PolicyJournal, 0))
	if strings.Contains(got, "REDACTED") {
		t.Fatalf("non-secret content must not be redacted, got %q", got)
	}
}
