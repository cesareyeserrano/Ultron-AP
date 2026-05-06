// Tests for the atomic-audit helper introduced for BL-010 / BG-024.
// Proves that WithAuditTx commits both the database mutation and the
// ActionLog row together — and rolls both back on any failure — so a
// privileged action can never land without its audit record.
//
// @aitri-trace BG-024 BL-010 FR-006
// TC-BG-024-001 .. 005
package database

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAtomicTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// TC-BG-024-001 — happy path: fn succeeds → both the mutation and the
// audit row are committed.
//
// @aitri-tc TC-BG-024-001
func TestWithAuditTx_HappyPath_BothCommit(t *testing.T) {
	db := newAtomicTestDB(t)

	// Seed an alert so DeleteAlertsTx has something to remove.
	require.NoError(t, db.CreateAlert(&Alert{Severity: "critical", Message: "x", Source: "t"}))

	err := db.WithAuditTx(func(tx *sql.Tx, e *ActionLogEntry) error {
		_, dErr := db.DeleteAlertsTx(tx, "critical")
		if dErr != nil {
			return dErr
		}
		e.Source = "alerts"
		e.Action = "clear"
		e.Target = "critical"
		e.Result = "success"
		e.Details = "deleted=1"
		return nil
	})
	require.NoError(t, err)

	// Alert deleted.
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Empty(t, alerts, "alert must be deleted on commit")

	// Audit row written.
	logs, err := db.ListActionLogs(10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "alerts", logs[0].Source)
	assert.Equal(t, "clear", logs[0].Action)
	assert.Equal(t, "deleted=1", logs[0].Details)
}

// TC-BG-024-002 — fn returns an error → BOTH the mutation and audit roll
// back. The alert that fn deleted in the tx is still present after.
//
// @aitri-tc TC-BG-024-002
func TestWithAuditTx_FnError_RollsBackBoth(t *testing.T) {
	db := newAtomicTestDB(t)

	require.NoError(t, db.CreateAlert(&Alert{Severity: "critical", Message: "x", Source: "t"}))

	wantErr := errors.New("simulated mid-action failure")
	err := db.WithAuditTx(func(tx *sql.Tx, e *ActionLogEntry) error {
		// Perform the delete inside the tx — then fail.
		if _, err := db.DeleteAlertsTx(tx, "critical"); err != nil {
			return err
		}
		// Note: we do NOT populate `e`; rollback should fire either way.
		return wantErr
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)

	// Alert NOT deleted (rollback restored the row).
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 1, "alert must survive rollback")

	// No audit log row was written.
	logs, err := db.ListActionLogs(10)
	require.NoError(t, err)
	assert.Empty(t, logs, "audit log must NOT be written when fn fails")
}

// TC-BG-024-003 — empty audit entry (fn forgot to populate it) still
// produces a valid INSERT with empty strings rather than an error;
// caller bug surfaces in the journal as an empty-Source row.
//
// @aitri-tc TC-BG-024-003
func TestWithAuditTx_EmptyEntry_StillCommits(t *testing.T) {
	db := newAtomicTestDB(t)
	err := db.WithAuditTx(func(tx *sql.Tx, e *ActionLogEntry) error {
		_, _ = db.DeleteAlertsTx(tx, "")
		// fn intentionally does NOT populate *e
		return nil
	})
	require.NoError(t, err)

	logs, err := db.ListActionLogs(10)
	require.NoError(t, err)
	require.Len(t, logs, 1, "an audit row should still be created")
	assert.Equal(t, "", logs[0].Source)
}

// TC-BG-024-004 — LogActionTx writes through an external transaction so
// callers that already manage their own tx can record an audit entry
// without spawning a nested transaction.
//
// @aitri-tc TC-BG-024-004
func TestLogActionTx_WritesInsideExternalTransaction(t *testing.T) {
	db := newAtomicTestDB(t)

	tx, err := db.Begin()
	require.NoError(t, err)

	require.NoError(t, db.LogActionTx(tx, ActionLogEntry{
		Source:  "settings",
		Action:  "save",
		Target:  "performance",
		Result:  "success",
		Details: "test",
	}))
	require.NoError(t, tx.Commit())

	logs, err := db.ListActionLogs(10)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "settings", logs[0].Source)
	assert.Equal(t, "save", logs[0].Action)
}

// TC-BG-024-005 — LogActionTx is rolled back when the surrounding tx
// fails to commit (here we explicitly Rollback). No audit row lands.
//
// @aitri-tc TC-BG-024-005
func TestLogActionTx_RolledBackWithSurroundingTransaction(t *testing.T) {
	db := newAtomicTestDB(t)

	tx, err := db.Begin()
	require.NoError(t, err)

	require.NoError(t, db.LogActionTx(tx, ActionLogEntry{
		Source: "settings",
		Action: "save",
	}))
	require.NoError(t, tx.Rollback())

	logs, err := db.ListActionLogs(10)
	require.NoError(t, err)
	assert.Empty(t, logs, "audit row must roll back with the parent tx")
}
