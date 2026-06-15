package alerts

import (
	"testing"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/stretchr/testify/assert"
)

// TestPruneProcessedNet is a regression test for BL-026: processedNet markers
// for events that have aged out of the recent window must be dropped so the map
// does not grow unbounded, while markers for events still in the window stay.
func TestPruneProcessedNet(t *testing.T) {
	eng := NewEngine(nil, nil, nil, nil, time.Minute)

	// Two rules processed events 1..3; event 1 has since aged out.
	for _, k := range []string{
		"7:1:wan_down", "7:2:wan_up", "7:3:wan_down", "9:1:public_ip_changed",
	} {
		eng.processedNet[k] = struct{}{}
	}
	// Malformed key must be left untouched (defensive).
	eng.processedNet["garbage"] = struct{}{}

	current := []database.NetEvent{{ID: 2}, {ID: 3}}
	eng.pruneProcessedNet(current)

	assert.NotContains(t, eng.processedNet, "7:1:wan_down", "aged-out event 1 marker should be pruned")
	assert.NotContains(t, eng.processedNet, "9:1:public_ip_changed", "aged-out event 1 marker should be pruned")
	assert.Contains(t, eng.processedNet, "7:2:wan_up", "in-window event 2 marker must remain")
	assert.Contains(t, eng.processedNet, "7:3:wan_down", "in-window event 3 marker must remain")
	assert.Contains(t, eng.processedNet, "garbage", "unparseable key left as-is")
}
