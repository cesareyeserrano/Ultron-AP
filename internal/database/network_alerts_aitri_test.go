package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/require"
)

func TestTC_NA_078h(t *testing.T) {
	// @aitri-tc TC-NA-078h
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	for i := 0; i < 11; i++ {
		require.NoError(t, db.CreateAlertConfig(&AlertConfig{Name: "Rule", Metric: "cpu", Operator: ">", Threshold: 80, Severity: "warning", Enabled: true, CooldownMinutes: 15}))
	}
	rows, err := db.Query("PRAGMA table_info(AlertConfig)")
	require.NoError(t, err)
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt, pk any
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk))
		cols[name] = true
	}
	require.True(t, cols["target"])
	require.True(t, cols["sustained_duration"])
	var count, defaults int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM AlertConfig").Scan(&count))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM AlertConfig WHERE sustained_duration=0 AND target IS NULL").Scan(&defaults))
	require.Equal(t, 11, count)
	require.Equal(t, 11, defaults)
}

func TestTC_NA_078e(t *testing.T) {
	// @aitri-tc TC-NA-078e
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := New(path)
	require.NoError(t, err)
	require.NoError(t, db.CreateAlertConfig(&AlertConfig{Name: "Rule", Metric: "cpu", Operator: ">", Threshold: 80, Severity: "warning", Enabled: true, CooldownMinutes: 15}))
	db.Close()
	db, err = New(path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM AlertConfig").Scan(&count))
	require.Equal(t, 1, count)
}

func TestTC_NA_078f(t *testing.T) {
	// @aitri-tc TC-NA-078f
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.CreateAlertConfig(&AlertConfig{Name: "CPU > 80", Metric: "cpu", Operator: ">", Threshold: 80, Severity: "critical", Enabled: true, CooldownMinutes: 15}))
	var old struct {
		ID              int64
		Name            string
		Metric          string
		Operator        string
		Threshold       float64
		Severity        string
		Enabled         int
		CooldownMinutes int
		CreatedAt       string
		UpdatedAt       string
	}
	err = db.QueryRow(`SELECT id, name, metric, operator, threshold, severity, enabled, cooldown_minutes, created_at, updated_at FROM AlertConfig WHERE id=1`).
		Scan(&old.ID, &old.Name, &old.Metric, &old.Operator, &old.Threshold, &old.Severity, &old.Enabled, &old.CooldownMinutes, &old.CreatedAt, &old.UpdatedAt)
	require.NoError(t, err)
	require.Equal(t, "cpu", old.Metric)
	require.Equal(t, 80.0, old.Threshold)
	require.Equal(t, 15, old.CooldownMinutes)
	require.NotEqual(t, sql.ErrNoRows, err)
}
