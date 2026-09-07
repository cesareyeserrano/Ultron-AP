// Tests for the helper's read-only Docker surface.
//
// The point of most of these is NEGATIVE: that a write action does not exist,
// that a bad id never reaches the daemon, and that the existing actions are
// unchanged. A fake daemon counts every request it receives so "rejected" can
// be distinguished from "rejected after asking".
//
// @aitri-trace FR-089 FR-090 FR-095 NFR-092 NFR-096 NFR-097
package main

import (
	"context"
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

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/dockerapi"
	"github.com/cesareyeserrano/ultron-ap/internal/logfilter"
	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
)

type fakeDaemon struct {
	mu    sync.Mutex
	paths []string
	h     func(w http.ResponseWriter, r *http.Request)
	sock  string
}

// newFakeDockerd starts an HTTP server on a Unix socket and points the
// package-level dockerClient at it for the duration of the test.
func newFakeDockerd(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) *fakeDaemon {
	t.Helper()
	dir, err := os.MkdirTemp("", "hd")
	require.NoError(t, err)
	sock := filepath.Join(dir, "d.sock")

	d := &fakeDaemon{h: h, sock: sock}
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.paths = append(d.paths, r.URL.Path)
		d.mu.Unlock()
		d.h(w, r)
	})}
	go func() { _ = srv.Serve(ln) }()

	prev := dockerClient
	dockerClient = dockerapi.New(sock, 2*time.Second)
	t.Cleanup(func() {
		dockerClient = prev
		_ = srv.Close()
		_ = os.RemoveAll(dir)
	})
	return d
}

func (d *fakeDaemon) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.paths)
}

