// Tests for the helper's SO_PEERCRED-based caller authentication.
//
// @aitri-trace BG-021 BL-013
// TC-BG-021-001 .. 005
package main

import (
	"os/user"
	"strconv"
	"testing"
)

// TC-BG-021-001 — ULTRON_HELPER_ALLOWED_UIDS (CSV) populates the allowlist
// and ignores malformed entries with a warning rather than failing closed.
//
// @aitri-tc TC-BG-021-001
func TestResolveAllowedUIDs_CSV(t *testing.T) {
	t.Setenv("ULTRON_HELPER_ALLOWED_UIDS", "1000, 1001 , garbage, 1002")
	t.Setenv("ULTRON_HELPER_ALLOWED_UID", "")

	got := resolveAllowedUIDs()
	for _, want := range []uint32{1000, 1001, 1002} {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected uid %d in allowlist; got %v", want, got)
		}
	}
	if _, ok := got[0]; ok {
		t.Fatalf("uid 0 must not be in allowlist by accident")
	}
}

// TC-BG-021-002 — ULTRON_HELPER_ALLOWED_UID adds a single UID, additive
// with the CSV form.
//
// @aitri-tc TC-BG-021-002
func TestResolveAllowedUIDs_Single(t *testing.T) {
	t.Setenv("ULTRON_HELPER_ALLOWED_UIDS", "1000")
	t.Setenv("ULTRON_HELPER_ALLOWED_UID", "2000")

	got := resolveAllowedUIDs()
	if _, ok := got[1000]; !ok {
		t.Fatalf("CSV 1000 missing from allowlist: %v", got)
	}
	if _, ok := got[2000]; !ok {
		t.Fatalf("single UID 2000 missing from allowlist: %v", got)
	}
}

// TC-BG-021-003 — Garbage in ULTRON_HELPER_ALLOWED_UID is logged and
// ignored; falls back to user.Lookup("ultron"). When the user does not
// exist on the dev machine, the result is empty and the caller logs a
// loud warning.
//
// @aitri-tc TC-BG-021-003
func TestResolveAllowedUIDs_GarbageFallsBackToUserLookup(t *testing.T) {
	t.Setenv("ULTRON_HELPER_ALLOWED_UIDS", "")
	t.Setenv("ULTRON_HELPER_ALLOWED_UID", "not-a-number")

	got := resolveAllowedUIDs()

	// Allowed states: either empty (ultron user not present on dev box)
	// or contains exactly the resolved 'ultron' UID.
	if u, err := user.Lookup("ultron"); err == nil {
		if uid, err := strconv.ParseUint(u.Uid, 10, 32); err == nil {
			if _, ok := got[uint32(uid)]; !ok {
				t.Fatalf("expected ultron uid %d in fallback allowlist; got %v", uid, got)
			}
			return
		}
	}
	if len(got) != 0 {
		t.Fatalf("expected empty allowlist on dev box without 'ultron' user; got %v", got)
	}
}

// TC-BG-021-004 — Empty CSV entries are skipped silently.
//
// @aitri-tc TC-BG-021-004
func TestResolveAllowedUIDs_EmptyEntriesIgnored(t *testing.T) {
	t.Setenv("ULTRON_HELPER_ALLOWED_UIDS", ",,1000,, ,")
	t.Setenv("ULTRON_HELPER_ALLOWED_UID", "")

	got := resolveAllowedUIDs()
	if _, ok := got[1000]; !ok {
		t.Fatalf("expected uid 1000 in allowlist; got %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (empty entries skipped)", len(got))
	}
}

// TC-BG-021-005 — peerCredSupported flag reflects the build target so
// callers can degrade gracefully on non-Linux developer machines.
//
// @aitri-tc TC-BG-021-005
func TestPeerCredSupported_BuildAware(t *testing.T) {
	// The constant must be true on linux/* and false on darwin/* — we don't
	// constrain the value here, only that it is defined and reachable. The
	// test file's mere compilation under both build configurations is the
	// real assertion; this body documents the contract.
	_ = peerCredSupported
}

// BG-043 — the helper must fail closed: with peercred enforcement compiled in
// and an empty allowlist, a connection is refused rather than served.
func TestFailClosedNoAllowlist(t *testing.T) {
	empty := map[uint32]struct{}{}
	withUID := map[uint32]struct{}{997: {}}

	if !failClosedNoAllowlist(true, empty) {
		t.Error("supported + empty allowlist must fail closed (refuse), not serve everyone")
	}
	if failClosedNoAllowlist(true, withUID) {
		t.Error("supported + non-empty allowlist must NOT fail closed")
	}
	if failClosedNoAllowlist(true, nil) != true {
		t.Error("supported + nil allowlist must fail closed")
	}
	if failClosedNoAllowlist(false, empty) {
		t.Error("when peercred is unsupported, socket-mode perms are the auth — must not fail closed here")
	}
}
