// The DNS failure-rate rule used to log "insufficient_dns_samples" on every
// evaluation — a line every 5 seconds, forever, for a condition that is normal
// and persistent. NFR-009 requires state transitions to be logged once per
// change, not once per poll.
//
// @aitri-trace NFR-009 BG-081
package alerts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// @aitri-tc BG-081-001 — entering the sparse state reports once, and staying
// in it reports never again.
func TestMarkDNSSparse_LogsOnceOnEntry(t *testing.T) {
	e := &Engine{}

	assert.True(t, e.markDNSSparse(true), "the first transition into sparse must report")
	for i := 0; i < 100; i++ {
		assert.Falsef(t, e.markDNSSparse(true),
			"evaluation %d stayed in the same state and must not report again", i+2)
	}
}

// @aitri-tc BG-081-002 — leaving the sparse state re-arms it, so a later
// recurrence is reported again rather than swallowed forever.
func TestMarkDNSSparse_ReArmsAfterRecovery(t *testing.T) {
	e := &Engine{}

	require := assert.New(t)
	require.True(e.markDNSSparse(true), "first entry reports")
	require.False(e.markDNSSparse(false), "recovery itself is not a 'report the warning' event")
	require.True(e.markDNSSparse(true), "a NEW occurrence after recovery must report again")
}

// @aitri-tc BG-081-003 — a healthy engine that never goes sparse reports
// nothing, so the guard cannot invert into logging the good case.
func TestMarkDNSSparse_QuietWhenHealthy(t *testing.T) {
	e := &Engine{}
	for i := 0; i < 10; i++ {
		assert.False(t, e.markDNSSparse(false), "a healthy evaluation must never report")
	}
}
