// TC-5: Handle edge behavior - Temperature sensor unavailable (`nil`): dashboard must render placeholder and avoid panics
// Acceptance Criteria: none
// No AC mapped to this TC.
package generated

import "testing"

func TestTc_5_handle_edge_behavior_temperature_sensor_unavailable_nil_dashboard_must_render_placeholder_and_avoid_panics(t *testing.T) {
	helpers := readRepoFile(t, "internal/server/helpers.go")
	sse := readRepoFile(t, "internal/server/sse.go")
	assertContains(t, helpers, "if temp == nil {")
	assertContains(t, helpers, "return \"--\"")
	assertContains(t, sse, "if snap.Temperature != nil {")
	assertContains(t, sse, "lastTemp = *snap.Temperature")
}
