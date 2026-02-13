package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertNotificationConfig_Create(t *testing.T) {
	db := setupAlertTestDB(t)

	nc := &NotificationConfig{
		Channel: "telegram",
		Enabled: true,
		Config:  `{"bot_token":"abc123","chat_id":"456"}`,
	}
	err := db.UpsertNotificationConfig(nc)
	require.NoError(t, err)

	got, err := db.GetNotificationConfig("telegram")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Enabled)
	assert.Contains(t, got.Config, "abc123")
}

func TestUpsertNotificationConfig_Update(t *testing.T) {
	db := setupAlertTestDB(t)

	db.UpsertNotificationConfig(&NotificationConfig{Channel: "telegram", Enabled: false, Config: `{"bot_token":"old"}`})
	db.UpsertNotificationConfig(&NotificationConfig{Channel: "telegram", Enabled: true, Config: `{"bot_token":"new"}`})

	got, err := db.GetNotificationConfig("telegram")
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Contains(t, got.Config, "new")
}

func TestGetNotificationConfig_NotFound(t *testing.T) {
	db := setupAlertTestDB(t)

	got, err := db.GetNotificationConfig("telegram")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestListNotificationConfigs(t *testing.T) {
	db := setupAlertTestDB(t)

	db.UpsertNotificationConfig(&NotificationConfig{Channel: "telegram", Config: "{}"})
	db.UpsertNotificationConfig(&NotificationConfig{Channel: "email", Config: "{}"})

	configs, err := db.ListNotificationConfigs()
	require.NoError(t, err)
	assert.Len(t, configs, 2)
}

func TestUpdateAlertConfig(t *testing.T) {
	db := setupAlertTestDB(t)

	ac := &AlertConfig{Name: "Old", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	ac.Name = "Updated"
	ac.Threshold = 95
	require.NoError(t, db.UpdateAlertConfig(ac))

	got, err := db.GetAlertConfig(ac.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, 95.0, got.Threshold)
}

func TestToggleAlertConfig(t *testing.T) {
	db := setupAlertTestDB(t)

	ac := &AlertConfig{Name: "Test", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	require.NoError(t, db.ToggleAlertConfig(ac.ID))

	got, err := db.GetAlertConfig(ac.ID)
	require.NoError(t, err)
	assert.False(t, got.Enabled)

	require.NoError(t, db.ToggleAlertConfig(ac.ID))

	got, err = db.GetAlertConfig(ac.ID)
	require.NoError(t, err)
	assert.True(t, got.Enabled)
}

func TestDeleteAlertConfig(t *testing.T) {
	db := setupAlertTestDB(t)

	ac := &AlertConfig{Name: "Delete Me", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	require.NoError(t, db.DeleteAlertConfig(ac.ID))

	got, err := db.GetAlertConfig(ac.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}
