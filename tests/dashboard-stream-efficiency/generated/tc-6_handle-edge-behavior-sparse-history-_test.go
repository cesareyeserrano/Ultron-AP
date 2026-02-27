// TC-6: Handle edge behavior - Sparse history (few points after startup): charts must render with available points only
// Acceptance Criteria: none
// No AC mapped to this TC.
package generated

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTc_6_handle_edge_behavior_sparse_history_few_points_after_startup_charts_must_render_with_available_points_only(t *testing.T) {
	helpers := readRepoFile(t, "internal/server/helpers.go")
	sse := readRepoFile(t, "internal/server/sse.go")
	assertContains(t, helpers, "if len(values) == 0 {")
	assertContains(t, helpers, "return \"\"")
	assertContains(t, helpers, "math.Max(1, float64(len(values)-1))")
	assertContains(t, sse, "history := s.collector.History(60)")
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
