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

// M6 — a quoted secret value containing spaces must be redacted whole; the
// old \S+ capture stopped at the first space and leaked the remainder.
func TestFilter_PolicyJournal_RedactsQuotedSecretWithSpaces(t *testing.T) {
	in := []byte(`app: PASSWORD="hunter2 correct horse" started` + "\n")
	got := string(Filter(in, PolicyJournal, 0))
	if strings.Contains(got, "correct horse") {
		t.Fatalf("quoted secret leaked: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %q", got)
	}
}

// M6 — password inside a connection string must be redacted while keeping the
// non-secret scheme/user/host for debuggability.
func TestFilter_PolicyJournal_RedactsConnectionStringPassword(t *testing.T) {
	in := []byte("app: dsn=postgres://admin:s3cr3tpw@db.internal:5432/app\n")
	got := string(Filter(in, PolicyJournal, 0))
	if strings.Contains(got, "s3cr3tpw") {
		t.Fatalf("connection-string password leaked: %q", got)
	}
	if !strings.Contains(got, "db.internal") {
		t.Fatalf("non-secret host should be preserved, got %q", got)
	}
}

// M6 — additional keyword coverage.
func TestFilter_PolicyJournal_RedactsPassphrase(t *testing.T) {
	in := []byte("app: passphrase=letmein credential: topsecret\n")
	got := string(Filter(in, PolicyJournal, 0))
	if strings.Contains(got, "letmein") || strings.Contains(got, "topsecret") {
		t.Fatalf("passphrase/credential leaked: %q", got)
	}
}

// M7 — a very large journal payload is capped and still redacted at the tail.
func TestFilter_PolicyJournal_LargeInputCappedAndRedacted(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100000; i++ {
		b.WriteString("filler line to grow the payload well beyond the cap\n")
	}
	b.WriteString("app: TOKEN=leakme\n")
	got := Filter([]byte(b.String()), PolicyJournal, 64*1024)
	if len(got) > 64*1024 {
		t.Fatalf("output not capped: %d bytes", len(got))
	}
	if strings.Contains(string(got), "leakme") {
		t.Fatalf("tail secret not redacted after cap: %q", string(got)[len(string(got))-80:])
	}
}

// M7 regression — a pre-trimmed large payload must still carry the truncation
// marker so callers know the log was cut (previously lost).
func TestFilter_PolicyJournal_LargeInputKeepsTruncationMarker(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100000; i++ {
		b.WriteString("filler line to grow the payload well beyond the cap\n")
	}
	got := Filter([]byte(b.String()), PolicyJournal, 64*1024)
	if len(got) > 64*1024 {
		t.Fatalf("output not capped: %d bytes", len(got))
	}
	if !strings.Contains(string(got), "truncated") {
		t.Fatalf("truncation marker missing from pre-trimmed large input:\n%s", string(got)[:120])
	}
}

// M6 — password-only connection string (empty user) must also be redacted.
func TestFilter_PolicyJournal_RedactsCredentialOnlyURL(t *testing.T) {
	got := string(Filter([]byte("app: url=redis://:s3cr3t@cache.internal:6379/0\n"), PolicyJournal, 0))
	if strings.Contains(got, "s3cr3t") {
		t.Fatalf("credential-only URL password leaked: %q", got)
	}
	if !strings.Contains(got, "cache.internal") {
		t.Fatalf("host should be preserved: %q", got)
	}
}
