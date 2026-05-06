package alerts

import (
	"testing"
	"time"
)

// @aitri-trace BG-028 BL-003 FR-004
//
// pruneCooldowns must evict entries older than cooldownRetention so
// docker:* / systemd:* keys (whose values are transient container /
// service names) don't accumulate forever as instances come and go.
// Fresh entries within the retention window must be preserved so an
// active cooldown is not lost mid-window.
func TestPruneCooldowns_EvictsStaleEntriesAndKeepsFresh(t *testing.T) {
	eng := NewEngine(nil, nil, nil, nil, time.Minute)

	now := time.Now()
	eng.cooldowns["docker:fresh"] = now.Add(-1 * time.Hour)            // inside retention
	eng.cooldowns["systemd:fresh"] = now.Add(-30 * time.Minute)        // inside retention
	eng.cooldowns["docker:stale"] = now.Add(-cooldownRetention - time.Minute) // outside
	eng.cooldowns["systemd:ancient"] = now.Add(-365 * 24 * time.Hour)  // far outside

	eng.pruneCooldowns()

	if _, ok := eng.cooldowns["docker:fresh"]; !ok {
		t.Fatalf("entry inside retention window must be preserved")
	}
	if _, ok := eng.cooldowns["systemd:fresh"]; !ok {
		t.Fatalf("entry inside retention window must be preserved")
	}
	if _, ok := eng.cooldowns["docker:stale"]; ok {
		t.Fatalf("entry outside retention window must be evicted")
	}
	if _, ok := eng.cooldowns["systemd:ancient"]; ok {
		t.Fatalf("very old entry must be evicted")
	}
	if got := len(eng.cooldowns); got != 2 {
		t.Fatalf("expected 2 surviving entries, got %d", got)
	}
}

func TestPruneCooldowns_NoOpOnEmptyMap(t *testing.T) {
	eng := NewEngine(nil, nil, nil, nil, time.Minute)
	eng.pruneCooldowns() // must not panic
	if len(eng.cooldowns) != 0 {
		t.Fatalf("empty map must remain empty")
	}
}
