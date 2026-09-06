package dockerapi

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDaemon is an httptest server bound to a Unix socket in a temp dir. It
// records every request it receives so a test can assert that a rejected id
// produced NO traffic at all — the difference between "validated" and
// "validated after asking the daemon".
type fakeDaemon struct {
	mu      sync.Mutex
	methods []string
	paths   []string
	handler func(w http.ResponseWriter, r *http.Request)
	srv     *http.Server
	sock    string
}

func newFakeDaemon(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *fakeDaemon {
	t.Helper()
	// Unix socket paths are capped near 104 bytes on darwin, and t.TempDir()
	// under a long test name can exceed it — use the short system temp dir.
	dir, err := os.MkdirTemp("", "dapi")
	require.NoError(t, err)
	sock := filepath.Join(dir, "d.sock")

	d := &fakeDaemon{handler: h, sock: sock}
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)

	d.srv = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.methods = append(d.methods, r.Method)
		d.paths = append(d.paths, r.URL.Path)
		d.mu.Unlock()
		d.handler(w, r)
	})}
	go func() { _ = d.srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = d.srv.Close()
		_ = os.RemoveAll(dir)
	})
	return d
}

func (d *fakeDaemon) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.paths)
}

func (d *fakeDaemon) seenMethods() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.methods...)
}

func (d *fakeDaemon) client() *Client { return New(d.sock, 2*time.Second) }

// notFoundDaemon answers 404 to everything.
func notFoundDaemon(t *testing.T) *fakeDaemon {
	return newFakeDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
}

// silentDaemon answers 200 with an empty JSON array. Used when the point of
// the test is that NO request should arrive.
func silentDaemon(t *testing.T) *fakeDaemon {
	return newFakeDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
}

// @aitri-tc TC-DVH-013f — a path-traversal id is rejected before any request
// is built, so the daemon sees nothing (AC-089-003).
func TestTC_DVH_013f(t *testing.T) {
	d := silentDaemon(t)
	_, err := d.client().Inspect(context.Background(), "../../info")
	require.ErrorIs(t, err, ErrInvalidID)
	assert.Equal(t, 0, d.count(), "a rejected id must not reach the socket")
}

// @aitri-tc TC-DVH-014f — a URL-encoded traversal is rejected the same way
// (AC-089-003).
func TestTC_DVH_014f(t *testing.T) {
	d := silentDaemon(t)
	_, err := d.client().Inspect(context.Background(), "%2e%2e%2finfo")
	require.ErrorIs(t, err, ErrInvalidID)
	assert.Equal(t, 0, d.count(), "a rejected id must not reach the socket")
}

// @aitri-tc TC-DVH-015f — special characters, a NUL byte and an over-long id
// are all rejected without touching the socket (AC-089-003).
func TestTC_DVH_015f(t *testing.T) {
	d := silentDaemon(t)
	cli := d.client()
	for _, id := range []string{
		";DROP TABLE containers",
		"abc\x00def",
		strings.Repeat("a", 200),
	} {
		_, err := cli.Inspect(context.Background(), id)
		require.ErrorIs(t, err, ErrInvalidID, "id %q must be rejected", id)
	}
	assert.Equal(t, 0, d.count(), "no rejected id may reach the socket")
}

// @aitri-tc TC-DVH-016e — every call this client can make issues a GET
// (AC-089-002).
func TestTC_DVH_016e(t *testing.T) {
	d := newFakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/json") && strings.Contains(r.URL.Path, "/containers/abc123"):
			_, _ = w.Write([]byte(`{"Config":{"Env":[]}}`))
		case strings.Contains(r.URL.Path, "/stats"):
			_, _ = w.Write([]byte(`{"memory_stats":{"usage":1,"limit":2}}`))
		case strings.Contains(r.URL.Path, "/logs"):
			_, _ = w.Write([]byte("plain"))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	})
	cli := d.client()
	ctx := context.Background()
	_, _ = cli.Containers(ctx)
	_, _ = cli.Stats(ctx, "abc123")
	_, _ = cli.Inspect(ctx, "abc123")
	_, _ = cli.Logs(ctx, "abc123", 100)

	methods := d.seenMethods()
	require.Len(t, methods, 4, "all four calls must reach the daemon")
	for i, m := range methods {
		assert.Equal(t, http.MethodGet, m, "request %d must be a GET", i)
	}
}

// @aitri-tc TC-DVH-020h — inspect yields the container's env var NAMES
// (AC-090-001).
func TestTC_DVH_020h(t *testing.T) {
	d := newFakeDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Config":{"Env":["PATH=/usr/bin","TZ=America/Bogota"]}}`))
	})
	insp, err := d.client().Inspect(context.Background(), "abc123")
	require.NoError(t, err)
	require.Len(t, insp.Config.Env, 2)
	assert.Equal(t, []string{"PATH=/usr/bin", "TZ=America/Bogota"}, insp.Config.Env)
	assert.Equal(t, []string{"PATH", "TZ"}, EnvNames(insp.Config.Env))
}

// @aitri-tc TC-DVH-021f — a secret's VALUE never survives the projection to
// the wire type; only its name does (AC-090-001).
func TestTC_DVH_021f(t *testing.T) {
	names := EnvNames([]string{"POSTGRES_PASSWORD=hunter2", "API_KEY=sk-live-9f3a"})
	blob, err := json.Marshal(names)
	require.NoError(t, err)
	s := string(blob)

	assert.Contains(t, s, "POSTGRES_PASSWORD")
	assert.Contains(t, s, "API_KEY")
	assert.NotContains(t, s, "hunter2", "an env VALUE must never cross the boundary")
	assert.NotContains(t, s, "sk-live-9f3a", "an env VALUE must never cross the boundary")
}

