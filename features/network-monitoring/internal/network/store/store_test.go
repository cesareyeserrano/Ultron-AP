package store

import "testing"

// TestTC_NM_006e_BackupIncludesSamplesAndEvents asserts that net_samples and
// net_events are in the BackupTables contract — guaranteeing the FR-015
// backup runner picks them up. The TC's full integration shape (decrypt the
// backup, re-open as SQLite, query) sits on top of this contract; here we
// pin the contract itself, which is the underlying invariant.
//
// @aitri-tc TC-NM-006e
func TestTC_NM_006e_BackupIncludesSamplesAndEvents(t *testing.T) {
	t.Parallel()
	tables := BackupTables()
	required := map[string]bool{"net_samples": false, "net_events": false}
	for _, name := range tables {
		if _, ok := required[name]; ok {
			required[name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Errorf("BackupTables() does not include %q (FR-015 backup would drop it)", name)
		}
	}
	if len(tables) == 0 {
		t.Fatal("BackupTables() returned empty list — backup would skip every net_* table")
	}
}
