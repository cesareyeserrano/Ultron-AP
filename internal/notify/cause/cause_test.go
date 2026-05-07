package cause

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stubProc returns the static list it was constructed with, optionally after
// a delay to simulate slow /proc scans (exercising NFR-005's 200ms budget).
type stubProc struct {
	samples []ProcSample
	delay   time.Duration
	calls   atomic.Int64
}

func (s *stubProc) TopProcesses(ctx context.Context, axis Axis, n int) ([]ProcSample, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	// Order respecting axis is the production reader's job; the renderer only
	// looks at the first element, so this stub returns a single sample.
	if n <= 0 || len(s.samples) == 0 {
		return nil, nil
	}
	out := make([]ProcSample, 0, n)
	for i := 0; i < n && i < len(s.samples); i++ {
		out = append(out, s.samples[i])
	}
	return out, nil
}

// fakeExec lets a test inject a canned subprocess response.
func fakeExec(stdout string, err error) execFn {
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return []byte(stdout), err
	}
}

// fakeExecRecorder captures the call so injection tests can assert no exec
// happened on the bad-name path.
type fakeExecRecorder struct {
	calls atomic.Int64
}

func (f *fakeExecRecorder) Exec() execFn {
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		f.calls.Add(1)
		return nil, nil
	}
}

// TestTC_TMU_029h covers FR-029 / AC-029-001: a CPU fire with ffmpeg as the
// top process at 78% renders "top: ffmpeg (78%)".
//
// @aitri-trace FR-029 US-029 AC-029-001 TC-TMU-029h
func TestTC_TMU_029h_CPUTopProcess(t *testing.T) {
	d := New(&stubProc{samples: []ProcSample{{Comm: "ffmpeg", CPUPct: 78.0}}})
	c, err := d.Resource(context.Background(), "cpu", ResourceData{})
	if err != nil {
		t.Fatalf("Resource err: %v", err)
	}
	if c == nil {
		t.Fatalf("Cause is nil; want non-nil")
	}
	if c.Line != "top: ffmpeg (78%)" {
		t.Fatalf("Line = %q; want %q", c.Line, "top: ffmpeg (78%)")
	}
	if c.Source != SourceProc {
		t.Fatalf("Source = %q; want %q", c.Source, SourceProc)
	}
}

// TestTC_TMU_029m covers FR-029 / AC-029-002: RAM fire renders MB.
//
// @aitri-trace FR-029 US-029 AC-029-002 TC-TMU-029m
func TestTC_TMU_029m_RAMTopProcess(t *testing.T) {
	d := New(&stubProc{samples: []ProcSample{{Comm: "chromium", RSSMB: 1200}}})
	c, err := d.Resource(context.Background(), "ram", ResourceData{})
	if err != nil {
		t.Fatalf("Resource err: %v", err)
	}
	if c == nil || c.Line != "top: chromium (1200 MB)" {
		t.Fatalf("Line = %q; want %q", lineOf(c), "top: chromium (1200 MB)")
	}
}

