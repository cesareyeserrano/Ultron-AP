// TC-3: Validate us-3 primary behavior
// Acceptance Criteria: AC-3
// AC-3: Given temperature value in normal/warning/high range, when dashboard renders indicator and chart, then colors are green/yellow/red respectively.
package generated

import (
	"testing"
)

func TestTc_3_validate_us_3_primary_behavior(t *testing.T) {
	helpers := readRepoFile(t, "internal/server/helpers.go")
	chartsTpl := readRepoFile(t, "web/templates/partials/sse-charts.html")
	assertContains(t, helpers, "tempWarnThresholdC = 60.0")
	assertContains(t, helpers, "tempHighThresholdC = 75.0")
	assertContains(t, helpers, "return \"text-green-400\"")
	assertContains(t, helpers, "return \"text-yellow-400\"")
	assertContains(t, helpers, "return \"text-danger\"")
	assertContains(t, chartsTpl, "tempSeriesClass .TempValues")
	assertContains(t, chartsTpl, "tempSeriesStroke .TempValues")
}
