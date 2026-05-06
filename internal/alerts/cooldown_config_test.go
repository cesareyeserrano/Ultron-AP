// Tests for the configurable Docker/Systemd transition cooldowns.
//
// @aitri-trace BG-023 BL-001 FR-004
// TC-BG-023-001 .. 005
package alerts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TC-BG-023-001 — A fresh engine with no setter call uses the historical
// 15-minute default. Backward compatible: pre-BL-001 deployments without
// the new PerformanceConfig fields see the same behaviour they always had.
//
// @aitri-tc TC-BG-023-001
func TestTransitionCooldown_DefaultIsFifteenMinutes(t *testing.T) {
	eng := &Engine{}
	assert.Equal(t, 15*time.Minute, eng.dockerCooldown())
	assert.Equal(t, 15*time.Minute, eng.systemdCooldown())
}

// TC-BG-023-002 — SetTransitionCooldowns updates both windows. Reads are
// goroutine-safe via atomic.Int64.
//
// @aitri-tc TC-BG-023-002
func TestSetTransitionCooldowns_UpdatesBoth(t *testing.T) {
	eng := &Engine{}
	eng.SetTransitionCooldowns(5*time.Minute, 30*time.Minute)
	assert.Equal(t, 5*time.Minute, eng.dockerCooldown())
	assert.Equal(t, 30*time.Minute, eng.systemdCooldown())
}

// TC-BG-023-003 — Sub-minute durations are rejected (silently ignored).
// Prevents a corrupt settings row from disabling cooldowns and flooding
// notifications.
//
// @aitri-tc TC-BG-023-003
func TestSetTransitionCooldowns_RejectsTooSmall(t *testing.T) {
	eng := &Engine{}
	eng.SetTransitionCooldowns(10*time.Minute, 10*time.Minute)
	// Now try to set sub-minute — must be ignored, original values preserved.
	eng.SetTransitionCooldowns(30*time.Second, 0)
	assert.Equal(t, 10*time.Minute, eng.dockerCooldown(), "sub-minute Docker cooldown must be ignored")
	assert.Equal(t, 10*time.Minute, eng.systemdCooldown(), "zero Systemd cooldown must be ignored")
}

// TC-BG-023-004 — Setting only one side leaves the other unchanged.
// Important for the runtime settings page that re-applies after partial
// edits.
//
// @aitri-tc TC-BG-023-004
func TestSetTransitionCooldowns_PartialUpdate(t *testing.T) {
	eng := &Engine{}
	eng.SetTransitionCooldowns(10*time.Minute, 20*time.Minute)
	// Only update Docker; pass 0 for Systemd to leave it alone.
	eng.SetTransitionCooldowns(7*time.Minute, 0)
	assert.Equal(t, 7*time.Minute, eng.dockerCooldown())
	assert.Equal(t, 20*time.Minute, eng.systemdCooldown(), "Systemd cooldown must survive a Docker-only update")
}

// TC-BG-023-005 — A newly-constructed engine via NewEngine also reports
// the default cooldown — the literal 15 min is no longer hidden in the
// hot path.
//
// @aitri-tc TC-BG-023-005
func TestNewEngine_TransitionCooldownDefault(t *testing.T) {
	db := setupTestDB(t)
	eng := NewEngine(db, nil, nil, nil, time.Minute)
	assert.Equal(t, 15*time.Minute, eng.dockerCooldown())
	assert.Equal(t, 15*time.Minute, eng.systemdCooldown())
}
