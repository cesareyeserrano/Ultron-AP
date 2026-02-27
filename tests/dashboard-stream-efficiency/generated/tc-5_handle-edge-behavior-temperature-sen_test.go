// TC-5: Handle edge behavior - Temperature sensor unavailable (`nil`): dashboard must render placeholder and avoid panics
// Acceptance Criteria: none
// No AC mapped to this TC.
package generated

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTc_5_handle_edge_behavior_temperature_sensor_unavailable_nil_dashboard_must_render_placeholder_and_avoid_panics(t *testing.T) {
	helpers := readRepoFile(t, "internal/server/helpers.go")
	sse := readRepoFile(t, "internal/server/sse.go")
	assertContains(t, helpers, "if temp == nil {")
	assertContains(t, helpers, "return \"--\"")
	assertContains(t, sse, "if snap.Temperature != nil {")
	assertContains(t, sse, "lastTemp = *snap.Temperature")
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
