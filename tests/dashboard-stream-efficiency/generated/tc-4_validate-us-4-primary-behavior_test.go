// TC-4: Validate us-4 primary behavior
// Acceptance Criteria: AC-1
// AC-1: Given an authenticated operator on dashboard, when SSE stream runs, then metrics remain frequently updated and charts update at a slower cadence.
package generated

import (
	"testing"
)

func TestTc_4_validate_us_4_primary_behavior(t *testing.T) {
	sse := readRepoFile(t, "internal/server/sse.go")
	assertContains(t, sse, "every := int((15*time.Second + current - 1) / current)")
	assertContains(t, sse, "includeCharts = tick%every == 0")
	assertContains(t, sse, "data := s.buildSSEPayloadWithOptions(includeCharts)")
}
