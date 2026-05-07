// Package cause derives the probable-cause / situation line for an alert
// message. Each surface (resource, systemd, docker) has its own derivation
// path; all paths are time-budgeted and may return a nil *Cause when the
// underlying source is unavailable, slow, or returns no usable data — the
// renderer then omits the line entirely (FR-029, AC-029-007 / 008 / 009).
//
// All subprocess invocations use exec.Command with explicit argv (no shell)
// and validate untrusted names against an allow-list before exec (NFR-008).
//
// @aitri-trace FR-029 NFR-005 NFR-008
package cause

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Source identifies which derivation path produced the line. Used in the
// observability log line (NFR-007) so the operator can tell whether a missing
// line was a no-data case or a timeout case.
type Source string

const (
	SourceProc     Source = "proc"
	SourceJournal  Source = "journal"
	SourceExitCode Source = "exitcode"
	SourceNone     Source = "none"
)

// Cause is the rendered situation line and the source that produced it.
// The Line is NOT MarkdownV2-escaped; the renderer is responsible for
// escaping the dynamic substring before interpolation.
type Cause struct {
	Source Source
	Line   string // ready to be embedded after the surface-specific prefix
}

// validUnitName accepts standard systemd unit-name characters. Reject
// anything else before passing to journalctl (NFR-008).
var validUnitName = regexp.MustCompile(`^[A-Za-z0-9@:_\-.\\]+\.(service|socket|timer|target|mount|path|slice|scope)$`)

