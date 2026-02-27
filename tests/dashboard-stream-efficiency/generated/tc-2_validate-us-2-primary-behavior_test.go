// TC-2: Validate us-2 primary behavior
// Acceptance Criteria: AC-2
// AC-2: Given temperature samples in collector history, when charts render, then temperature history chart is visible.
package generated

import (
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
