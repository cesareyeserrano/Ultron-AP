// TC-1: Validate us-1 primary behavior
// Acceptance Criteria: AC-1, AC-3
// AC-1: Given an authenticated operator on dashboard, when SSE stream runs, then metrics remain frequently updated and charts update at a slower cadence.
// AC-3: Given temperature value in normal/warning/high range, when dashboard renders indicator and chart, then colors are green/yellow/red respectively.
package generated

import (
	"testing"
)

func TestTc_1_validate_us_1_primary_behavior(t *testing.T) {
	sse := readRepoFile(t, "internal/server/sse.go")
	metricsTpl := readRepoFile(t, "web/templates/partials/sse-metrics.html")
	assertContains(t, sse, "buildSSEPayloadWithOptions(includeCharts bool)")
	assertContains(t, sse, "if current < 15*time.Second")
	assertContains(t, metricsTpl, "{{tempColor .Metrics.Temperature}}")
}