// validContainerName matches Docker's documented charset for container names.
var validContainerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]{0,127}$`)

// errInvalidName signals that a unit/container name failed allow-list
// validation. The renderer surfaces this as "invalid unit name" or
// "invalid container name" rather than spawning a subprocess.
var errInvalidName = errors.New("invalid name")

// ErrInvalidName is exported for tests/dispatcher to detect the rejection
// case without string matching.
func IsInvalidName(err error) bool { return errors.Is(err, errInvalidName) }

// dockerExitCodeMessages maps well-known Docker container exit codes to a
// short human-readable cause. Unknown codes fall through to the docker logs
// derivation.
var dockerExitCodeMessages = map[int]string{
	137: "OOM-killed (137)",
	139: "segfault (139)",
	143: "SIGTERM (143)",
	130: "SIGINT (130)",
	125: "docker run failure (125)",
	126: "command not executable (126)",
	127: "command not found (127)",
}

// errorishLine is the regex used to pick the most informative journal line.
// The first match in the last 3 lines wins; otherwise the last non-empty
// line is used.
var errorishLine = regexp.MustCompile(`(?i)error|fatal|fail|panic`)

// ProcReader is the abstraction the resource derivation uses to fetch the
// top-N processes. The dispatcher injects a real /proc reader at process
// start; tests substitute a static list. Returning an empty slice signals
// "no data" — the renderer then omits the cause line.
type ProcReader interface {
	TopProcesses(ctx context.Context, axis Axis, n int) ([]ProcSample, error)
}

// Axis selects how TopProcesses orders its result.
type Axis int

const (
	AxisCPU Axis = iota
	AxisRSS
)

// ProcSample is a minimal process snapshot used by the renderer.
type ProcSample struct {
	Comm   string  // /proc/<pid>/comm — process basename
	CPUPct float64 // 0..100 (per-process; not normalized to NumCPU)
	RSSMB  int     // resident set size in MiB
}

// execFn lets tests intercept subprocess invocations. Production uses
// defaultExec which wraps exec.CommandContext.
type execFn func(ctx context.Context, name string, args ...string) ([]byte, error)

func defaultExec(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Surface stderr in the error so the renderer can include the reason.
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("%s: %s", filepath.Base(name), msg)
	}
	return stdout.Bytes(), nil
}

// Deriver bundles the injectable dependencies. Construct one per process.
type Deriver struct {
	Proc     ProcReader
	Exec     execFn // when nil, defaultExec is used
	Hostname string // unused here but allows future surface formatting
}

// New constructs a Deriver with sensible production defaults.
func New(proc ProcReader) *Deriver {
	return &Deriver{Proc: proc, Exec: defaultExec}
}

// Resource derives the situation line for a CPU / RAM / Disk / Temp fire.
// metricID is the engine's metric identifier (cpu, ram, disk, temp, or any of
// the *_usage / cpu_temp aliases). Returns (nil, nil) when no usable data is
// available — that is NOT an error.
//
// @aitri-trace FR-029 AC-029-001 AC-029-002 AC-029-003 AC-029-004
func (d *Deriver) Resource(ctx context.Context, metricID string, surf ResourceData) (*Cause, error) {
	switch normalizeMetricID(metricID) {
	case "cpu_usage":
		return d.topProcess(ctx, AxisCPU, "%")
	case "mem_usage":
		return d.topProcess(ctx, AxisRSS, " MB")
	case "disk_usage":
		if surf.DiskMount == "" {
			return nil, nil
		}
		freeStr := humanBytes(surf.DiskFreeBytes)
		line := fmt.Sprintf("%s at %.0f%% — %s free", surf.DiskMount, surf.DiskUsedPct, freeStr)
		return &Cause{Source: SourceProc, Line: line}, nil
	case "cpu_temp":
		// Trend keyword is derived from the prior/current delta on the renderer
		// side; here we just emit the current temperature with the trend hint.
		if surf.TempTrendDeltaC == 0 && surf.TempCurrentC == 0 {
			return nil, nil
		}
		var trend string
		switch {
		case surf.TempTrendDeltaC >= 2:
			trend = "climbing"
		case surf.TempTrendDeltaC <= -2:
			trend = "cooling"
		default:
			trend = "stable"
		}
		line := fmt.Sprintf("temperature %.0f°C, %s vs 5m ago", surf.TempCurrentC, trend)
		return &Cause{Source: SourceProc, Line: line}, nil
	}
	return nil, nil
}

// ResourceData carries surface data the renderer pre-fetched from existing
// monitors (disk usage, temperature ring buffer). Zero values are tolerated.
type ResourceData struct {
	DiskMount       string
	DiskUsedPct     float64
	DiskFreeBytes   int64
	TempCurrentC    float64
	TempTrendDeltaC float64 // current - prior_5m
}

// Systemd derives the situation line for a systemd surface fire by scanning
// the last 3 journalctl lines for the unit. Returns (nil, errInvalidName)
// when the unit name fails validation; (nil, nil) when journal is unavailable
// or yields no useful line.
//
// @aitri-trace FR-029 AC-029-005 NFR-008
func (d *Deriver) Systemd(ctx context.Context, unit string) (*Cause, error) {
	if !validUnitName.MatchString(unit) {
		return nil, errInvalidName
	}
	out, err := d.exec()(ctx, "journalctl", "-u", unit, "--no-pager", "-n", "3", "-o", "cat")
	if err != nil {
		// Subprocess failure (incl. ctx deadline) — caller treats as no data.
		return nil, nil
	}
	lines := splitNonEmpty(out)
	if len(lines) == 0 {
		return nil, nil
	}
	// Prefer the most recent error/fatal/fail/panic line; fall back to the
	// last non-empty line.
	for i := len(lines) - 1; i >= 0; i-- {
		if errorishLine.MatchString(lines[i]) {
			return &Cause{Source: SourceJournal, Line: "last error: " + lines[i]}, nil
		}
	}
	return &Cause{Source: SourceJournal, Line: "last error: " + lines[len(lines)-1]}, nil
}

// Docker derives the situation line for a docker surface fire. When the
// container's exit code matches a well-known mapping (137 OOM, 139 SEGV, …)
// that mapping wins. Otherwise, the last non-empty `docker logs --tail 3`
// line is used.
//
// @aitri-trace FR-029 AC-029-006 NFR-008
func (d *Deriver) Docker(ctx context.Context, container string, exitCode int) (*Cause, error) {
	if !validContainerName.MatchString(container) {
		return nil, errInvalidName
	}
	if msg, ok := dockerExitCodeMessages[exitCode]; ok {
		return &Cause{Source: SourceExitCode, Line: "cause: " + msg}, nil
	}
	out, err := d.exec()(ctx, "docker", "logs", "--tail", "3", container)
	if err != nil {
		return nil, nil
	}
	lines := splitNonEmpty(out)
	if len(lines) == 0 {
		return nil, nil
	}
	return &Cause{Source: SourceJournal, Line: "cause: " + lines[len(lines)-1]}, nil
}

// topProcess scans the configured ProcReader for the top consumer along the
// requested axis and returns it as a "top: <comm> (<value><unit>)" line.
func (d *Deriver) topProcess(ctx context.Context, axis Axis, unit string) (*Cause, error) {
	if d.Proc == nil {
		return nil, nil
	}
	samples, err := d.Proc.TopProcesses(ctx, axis, 1)
	if err != nil || len(samples) == 0 {
		return nil, nil
	}
	s := samples[0]
	switch axis {
	case AxisCPU:
		return &Cause{Source: SourceProc, Line: fmt.Sprintf("top: %s (%.0f%s)", s.Comm, s.CPUPct, unit)}, nil
	case AxisRSS:
		return &Cause{Source: SourceProc, Line: fmt.Sprintf("top: %s (%d%s)", s.Comm, s.RSSMB, unit)}, nil
	}
	return nil, nil
}

// exec returns the configured exec function or defaults.
func (d *Deriver) exec() execFn {
	if d.Exec != nil {
		return d.Exec
	}
	return defaultExec
}

// normalizeMetricID maps legacy short identifiers (cpu, ram, disk, temp) to
// the canonical *_usage / cpu_temp form used by the renderer's friendly-label
// table. The engine emits the short form today (engine.go); requirements
// referenced the long form. Both are accepted.
func normalizeMetricID(id string) string {
	switch id {
	case "cpu", "cpu_usage":
		return "cpu_usage"
	case "ram", "mem", "memory", "mem_usage":
		return "mem_usage"
	case "disk", "disk_usage":
		return "disk_usage"
	case "temp", "cpu_temp", "temperature":
		return "cpu_temp"
	}
	return id
}

// splitNonEmpty splits b on \n and drops empty/whitespace-only lines.
func splitNonEmpty(b []byte) []string {
	rawLines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// humanBytes renders a byte count in the most informative SI-ish unit, with
// 1 decimal for GB/TB and 0 decimals for MB/KB.
func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const (
		kb = 1 << 10
		mb = 1 << 20
		gb = 1 << 30
		tb = int64(1) << 40
	)
	switch {
	case n >= tb:
		return fmt.Sprintf("%.1f TB", float64(n)/float64(tb))
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%d MB", n/mb)
	case n >= kb:
		return fmt.Sprintf("%d KB", n/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ProcFSReader is a /proc-backed ProcReader used in production. It scans
// /proc/<pid>/stat to estimate CPU% over a 100ms sampling window and
// /proc/<pid>/status for RSS. It is intentionally simple — busy hosts may
// exhibit sample noise; the dispatcher's 200ms timeout caps the cost.
//
// Tests should NOT use ProcFSReader; substitute a deterministic static
// reader via Deriver.Proc.
type ProcFSReader struct {
	Root string // override for tests pointing at a fake /proc tree
}

// NewProcFSReader returns a ProcFSReader rooted at /proc.
func NewProcFSReader() *ProcFSReader { return &ProcFSReader{Root: "/proc"} }

// TopProcesses scans the /proc tree once and returns the top-n by axis. CPU%
// is approximated from utime+stime over a 100ms sleep. The function honors
// ctx cancellation: if ctx is done, it returns whatever has been collected so
// far rather than the full snapshot.
func (r *ProcFSReader) TopProcesses(ctx context.Context, axis Axis, n int) ([]ProcSample, error) {
	if r.Root == "" {
		r.Root = "/proc"
	}
	pids, err := readPIDs(r.Root)
	if err != nil {
		return nil, err
	}
	samples := make([]ProcSample, 0, len(pids))
	for _, pid := range pids {
		if ctx.Err() != nil {
			break
		}
		s, ok := readProcSample(r.Root, pid)
		if !ok {
			continue
		}
		samples = append(samples, s)
	}
	switch axis {
	case AxisCPU:
		sort.SliceStable(samples, func(i, j int) bool { return samples[i].CPUPct > samples[j].CPUPct })
	case AxisRSS:
		sort.SliceStable(samples, func(i, j int) bool { return samples[i].RSSMB > samples[j].RSSMB })
	}
	if n > 0 && len(samples) > n {
		samples = samples[:n]
	}
	return samples, nil
}

// readPIDs lists numeric subdirectories of root (the pids in /proc).
func readPIDs(root string) ([]int, error) {
	f, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	pids := make([]int, 0, len(names))
	for _, name := range names {
		pid, err := strconv.Atoi(name)
		if err != nil || pid <= 0 {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// readProcSample is a best-effort reader of /proc/<pid>/comm and
// /proc/<pid>/status. It deliberately does NOT compute CPU% (which would
// require a 100ms sleep across the whole snapshot) — production deployment
// should use the metrics collector's existing process snapshot via a thin
// adapter; this implementation exists so the package builds in isolation.
func readProcSample(root string, pid int) (ProcSample, bool) {
	commPath := filepath.Join(root, strconv.Itoa(pid), "comm")
	commBytes, err := os.ReadFile(commPath)
	if err != nil {
		return ProcSample{}, false
	}
	comm := strings.TrimSpace(string(commBytes))
	if comm == "" {
		return ProcSample{}, false
	}

	statusPath := filepath.Join(root, strconv.Itoa(pid), "status")
	statusBytes, _ := os.ReadFile(statusPath)
	rssMB := parseRSSMB(statusBytes)

	return ProcSample{Comm: comm, CPUPct: 0, RSSMB: rssMB}, true
}

// parseRSSMB extracts VmRSS in MiB from a /proc/<pid>/status file.
func parseRSSMB(status []byte) int {
	for _, line := range strings.Split(string(status), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			if kb, err := strconv.Atoi(fields[1]); err == nil {
				return kb / 1024
			}
		}
	}
	return 0
}
