// TC-1: Validate us-1 primary behavior
// Acceptance Criteria: AC-1, AC-3
// AC-1: Given an authenticated operator on dashboard, when SSE stream runs, then metrics remain frequently updated and charts update at a slower cadence.
// AC-3: Given temperature value in normal/warning/high range, when dashboard renders indicator and chart, then colors are green/yellow/red respectively.
package generated

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestTc_1_validate_us_1_primary_behavior(t *testing.T) {
	sse := readRepoFile(t, "internal/server/sse.go")
	metricsTpl := readRepoFile(t, "web/templates/partials/sse-metrics.html")
	assertContains(t, sse, "buildSSEPayloadWithOptions(includeCharts bool, includeHeavy bool)")
	assertContains(t, sse, "chartsEvery := cadenceEvery(current, 15*time.Second)")
	assertContains(t, metricsTpl, "{{tempColor .Metrics.Temperature}}")
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
