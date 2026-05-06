// Tests for the helper's log-finalization layer that pipes raw
// journalctl/dmesg/ps output through internal/logfilter before it
// crosses the IPC boundary back to the unprivileged web process.
//
// @aitri-trace BG-026 BL-011
package main

import (
	"strings"
	"testing"

	"github.com/cesareyeserrano/ultron-ap/internal/logfilter"
)

// PolicyJournal must redact env-style secrets that journalctl tends to
// leak when a service starts up with config in its environment block.
func TestFinalizeLog_JournalRedactsSecretsAndCaps(t *testing.T) {
	in := []byte("Apr 23 10:00 host app[1]: TOKEN=abc123 starting\n" +
		"Apr 23 10:00 host app[1]: Authorization: Bearer eyJtest.payload.sig\n")
	got, err := finalizeLog(in, nil, logfilter.PolicyJournal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "abc123") {
		t.Fatalf("env secret leaked: %q", got)
	}
	if strings.Contains(got, "eyJtest.payload.sig") {
		t.Fatalf("bearer token leaked: %q", got)
	}
}

// PolicyNone (used for ps and dmesg) must pass content through
// unchanged. A process called password-manager in `ps` output is not a
// leaked secret and must remain visible to the operator.
func TestFinalizeLog_NonePolicyDoesNotRedact(t *testing.T) {
	in := []byte("PID COMM\n  1 password-manager\n")
	got, err := finalizeLog(in, nil, logfilter.PolicyNone)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "password-manager") {
		t.Fatalf("PolicyNone must preserve content, got %q", got)
	}
}

// Underlying journalctl/dmesg/ps errors must propagate unchanged so the
// caller still sees the real failure mode (e.g. service not running).
func TestFinalizeLog_PropagatesUnderlyingError(t *testing.T) {
	want := "no such service"
	_, err := finalizeLog([]byte("ignored"), &errString{want}, logfilter.PolicyJournal)
	if err == nil || err.Error() != want {
		t.Fatalf("expected error %q to propagate, got %v", want, err)
	}
}

type errString struct{ s string }

func (e *errString) Error() string { return e.s }
