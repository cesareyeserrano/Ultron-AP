// TC-2: Validate us-2 primary behavior
// Acceptance Criteria: AC-2
// AC-2: Given temperature samples in collector history, when charts render, then temperature history chart is visible.
package generated

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestTc_2_validate_us_2_primary_behavior(t *testing.T) {
	chartsTpl := readRepoFile(t, "web/templates/partials/sse-charts.html")
	sse := readRepoFile(t, "internal/server/sse.go")
	assertContains(t, chartsTpl, "Temp History")
	assertContains(t, chartsTpl, ".TempValues")
	assertContains(t, chartsTpl, "sparklineSVGColor .TempValues")
	assertContains(t, sse, "dd.TempValues = make([]float64, len(history))")
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
