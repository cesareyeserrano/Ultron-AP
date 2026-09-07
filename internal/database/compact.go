// Module:       internal/database (compact)
// Purpose:      Reclaim disk space after a prune. DELETE marks pages free
//
//	inside the file; it never shrinks the file itself.
//
// Dependencies: standard library only.
//
// @aitri-trace FR-099, US-099
package database

import "fmt"

// FreeSpaceBytes reports how many bytes the database holds as free pages —
// space already deleted but not returned to the filesystem.
//
// Returns the byte count, or an error if either pragma cannot be read.
//
// @aitri-trace FR-099, AC-099-002, TC-NSR-031e
func (db *DB) FreeSpaceBytes() (int64, error) {
	var freelist, pageSize int64
	if err := db.QueryRow("PRAGMA freelist_count").Scan(&freelist); err != nil {
		return 0, fmt.Errorf("read freelist_count: %w", err)
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("read page_size: %w", err)
	}
	return freelist * pageSize, nil
}

// Compact rewrites the database file, returning free pages to the filesystem.
//
// It blocks writers for the duration, which is why the caller gates it behind a
// size threshold rather than running it on every prune (see the retention job).
// Run it AFTER pruning, never before: on the production database that is the
// difference between rewriting ~175 MB and rewriting 719 MB.
//
// VACUUM cannot run inside a transaction, so this executes on the connection
// directly.
//
// The checkpoint afterwards is NOT optional. This database runs in WAL mode,
// where VACUUM writes the rebuilt content into the write-ahead log and leaves
// the main file at its old size until a checkpoint folds the WAL back in. Without
// TRUNCATE, `ls -la ultron.db` would still read 719 MB after a successful
// compaction and the whole point of FR-099 would be silently unmet — the
// rows gone, the disk not returned. TestTC_NSR_030h caught exactly that.
//
// @aitri-trace FR-099, AC-099-001, TC-NSR-030h
func (db *DB) Compact() error {
	if _, err := db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	// TRUNCATE (rather than PASSIVE or FULL) is what actually shrinks the WAL
	// file back to zero and lets the main file reach its compacted size on disk.
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("wal checkpoint after vacuum: %w", err)
	}
	return nil
}
