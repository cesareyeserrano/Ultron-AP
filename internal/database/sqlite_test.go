package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_CreatesDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Verify DB is open
	err = db.Ping()
	assert.NoError(t, err)
}

func TestNew_CreatesDirectory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "subdir", "nested", "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	err = db.Ping()
	assert.NoError(t, err)
}

func TestNew_TablesExist(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	expectedTables := []string{"User", "Session", "Alert", "AlertConfig", "ActionLog"}

	for _, table := range expectedTables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		assert.NoError(t, err, "table %s should exist", table)
		assert.Equal(t, table, name)
	}
}

func TestNew_WALMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode)
}

// BG-017 regression: busy_timeout must be applied to every pooled connection,
// not just the first one created at sql.Open(). Force the pool to spawn
// multiple concurrent connections and check each reports the configured value.
func TestNew_BusyTimeoutOnAllConnections(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	const conns = 4
	db.SetMaxOpenConns(conns)

	// Hold `conns` transactions open simultaneously so each PRAGMA query
	// lands on a distinct pooled connection.
	type result struct {
		ms  int
		err error
	}
	ch := make(chan result, conns)
	gate := make(chan struct{})
	for i := 0; i < conns; i++ {
		go func() {
			tx, err := db.Begin()
			if err != nil {
				ch <- result{err: err}
				return
			}
			defer tx.Rollback()
			var ms int
			err = tx.QueryRow("PRAGMA busy_timeout").Scan(&ms)
			<-gate
			ch <- result{ms: ms, err: err}
		}()
	}

	close(gate)
	for i := 0; i < conns; i++ {
		r := <-ch
		require.NoError(t, r.err, "conn %d", i)
		assert.Equal(t, 5000, r.ms, "conn %d busy_timeout", i)
	}
}

func TestNew_IntegrityCheck(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	var result string
	err = db.QueryRow("PRAGMA integrity_check").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestNew_IdempotentSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Open and close once
	db1, err := New(dbPath)
	require.NoError(t, err)
	db1.Close()

	// Open again — should not fail (CREATE TABLE IF NOT EXISTS)
	db2, err := New(dbPath)
	require.NoError(t, err)
	defer db2.Close()

	err = db2.Ping()
	assert.NoError(t, err)
}

func TestDB_Close(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := New(dbPath)
	require.NoError(t, err)

	err = db.Close()
	assert.NoError(t, err)

	// After close, ping should fail
	err = db.Ping()
	assert.Error(t, err)
}
