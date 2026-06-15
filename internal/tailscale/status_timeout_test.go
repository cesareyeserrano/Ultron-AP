package tailscale

import (
	"context"
	"testing"
	"time"
)

// BG-041: a hung `tailscale` CLI must not block GetStatus indefinitely.
func TestGetStatus_TimesOutOnHungCommand(t *testing.T) {
	origCmd := runStatusCommand
	origTimeout := statusCommandTimeout
	t.Cleanup(func() {
		runStatusCommand = origCmd
		statusCommandTimeout = origTimeout
	})

	statusCommandTimeout = 50 * time.Millisecond
	// Simulate a CLI that never returns until its context is cancelled.
	runStatusCommand = func(ctx context.Context) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	start := time.Now()
	_, err := GetStatus()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when the tailscale command hangs")
	}
	if elapsed > time.Second {
		t.Fatalf("GetStatus did not honor the timeout: took %s (a missing timeout would hang forever)", elapsed)
	}
}

// Happy path: the timeout wiring must not break normal parsing.
func TestGetStatus_ParsesValidOutput(t *testing.T) {
	origCmd := runStatusCommand
	t.Cleanup(func() { runStatusCommand = origCmd })

	runStatusCommand = func(ctx context.Context) ([]byte, error) {
		return []byte(`{"Self":{"HostName":"pi","TailscaleIPs":["100.64.0.1"],"OS":"linux"},"Peer":{},"User":{}}`), nil
	}

	status, err := GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Self.Hostname != "pi" {
		t.Fatalf("got Self.Hostname %q, want %q", status.Self.Hostname, "pi")
	}
}