// TestTC_TMU_029d covers FR-029 / AC-029-003: disk fire formats free space.
//
// @aitri-trace FR-029 US-029 AC-029-003 TC-TMU-029d
func TestTC_TMU_029d_DiskFreeSpace(t *testing.T) {
	d := New(nil) // disk path doesn't need ProcReader
	gb := int64(1)<<30 + int64(1)<<28 // ~1.25 GB
	c, err := d.Resource(context.Background(), "disk", ResourceData{
		DiskMount: "/", DiskUsedPct: 95, DiskFreeBytes: gb,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "/ at 95% — 1.2 GB free"
	if c == nil || c.Line != want {
		t.Fatalf("Line = %q; want %q", lineOf(c), want)
	}
}

// TestTC_TMU_029t covers FR-029 / AC-029-004: temperature renders 'climbing'
// when Δ ≥ +2°C.
//
// @aitri-trace FR-029 US-029 AC-029-004 TC-TMU-029t
func TestTC_TMU_029t_TemperatureClimbing(t *testing.T) {
	d := New(nil)
	c, err := d.Resource(context.Background(), "temp", ResourceData{
		TempCurrentC: 78, TempTrendDeltaC: 6,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c == nil || !strings.Contains(c.Line, "climbing") {
		t.Fatalf("Line = %q; want contains 'climbing'", lineOf(c))
	}
	if strings.Contains(c.Line, "stable") || strings.Contains(c.Line, "cooling") {
		t.Fatalf("Line = %q; must not contain 'stable' or 'cooling'", c.Line)
	}
}

// TestTC_TMU_029_TempStableAndCooling spot-checks the other two trend bands.
//
// @aitri-trace FR-029 AC-029-004 TC-TMU-029-temp-bands
func TestTC_TMU_029_TempStableAndCooling(t *testing.T) {
	d := New(nil)
	c, _ := d.Resource(context.Background(), "temp", ResourceData{TempCurrentC: 70, TempTrendDeltaC: 0.5})
	if c == nil || !strings.Contains(c.Line, "stable") {
		t.Fatalf("Δ=+0.5: Line=%q want 'stable'", lineOf(c))
	}
	c, _ = d.Resource(context.Background(), "temp", ResourceData{TempCurrentC: 65, TempTrendDeltaC: -3})
	if c == nil || !strings.Contains(c.Line, "cooling") {
		t.Fatalf("Δ=-3: Line=%q want 'cooling'", lineOf(c))
	}
}

// TestTC_TMU_029s covers FR-029 / AC-029-005: a systemd fire whose journal
// contains "connect() failed" picks that line and prefixes with "last error:".
//
// @aitri-trace FR-029 US-029 AC-029-005 TC-TMU-029s
func TestTC_TMU_029s_SystemdLastErrorLine(t *testing.T) {
	d := New(nil)
	d.Exec = fakeExec("started\nconnect() failed\nshutting down\n", nil)
	c, err := d.Systemd(context.Background(), "nginx.service")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "last error: connect() failed"
	if c == nil || c.Line != want {
		t.Fatalf("Line = %q; want %q", lineOf(c), want)
	}
	if c.Source != SourceJournal {
		t.Fatalf("Source = %q; want %q", c.Source, SourceJournal)
	}
}

// TestTC_TMU_029x covers FR-029 / AC-029-006: docker exit code 137 maps to
// the OOM-killed cause string.
//
// @aitri-trace FR-029 US-029 AC-029-006 TC-TMU-029x
func TestTC_TMU_029x_DockerExitCode137(t *testing.T) {
	d := New(nil)
	c, err := d.Docker(context.Background(), "mealie", 137)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "cause: OOM-killed (137)"
	if c == nil || c.Line != want {
		t.Fatalf("Line = %q; want %q", lineOf(c), want)
	}
	if c.Source != SourceExitCode {
		t.Fatalf("Source = %q; want %q", c.Source, SourceExitCode)
	}
}

// TestTC_TMU_029f covers FR-029 / AC-029-008: the cause is omitted when the
// derivation exceeds its budget. We simulate this with a stubProc whose
// delay (300ms) exceeds the ctx deadline (50ms).
//
// @aitri-trace FR-029 US-029 AC-029-008 NFR-005 TC-TMU-029f
func TestTC_TMU_029f_TimeoutOmitsLine(t *testing.T) {
	d := New(&stubProc{samples: []ProcSample{{Comm: "x", CPUPct: 50}}, delay: 300 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	t0 := time.Now()
	c, _ := d.Resource(ctx, "cpu", ResourceData{})
	elapsed := time.Since(t0)

	if c != nil {
		t.Fatalf("Cause = %+v; want nil on timeout", c)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("derivation didn't return promptly on timeout: %v", elapsed)
	}
}

// TestTC_TMU_029n covers FR-029 / AC-029-009: an empty proc-snapshot result
// causes the cause line to be omitted — never rendered as "cause: unknown".
//
// @aitri-trace FR-029 US-029 AC-029-009 TC-TMU-029n
func TestTC_TMU_029n_NoDataOmits(t *testing.T) {
	d := New(&stubProc{samples: nil})
	c, err := d.Resource(context.Background(), "cpu", ResourceData{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c != nil {
		t.Fatalf("Cause = %+v; want nil for empty snapshot", c)
	}
}

// TestTC_TMU_NFR008b covers NFR-008: a unit name with a shell-injection
// payload is rejected before any exec call is made.
//
// @aitri-trace FR-029 NFR-008 TC-TMU-NFR008b
func TestTC_TMU_NFR008b_MaliciousUnitNameRejected(t *testing.T) {
	d := New(nil)
	rec := &fakeExecRecorder{}
	d.Exec = rec.Exec()

	_, err := d.Systemd(context.Background(), "nginx; rm -rf /")
	if !errors.Is(err, errInvalidName) {
		t.Fatalf("err = %v; want errInvalidName", err)
	}
	if !IsInvalidName(err) {
		t.Fatalf("IsInvalidName(err) = false; want true")
	}
	if got := rec.calls.Load(); got != 0 {
		t.Fatalf("exec was called %d times; want 0 (must be rejected before exec)", got)
	}
}

// TestTC_TMU_NFR008c covers NFR-008 for docker container names.
//
// @aitri-trace FR-029 NFR-008 TC-TMU-NFR008c
func TestTC_TMU_NFR008c_MaliciousContainerNameRejected(t *testing.T) {
	d := New(nil)
	rec := &fakeExecRecorder{}
	d.Exec = rec.Exec()
	_, err := d.Docker(context.Background(), "$(reboot)", 0)
	if !errors.Is(err, errInvalidName) {
		t.Fatalf("err = %v; want errInvalidName", err)
	}
	if got := rec.calls.Load(); got != 0 {
		t.Fatalf("exec was called %d times; want 0", got)
	}
}

// TestTC_TMU_029_JournalErrorReturnsNil — when journalctl exits non-zero
// the derivation returns (nil, nil) so the renderer omits the line. This is
// the systemd analogue of the FR-029 timeout / no-data branch.
//
// @aitri-trace FR-029 AC-029-009 TC-TMU-029-journal-err
func TestTC_TMU_029_JournalErrorReturnsNil(t *testing.T) {
	d := New(nil)
	d.Exec = fakeExec("", errors.New("journalctl: permission denied"))
	c, err := d.Systemd(context.Background(), "nginx.service")
	if err != nil {
		t.Fatalf("err = %v; want nil (no error surfaced)", err)
	}
	if c != nil {
		t.Fatalf("Cause = %+v; want nil", c)
	}
}

// TestTC_TMU_029_DockerLogsFallback — when no exit-code mapping matches and
// docker logs has output, the last non-empty line is used.
//
// @aitri-trace FR-029 AC-029-006 TC-TMU-029-docker-logs
func TestTC_TMU_029_DockerLogsFallback(t *testing.T) {
	d := New(nil)
	d.Exec = fakeExec("starting\nready\nconnection refused\n", nil)
	c, err := d.Docker(context.Background(), "mealie", 1) // exit code 1 not in map
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := "cause: connection refused"
	if c == nil || c.Line != want {
		t.Fatalf("Line = %q; want %q", lineOf(c), want)
	}
}

// TestTC_TMU_029_ResolveSurfaceCallerSuppresses confirms by contract that
// the cause package never produces output for resolve events — resolve
// events never call this package; the renderer skips it. So we only test
// the input contract: an empty metricID returns (nil, nil).
//
// @aitri-trace FR-029 AC-029-007 TC-TMU-029r
func TestTC_TMU_029r_UnknownMetricReturnsNil(t *testing.T) {
	d := New(&stubProc{samples: []ProcSample{{Comm: "x", CPUPct: 50}}})
	c, err := d.Resource(context.Background(), "totally_unknown", ResourceData{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c != nil {
		t.Fatalf("Cause = %+v; want nil for unknown metric ID", c)
	}
}

// helper for diagnostic messages
func lineOf(c *Cause) string {
	if c == nil {
		return "<nil>"
	}
	return c.Line
}
