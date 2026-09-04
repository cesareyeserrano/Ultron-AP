// Acceptance-criterion coverage for the bundled rule decoder.
//
// @aitri-trace FR-040 TC-IE-013f
package rules

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-IE-013f / AC-040-003
// Two rules sharing an id: only the FIRST occurrence loads, and the second is
// logged as duplicate-rule-id. TC-IE-002e covers this alongside two unrelated
// rejections; this isolates it so a change to either one cannot mask the other.
//
// @aitri-tc TC-IE-013f
func TestTC_IE_013f_DuplicateRuleIDKeepsFirstAndLogsSecond(t *testing.T) {
	// @aitri-tc TC-IE-013f
	raw := []byte(`[
		{
			"id": "same_id",
			"title": "First",
			"condition": {"op":"gt","left":{"var":"cpu_pct"},"right":{"const":90}},
			"severity": "critical",
			"verdict": "first verdict",
			"recommendation": "first recommendation"
		},
		{
			"id": "same_id",
			"title": "Second",
			"condition": {"op":"gt","left":{"var":"ram_pct"},"right":{"const":80}},
			"severity": "warn",
			"verdict": "second verdict",
			"recommendation": "second recommendation"
		}
	]`)

	logf, lines, mu := captureLogger()
	rs, err := LoadFromBytes(raw, logf)

	// A duplicate is a rejected rule, not a broken document — the load itself
	// must still succeed so one bad entry cannot take the whole set down.
	require.NoError(t, err)
	require.Len(t, rs, 1, "exactly one rule survives a duplicate id")

	assert.Equal(t, "same_id", rs[0].ID)
	assert.Equal(t, "First", rs[0].Title, "the FIRST occurrence is the one kept")
	assert.Equal(t, "critical", string(rs[0].Severity), "the survivor keeps its own severity, not the duplicate's")

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(*lines, "\n")
	assert.Contains(t, joined, "duplicate-rule-id", "the rejection reason is logged")
	assert.Contains(t, joined, "same_id", "the log names the offending rule id")
}
