// Tests for PerformanceConfig cooldown back-compat and defaults.
//
// @aitri-trace BG-023 BL-001 FR-004
package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPerfTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// TC-BG-023-006 — Factory defaults include 15-minute cooldowns.
//
// @aitri-tc TC-BG-023-006
func TestDefaultPerformanceConfig_IncludesCooldowns(t *testing.T) {
	cfg := DefaultPerformanceConfig()
	assert.Equal(t, 15, cfg.DockerCooldownMin)
	assert.Equal(t, 15, cfg.SystemdCooldownMin)
}

// TC-BG-023-007 — Reading a row persisted before BL-001 (no cooldown
// fields in the JSON) returns the documented default rather than zero,
// so SetTransitionCooldowns doesn't silently drop a fresh boot's setup.
//
// @aitri-tc TC-BG-023-007
func TestGetPerformanceConfig_LegacyRowFillsCooldownDefaults(t *testing.T) {
	db := newPerfTestDB(t)

	// Write a config row missing the cooldown fields (simulates a row
	// persisted before BL-001 was deployed).
	require.NoError(t, db.UpsertNotificationConfig(&NotificationConfig{
		Channel: "performance",
		Enabled: true,
		Config:  `{"SSEIntervalSec":5,"DiskIntervalMin":30,"DockerIntervalSec":10,"SystemdIntervalSec":30,"BackupIntervalHours":24}`,
	}))

	cfg, err := db.GetPerformanceConfig()
	require.NoError(t, err)
	assert.Equal(t, 15, cfg.DockerCooldownMin, "legacy row must fall back to 15-min Docker cooldown default")
	assert.Equal(t, 15, cfg.SystemdCooldownMin, "legacy row must fall back to 15-min Systemd cooldown default")
}

// TC-BG-023-008 — A row with explicit cooldowns round-trips through Save
// + Get unchanged.
//
// @aitri-tc TC-BG-023-008
func TestSavePerformanceConfig_CooldownsRoundTrip(t *testing.T) {
	db := newPerfTestDB(t)

	want := PerformanceConfig{
		SSEIntervalSec:      5,
		DiskIntervalMin:     30,
		DockerIntervalSec:   10,
		SystemdIntervalSec:  30,
		BackupIntervalHours: 24,
		DockerCooldownMin:   7,
		SystemdCooldownMin:  42,
	}
	require.NoError(t, db.SavePerformanceConfig(want))

	got, err := db.GetPerformanceConfig()
	require.NoError(t, err)
	assert.Equal(t, 7, got.DockerCooldownMin)
	assert.Equal(t, 42, got.SystemdCooldownMin)
}
