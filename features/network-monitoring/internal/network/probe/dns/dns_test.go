package dns

import (
	"crypto/rand"
	"regexp"
	"testing"
)

// labelPattern matches the "n" prefix + 16 lowercase hex chars produced by
// RandomCacheBypassLabel.
var labelPattern = regexp.MustCompile(`^n[0-9a-f]{16}$`)

// TestTC_NM_002e_CacheBypassLabelRandomised verifies that consecutive labels
// differ (so resolver caches do not return stale answers) and conform to the
// "n" + 16 hex pattern.
//
// @aitri-tc TC-NM-002e
func TestTC_NM_002e_CacheBypassLabelRandomised(t *testing.T) {
	t.Parallel()

	const samples = 32
	seen := make(map[string]struct{}, samples)
	for i := 0; i < samples; i++ {
		got, err := RandomCacheBypassLabel(rand.Reader)
		if err != nil {
			t.Fatalf("sample %d: unexpected error %v", i, err)
		}
		if !labelPattern.MatchString(got) {
			t.Fatalf("sample %d: label %q does not match pattern %s", i, got, labelPattern)
		}
		if _, dup := seen[got]; dup {
			t.Fatalf("sample %d: duplicate label %q after %d samples", i, got, len(seen))
		}
		seen[got] = struct{}{}
	}
}

// TestTC_NM_002e_CacheBypassLabelLetterLeading guards the RFC 1035 invariant
// that a DNS label cannot start with a digit. Regression-grade.
//
// @aitri-tc TC-NM-002e
func TestTC_NM_002e_CacheBypassLabelLetterLeading(t *testing.T) {
	t.Parallel()
	got, err := RandomCacheBypassLabel(rand.Reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatal("empty label")
	}
	first := got[0]
	if first < 'a' || first > 'z' {
		t.Fatalf("first char %q is not a lowercase letter (RFC 1035)", first)
	}
}
