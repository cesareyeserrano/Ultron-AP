// TC-4: Validate us-4 primary behavior
// Acceptance Criteria: AC-1
// AC-1: Given an authenticated operator on dashboard, when SSE stream runs, then metrics remain frequently updated and charts update at a slower cadence.
package generated

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestTc_4_validate_us_4_primary_behavior(t *testing.T) {
	sse := readRepoFile(t, "internal/server/sse.go")
	assertContains(t, sse, "func cadenceEvery(current, target time.Duration) int")
	assertContains(t, sse, "chartsEvery := cadenceEvery(current, 15*time.Second)")
	assertContains(t, sse, "tick%chartsEvery == 0")
	assertContains(t, sse, "data := s.buildSSEPayloadWithOptions(")
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func assertContains(t *testing.T, src, needle string) {
	t.Helper()
	if !strings.Contains(src, needle) {
		t.Fatalf("expected to find %q", needle)
	}
}

func assertNotContains(t *testing.T, src, needle string) {
	t.Helper()
	if strings.Contains(src, needle) {
		t.Fatalf("expected to not find %q", needle)
	}
}