// @aitri-tc TC-DVH-022e — published ports decode with host port, container
// port and protocol (AC-090-002).
func TestTC_DVH_022e(t *testing.T) {
	d := newFakeDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"NetworkSettings":{"Ports":{"8123/tcp":[{"HostIp":"0.0.0.0","HostPort":"8123"}]}}}`))
	})
	insp, err := d.client().Inspect(context.Background(), "abc123")
	require.NoError(t, err)
	binds := insp.NetworkSettings.Ports["8123/tcp"]
	require.Len(t, binds, 1)
	assert.Equal(t, "8123", binds[0].HostPort)
	assert.Equal(t, "tcp", ProtoOf("8123/tcp"))
}

// @aitri-tc TC-DVH-023e — mounts decode with source, destination and mode
// (AC-090-003).
func TestTC_DVH_023e(t *testing.T) {
	d := newFakeDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Mounts":[{"Source":"/srv/ha","Destination":"/config","Mode":"rw"}]}`))
	})
	insp, err := d.client().Inspect(context.Background(), "abc123")
	require.NoError(t, err)
	require.Len(t, insp.Mounts, 1)
	assert.Equal(t, "/srv/ha", insp.Mounts[0].Source)
	assert.Equal(t, "/config", insp.Mounts[0].Destination)
	assert.Equal(t, "rw", insp.Mounts[0].Mode)
}

// @aitri-tc TC-DVH-024f — a missing container is an error, not an empty
// detail that would render as "a container with no ports" (AC-090-004).
func TestTC_DVH_024f(t *testing.T) {
	d := notFoundDaemon(t)
	insp, err := d.client().Inspect(context.Background(), "nosuch")
	require.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, insp, "a not-found container must yield nil, never a zero-valued detail")
}

// @aitri-tc TC-DVH-074e — the log de-multiplexer handles whole frames, a
// payload split across reads, and a truncated header, without panicking
// (AC-095-001).
func TestTC_DVH_074e(t *testing.T) {
	frame := func(stream byte, payload string) []byte {
		h := make([]byte, frameHeaderLen)
		h[0] = stream
		binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
		return append(h, payload...)
	}

	// (a) two complete frames, stdout then stderr, concatenated in order.
	whole := append(frame(1, "out-line\n"), frame(2, "err-line\n")...)
	assert.Equal(t, "out-line\nerr-line\n", string(Demux(whole)))

	// (b) a frame whose payload is shorter than its declared length — the
	// readable part survives.
	split := frame(1, "hello world")
	split = split[:frameHeaderLen+5] // declares 11 bytes, carries 5
	assert.Equal(t, "hello", string(Demux(split)))

	// (c) a truncated 5-byte header carries no payload and must not panic.
	assert.NotPanics(t, func() { _ = Demux([]byte{1, 0, 0, 0, 9}) })

	// Unframed TTY output is passed through untouched.
	assert.Equal(t, "plain text\n", string(Demux([]byte("plain text\n"))))
}

// CPU and memory percentage maths moved here with the transport when the SDK
// was dropped. These carry over the coverage the old
// internal/docker.calculateCPUPercent tests had.
func TestStatsSnapshot_CPUPercent(t *testing.T) {
	s := &StatsSnapshot{}
	s.CPUStats.CPUUsage.TotalUsage = 200_000_000
	s.CPUStats.SystemUsage = 1_000_000_000
	s.CPUStats.OnlineCPUs = 4
	s.PreCPUStats.CPUUsage.TotalUsage = 100_000_000
	s.PreCPUStats.SystemUsage = 500_000_000
	// cpuDelta=100M, sysDelta=500M, ratio=0.2, cpus=4 → 80%
	assert.InDelta(t, 80.0, s.CPUPercent(), 0.1)
}

func TestStatsSnapshot_CPUPercent_ZeroDelta(t *testing.T) {
	s := &StatsSnapshot{}
	s.CPUStats.CPUUsage.TotalUsage = 100
	s.CPUStats.SystemUsage = 100
	s.PreCPUStats.CPUUsage.TotalUsage = 100
	s.PreCPUStats.SystemUsage = 100
	assert.Equal(t, 0.0, s.CPUPercent(), "no delta yet is 0%, not a division by zero")
}

func TestStatsSnapshot_CPUPercent_DefaultsToOneCPU(t *testing.T) {
	s := &StatsSnapshot{}
	s.CPUStats.CPUUsage.TotalUsage = 200
	s.CPUStats.SystemUsage = 2000
	s.CPUStats.OnlineCPUs = 0 // daemon did not report it
	s.PreCPUStats.CPUUsage.TotalUsage = 100
	s.PreCPUStats.SystemUsage = 1000
	assert.InDelta(t, 10.0, s.CPUPercent(), 0.01, "a missing CPU count must read as 1, not 0")
}

func TestStatsSnapshot_MemPercent(t *testing.T) {
	s := &StatsSnapshot{}
	s.MemoryStats.Usage = 268435456
	s.MemoryStats.Limit = 1073741824
	assert.InDelta(t, 25.0, s.MemPercent(), 0.01)

	s.MemoryStats.Limit = 0
	assert.Equal(t, 0.0, s.MemPercent(), "no limit reported must not divide by zero")
}