// listDaemon serves a two-container list plus stats for the running one.
func listDaemon(t *testing.T) *fakeDaemon {
	return newFakeDockerd(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/stats"):
			_, _ = w.Write([]byte(`{"cpu_stats":{"cpu_usage":{"total_usage":200},"system_cpu_usage":2000,"online_cpus":4},"precpu_stats":{"cpu_usage":{"total_usage":100},"system_cpu_usage":1000},"memory_stats":{"usage":268435456,"limit":1073741824}}`))
		case strings.Contains(r.URL.Path, "/logs"):
			_, _ = w.Write([]byte("line one\nline two\n"))
		case r.URL.Path == "/containers/json":
			_, _ = w.Write([]byte(`[{"Id":"aaa111","Names":["/homeassistant"],"Image":"ha:latest","State":"running","Status":"Up 2 hours","Created":1700000000},
			                        {"Id":"bbb222","Names":["/ledger"],"Image":"ledger:1","State":"exited","Status":"Exited (0) 3 hours ago","Created":1700000000}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// call runs one request through dispatch — the same entry point handleConn
// uses once a connection has passed the SO_PEERCRED check.
func call(t *testing.T, action string, payload any) privileged.Response {
	t.Helper()
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		require.NoError(t, err)
		raw = b
	}
	return dispatch(privileged.Request{Action: action, Payload: raw})
}

// @aitri-tc TC-DVH-010h — docker.list is served and returns the container
// list with stats merged in (AC-089-004).
func TestTC_DVH_010h(t *testing.T) {
	listDaemon(t)
	resp := call(t, "docker.list", map[string]any{})
	require.True(t, resp.OK, "message=%s", resp.Message)

	var got []docker.ContainerInfo
	require.NoError(t, json.Unmarshal(resp.Payload, &got))
	require.Len(t, got, 2)
	assert.Equal(t, "homeassistant", got[0].Name)
	assert.Equal(t, "ha:latest", got[0].Image)
	assert.Equal(t, "ledger", got[1].Name)
	// 100/1000 * 4 cores * 100 = 40%
	assert.InDelta(t, 40.0, got[0].CPUPercent, 0.01)
	assert.InDelta(t, 25.0, got[0].MemPercent, 0.01)
}

// @aitri-tc TC-DVH-011f — docker.start does not exist as an action, and no
// request reaches the daemon (AC-089-001).
func TestTC_DVH_011f(t *testing.T) {
	d := listDaemon(t)
	resp := call(t, "docker.start", map[string]any{"id": "aaa111"})
	assert.False(t, resp.OK)
	assert.Equal(t, "unknown action", resp.Message)
	assert.Equal(t, 0, d.count(), "a write action must never reach the daemon")
}

// @aitri-tc TC-DVH-012f — stop, restart and exec are equally absent
// (AC-089-001).
func TestTC_DVH_012f(t *testing.T) {
	d := listDaemon(t)
	for _, action := range []string{"docker.stop", "docker.restart", "docker.exec"} {
		resp := call(t, action, map[string]any{"id": "aaa111"})
		assert.False(t, resp.OK, "%s must not be served", action)
		assert.Equal(t, "unknown action", resp.Message, "%s", action)
	}
	assert.Equal(t, 0, d.count(), "no write action may reach the daemon")
}

// @aitri-tc TC-DVH-070h — container logs are served through the helper
// (AC-095-001).
func TestTC_DVH_070h(t *testing.T) {
	listDaemon(t)
	resp := call(t, "docker.logs", map[string]any{"id": "aaa111", "lines": 100})
	require.True(t, resp.OK, "message=%s", resp.Message)

	var out string
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	assert.Equal(t, "line one\nline two\n", out)
}

// @aitri-tc TC-DVH-073f — an invalid id in docker.logs errors out and returns
// no other container's output (AC-095-004).
func TestTC_DVH_073f(t *testing.T) {
	d := listDaemon(t)
	resp := call(t, "docker.logs", map[string]any{"id": "../aaa111", "lines": 100})
	assert.False(t, resp.OK)
	assert.Contains(t, resp.Message, "invalid container id")
	assert.NotContains(t, string(resp.Payload), "line one")
	assert.Equal(t, 0, d.count(), "a rejected id must not reach the daemon")
}

// @aitri-tc TC-DVH-083e — id validation is applied on every action that takes
// one; only the id-less docker.list reaches the daemon (AC-090-001).
func TestTC_DVH_083e(t *testing.T) {
	d := listDaemon(t)
	assert.False(t, call(t, "docker.inspect", map[string]any{"id": "../../info"}).OK)
	assert.False(t, call(t, "docker.logs", map[string]any{"id": "../../info", "lines": 10}).OK)
	require.True(t, call(t, "docker.list", map[string]any{}).OK)

	// docker.list issues 1 list call + 1 stats call for the single running
	// container; the two rejected ids contribute nothing.
	assert.Equal(t, 2, d.count(), "only docker.list may have reached the daemon")
}

// @aitri-tc TC-DVH-090h — ping is unchanged by the new actions (NFR-096).
func TestTC_DVH_090h(t *testing.T) {
	resp := call(t, "ping", nil)
	assert.True(t, resp.OK)
	assert.Equal(t, "pong", resp.Message)
}

// @aitri-tc TC-DVH-091f — an unknown action still reports "unknown action"
// (NFR-096).
func TestTC_DVH_091f(t *testing.T) {
	resp := call(t, "no-such-action", nil)
	assert.False(t, resp.OK)
	assert.Equal(t, "unknown action", resp.Message)
}

// @aitri-tc TC-DVH-092e — systemctl still rejects an option-shaped unit name
// (NFR-096).
func TestTC_DVH_092e(t *testing.T) {
	resp := call(t, "systemctl", map[string]any{"action": "start", "name": "-M evil"})
	assert.False(t, resp.OK)
	assert.Contains(t, resp.Message, "invalid service name")
}

// @aitri-tc TC-DVH-071f — a secret in container output is redacted before it
// leaves the helper (AC-095-002).
func TestTC_DVH_071f(t *testing.T) {
	in := []byte("app[1]: TOKEN=abc123secret starting\n")
	got, err := finalizeLog(in, nil, logfilter.PolicyJournal)
	require.NoError(t, err)
	assert.NotContains(t, got, "abc123secret", "a secret must not survive redaction")
	assert.Contains(t, got, "TOKEN=", "the line itself must survive, masked")
}

// @aitri-tc TC-DVH-101f — container logs and journalctl logs go through the
// SAME policy: identical input must yield byte-identical output. A laxer
// container path is exactly what this asserts cannot exist (NFR-097).
func TestTC_DVH_101f(t *testing.T) {
	in := []byte("app[1]: TOKEN=abc123secret and Authorization: Bearer eyJa.b.c\n")

	viaJournal, err := finalizeLog(in, nil, logfilter.PolicyJournal)
	require.NoError(t, err)

	newFakeDockerd(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/logs") {
			_, _ = w.Write(in)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	viaContainer, err := handleDockerLogs(context.Background(), "aaa111", 100)
	require.NoError(t, err)

	assert.Equal(t, viaJournal, viaContainer,
		"the container log path must not use a laxer redaction policy than journalctl")
}

// @aitri-tc TC-DVH-100h — journalctl redaction is unchanged (NFR-097).
func TestTC_DVH_100h(t *testing.T) {
	in := []byte("Apr 23 10:00 host app[1]: TOKEN=deadbeef starting\n")
	got, err := finalizeLog(in, nil, logfilter.PolicyJournal)
	require.NoError(t, err)
	assert.NotContains(t, got, "deadbeef")
}

// @aitri-tc TC-DVH-072e — container output is capped by the same byte ceiling
// journalctl output is (AC-095-003).
func TestTC_DVH_072e(t *testing.T) {
	big := []byte(strings.Repeat("x", 5<<20)) // 5 MiB
	got, err := finalizeLog(big, nil, logfilter.PolicyJournal)
	require.NoError(t, err)
	assert.Less(t, len(got), len(big), "output must be capped, not returned whole")
}

// @aitri-tc TC-DVH-102e — the cap is identical on both paths (NFR-097).
func TestTC_DVH_102e(t *testing.T) {
	big := []byte(strings.Repeat("y", 5<<20))
	viaJournal, err := finalizeLog(big, nil, logfilter.PolicyJournal)
	require.NoError(t, err)

	newFakeDockerd(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/logs") {
			_, _ = w.Write(big)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	viaContainer, err := handleDockerLogs(context.Background(), "aaa111", 100)
	require.NoError(t, err)

	assert.Equal(t, len(viaJournal), len(viaContainer), "both paths must cap at the same length")
}

// @aitri-tc TC-DVH-080h — a caller whose UID is on the allowlist reaches the
// Docker actions, over a real Unix socket, through handleConn (AC-089-004).
func TestTC_DVH_080h(t *testing.T) {
	listDaemon(t)

	dir, err := os.MkdirTemp("", "hsock")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "h.sock")

	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	// Authorise the UID this test process actually runs as.
	prev := allowedUIDs
	allowedUIDs = map[uint32]struct{}{uint32(os.Getuid()): {}}
	defer func() { allowedUIDs = prev }()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		handleConn(c)
	}()

	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

	_, err = conn.Write([]byte(`{"action":"docker.list","payload":{}}` + "\n"))
	require.NoError(t, err)

	var resp privileged.Response
	require.NoError(t, json.NewDecoder(conn).Decode(&resp))
	require.True(t, resp.OK, "message=%s", resp.Message)

	var got []docker.ContainerInfo
	require.NoError(t, json.Unmarshal(resp.Payload, &got))
	require.Len(t, got, 2)

	// Assert the payload actually carries the container data, not just the
	// right number of empty structs: an authorised round trip that returned
	// two blanks would satisfy a length check and prove nothing.
	assert.Equal(t, "homeassistant", got[0].Name)
	assert.Equal(t, "ha:latest", got[0].Image)
	assert.Equal(t, docker.HealthRunning, got[0].Health)
	assert.Equal(t, "ledger", got[1].Name)
	assert.Equal(t, docker.HealthStopped, got[1].Health)
	assert.InDelta(t, 40.0, got[0].CPUPercent, 0.01, "stats must survive the socket round trip")
}

// @aitri-tc TC-DVH-081f — a caller whose UID is not on the allowlist is
// refused, and the refusal happens before the request line is read, so no
// Docker action can run (NFR-092).
func TestTC_DVH_081f(t *testing.T) {
	allow := map[uint32]struct{}{65534: {}}
	msg, logLine, ok := authorize(true, allow, 1000, nil)

	assert.False(t, ok, "uid 1000 is not on the allowlist and must be refused")
	assert.Equal(t, "forbidden", msg)
	assert.Contains(t, logLine, "unauthorised uid=1000")

	// The allowed UID is admitted, proving the check discriminates rather
	// than refusing everything.
	_, _, ok = authorize(true, allow, 65534, nil)
	assert.True(t, ok, "the allow-listed uid must be admitted")
}

// @aitri-tc TC-DVH-082f — with enforcement on and no allowlist resolvable the
// helper fails closed for Docker actions too (NFR-092, BG-043).
func TestTC_DVH_082f(t *testing.T) {
	msg, logLine, ok := authorize(true, map[uint32]struct{}{}, 1000, nil)
	assert.False(t, ok, "an empty allowlist must fail closed, not open")
	assert.Equal(t, "forbidden", msg)
	assert.Contains(t, logLine, "fail-closed")

	msg, _, ok = authorize(true, nil, 1000, nil)
	assert.False(t, ok, "a nil allowlist must fail closed too")
	assert.Equal(t, "forbidden", msg)
}
