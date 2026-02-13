package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAlertTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAlertConfig(t *testing.T) {
	db := setupAlertTestDB(t)

	ac := &AlertConfig{
		Name:            "High CPU",
		Metric:          "cpu",
		Operator:        ">",
		Threshold:       90,
		Severity:        "critical",
		Enabled:         true,
		CooldownMinutes: 15,
	}
	err := db.CreateAlertConfig(ac)
	require.NoError(t, err)
	assert.Equal(t, int64(1), ac.ID)
}

func TestListAlertConfigs(t *testing.T) {
	db := setupAlertTestDB(t)

	db.CreateAlertConfig(&AlertConfig{Name: "A", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15})
	db.CreateAlertConfig(&AlertConfig{Name: "B", Metric: "ram", Operator: ">", Threshold: 85, Severity: "warning", Enabled: false, CooldownMinutes: 10})

	configs, err := db.ListAlertConfigs()
	require.NoError(t, err)
	assert.Len(t, configs, 2)
	assert.Equal(t, "A", configs[0].Name)
	assert.True(t, configs[0].Enabled)
	assert.False(t, configs[1].Enabled)
}

func TestListEnabledAlertConfigs(t *testing.T) {
	db := setupAlertTestDB(t)

	db.CreateAlertConfig(&AlertConfig{Name: "A", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15})
	db.CreateAlertConfig(&AlertConfig{Name: "B", Metric: "ram", Operator: ">", Threshold: 85, Severity: "warning", Enabled: false, CooldownMinutes: 10})

	configs, err := db.ListEnabledAlertConfigs()
	require.NoError(t, err)
	assert.Len(t, configs, 1)
	assert.Equal(t, "A", configs[0].Name)
}

func TestCreateAlert(t *testing.T) {
	db := setupAlertTestDB(t)

	configID := int64(1)
	value := 95.5
	a := &Alert{
		ConfigID: &configID,
		Severity: "critical",
		Message:  "CPU at 95.5%",
		Source:   "cpu",
		Value:    &value,
	}
	err := db.CreateAlert(a)
	require.NoError(t, err)
	assert.Equal(t, int64(1), a.ID)
}

func TestListAlerts(t *testing.T) {
	db := setupAlertTestDB(t)

	value1 := 91.0
	value2 := 86.0
	db.CreateAlert(&Alert{Severity: "critical", Message: "CPU high", Source: "cpu", Value: &value1})
	db.CreateAlert(&Alert{Severity: "warning", Message: "RAM high", Source: "ram", Value: &value2})

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 2)
	// Verify both messages exist
	messages := []string{alerts[0].Message, alerts[1].Message}
	assert.Contains(t, messages, "CPU high")
	assert.Contains(t, messages, "RAM high")
}

func TestListAlerts_Limit(t *testing.T) {
	db := setupAlertTestDB(t)

	for i := 0; i < 5; i++ {
		db.CreateAlert(&Alert{Severity: "info", Message: "test", Source: "cpu"})
	}

	alerts, err := db.ListAlerts(3)
	require.NoError(t, err)
	assert.Len(t, alerts, 3)
}

func TestAlertConfigCount(t *testing.T) {
	db := setupAlertTestDB(t)

	count, err := db.AlertConfigCount()
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	db.CreateAlertConfig(&AlertConfig{Name: "A", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15})
	count, err = db.AlertConfigCount()
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestGetAlertConfig(t *testing.T) {
	db := setupAlertTestDB(t)

	ac := &AlertConfig{Name: "Test", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	db.CreateAlertConfig(ac)

	got, err := db.GetAlertConfig(ac.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Test", got.Name)
	assert.Equal(t, 90.0, got.Threshold)
}

func TestGetAlertConfig_NotFound(t *testing.T) {
	db := setupAlertTestDB(t)

	got, err := db.GetAlertConfig(999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestSeedDefaultAlertConfigs(t *testing.T) {
	db := setupAlertTestDB(t)

	err := db.SeedDefaultAlertConfigs()
	require.NoError(t, err)

	configs, err := db.ListAlertConfigs()
	require.NoError(t, err)
	assert.Len(t, configs, 4)
	assert.Equal(t, "High CPU", configs[0].Name)
	assert.Equal(t, "High Memory", configs[1].Name)
	assert.Equal(t, "Disk Full", configs[2].Name)
	assert.Equal(t, "High Temperature", configs[3].Name)
}

func TestSeedDefaultAlertConfigs_Idempotent(t *testing.T) {
	db := setupAlertTestDB(t)

	db.SeedDefaultAlertConfigs()
	db.SeedDefaultAlertConfigs() // Should not duplicate

	configs, err := db.ListAlertConfigs()
	require.NoError(t, err)
	assert.Len(t, configs, 4)
}

func TestAlert_NilConfigID(t *testing.T) {
	db := setupAlertTestDB(t)

	a := &Alert{
		Severity: "warning",
		Message:  "Docker container stopped",
		Source:   "docker:nginx",
	}
	err := db.CreateAlert(a)
	require.NoError(t, err)

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 1)
	assert.Nil(t, alerts[0].ConfigID)
	assert.Nil(t, alerts[0].Value)
}
