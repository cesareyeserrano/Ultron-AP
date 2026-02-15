package notify

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDispatcher_StartStop(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	d.Start()
	time.Sleep(10 * time.Millisecond)
	d.Stop()
}

func TestDispatcher_Dispatch_NoChannels(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	d.Start()
	defer d.Stop()

	// Should not panic even with no notification channels configured
	d.Dispatch(&database.Alert{Severity: "info", Message: "test", Source: "test"})
	time.Sleep(50 * time.Millisecond)
}

func TestDispatcher_QueueFull(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	// Don't start — queue will fill up but not be consumed

	// Fill queue
	for i := 0; i < 100; i++ {
		d.Dispatch(&database.Alert{Severity: "info", Message: "test", Source: "test"})
	}

	// This should drop without blocking
	d.Dispatch(&database.Alert{Severity: "info", Message: "dropped", Source: "test"})

	assert.Len(t, d.queue, 100) // Queue still at capacity
}

func TestDispatcher_BuildNotifiers_NoConfig(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)

	notifiers := d.buildNotifiers()
	assert.Empty(t, notifiers)
}

func TestDispatcher_BuildNotifiers_TelegramEnabled(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "telegram",
		Enabled: true,
		Config:  `{"bot_token":"123:ABC","chat_id":"456"}`,
	})

	d := NewDispatcher(db)
	notifiers := d.buildNotifiers()
	assert.Len(t, notifiers, 1)
	assert.Equal(t, "telegram", notifiers[0].Name())
}

func TestDispatcher_BuildNotifiers_TelegramDisabled(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "telegram",
		Enabled: false,
		Config:  `{"bot_token":"123:ABC","chat_id":"456"}`,
	})

	d := NewDispatcher(db)
	notifiers := d.buildNotifiers()
	assert.Empty(t, notifiers)
}
