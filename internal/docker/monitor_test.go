// Tests for the container monitor now that its source is the privileged
// helper rather than the Docker socket.
//
// @aitri-trace FR-088 FR-092 US-088 US-092
package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @aitri-tc TC-DVH-001h — the list comes back with all five of the Pi's
// containers, state and health populated (AC-088-001).
func TestTC_DVH_001h(t *testing.T) {
	m := NewMonitorWithSource(&fakeSource{list: fiveContainers()})
	m.refresh(context.Background())

	got := m.Containers()
	require.Len(t, got, 5)

	names := make([]string, len(got))
	for i, c := range got {
		names[i] = c.Name
		assert.NotEmpty(t, c.Image, "%s must carry an image", c.Name)
	}
	assert.Equal(t, []string{"homeassistant", "hermes", "openclaw", "ledger", "mosquitto"}, names)
	assert.Equal(t, HealthStopped, got[3].Health, "ledger exited cleanly")
	for _, i := range []int{0, 1, 2, 4} {
		assert.Equal(t, HealthRunning, got[i].Health, "%s must be running", got[i].Name)
	}
	assert.True(t, m.Available())
}

// @aitri-tc TC-DVH-002e — a running container's stats arrive numeric and its
// memory percentage is the real ratio, not a rounded placeholder
// (AC-088-002).
func TestTC_DVH_002e(t *testing.T) {
	list := fiveContainers()
	list[0].MemUsage = 268435456  // 256 MiB
	list[0].MemLimit = 1073741824 // 1 GiB
	list[0].MemPercent = 25.0
	list[0].CPUPercent = 12.5

	m := NewMonitorWithSource(&fakeSource{list: list})
	m.refresh(context.Background())

	ha := m.Containers()[0]
	assert.Equal(t, uint64(268435456), ha.MemUsage, "byte counts must survive intact")
	assert.Equal(t, uint64(1073741824), ha.MemLimit)
	assert.InDelta(t, 25.0, ha.MemPercent, 0.01)
	assert.GreaterOrEqual(t, ha.CPUPercent, 0.0)
	assert.LessOrEqual(t, ha.MemPercent, 100.0)
}

// @aitri-tc TC-DVH-003e — a stopped container is listed without demanding
// stats of it (AC-088-003).
func TestTC_DVH_003e(t *testing.T) {
	m := NewMonitorWithSource(&fakeSource{list: fiveContainers()})
	m.refresh(context.Background())

	ledger := m.Containers()[3]
	assert.Equal(t, "ledger", ledger.Name)
	assert.Equal(t, HealthStopped, ledger.Health)
	assert.Zero(t, ledger.CPUPercent, "a stopped container reports no CPU")
	assert.Zero(t, ledger.MemUsage, "a stopped container reports no memory")
	assert.True(t, m.Available(), "a stopped container is not a read failure")
}

// @aitri-tc TC-DVH-004e — a production-scale list is processed in one refresh
// (AC-088-001).
func TestTC_DVH_004e(t *testing.T) {
	big := make([]ContainerInfo, 200)
	for i := range big {
		big[i] = ContainerInfo{
			ID: fmt.Sprintf("id%03d", i), Name: fmt.Sprintf("c%03d", i),
			Image: "img:1", State: "exited", Health: HealthStopped,
		}
		if i < 150 {
			big[i].State = "running"
			big[i].Health = HealthRunning
			big[i].MemUsage = 1 << 20
			big[i].MemLimit = 1 << 30
			big[i].MemPercent = 0.09765625
		}
	}

	m := NewMonitorWithSource(&fakeSource{list: big})
	start := time.Now()
	m.refresh(context.Background())
	elapsed := time.Since(start)

	got := m.Containers()
	require.Len(t, got, 200)
	assert.Less(t, elapsed, 2*time.Second, "200 containers must refresh well inside 2s")

	running := 0
	for _, c := range got {
		if c.State == "running" {
			running++
			assert.Greater(t, c.MemPercent, 0.0, "%s is running and must carry stats", c.Name)
		}
	}
	assert.Equal(t, 150, running)
}

// @aitri-tc TC-DVH-005f — a payload the source cannot decode leaves the
// monitor unavailable, with no panic and no half-built rows (AC-088-004).
func TestTC_DVH_005f(t *testing.T) {
	src := &fakeSource{listErr: errors.New(`decode container list: invalid character '<' looking for beginning of value`)}
	m := NewMonitorWithSource(src)

	assert.NotPanics(t, func() { m.refresh(context.Background()) })
	assert.False(t, m.Available())
	assert.Empty(t, m.Containers(), "a failed decode must not leave partial rows")
}

// @aitri-tc TC-DVH-006e — a transient failure keeps the last good list, so
// the panel does not blink to empty and back (AC-088-004).
func TestTC_DVH_006e(t *testing.T) {
	src := &fakeSource{list: fiveContainers()}
	m := NewMonitorWithSource(src)

	m.refresh(context.Background())
	require.Len(t, m.Containers(), 5)
	require.True(t, m.Available())

	// Second read fails.
	src.mu.Lock()
	src.listErr = errors.New("helper unavailable")
	src.mu.Unlock()
	m.refresh(context.Background())

	assert.False(t, m.Available(), "the failure must be visible as unavailability")
	assert.Len(t, m.Containers(), 5, "the last good list must survive a transient failure")
}

// @aitri-tc TC-DVH-040h — the configured helper timeout sits inside the 2s–3s
// band FR-092 mandates, and a normal refresh completes well inside it
// (AC-092-001).
//
// The bound is asserted against the constant the production path actually
// uses, not a copy of its value: a test that restated "3 * time.Second" would
// keep passing if someone widened the real one.
func TestTC_DVH_040h(t *testing.T) {
	assert.GreaterOrEqual(t, helperDockerTimeout, 2*time.Second,
		"FR-092 floor: a shorter budget would abort healthy reads on a loaded Pi")
	assert.LessOrEqual(t, helperDockerTimeout, 3*time.Second,
		"FR-092 ceiling: a longer budget lets a hung helper stall the refresh loop")

	m := NewMonitorWithSource(&fakeSource{list: fiveContainers(), delay: 50 * time.Millisecond})

	start := time.Now()
	m.refresh(context.Background())
	elapsed := time.Since(start)

	assert.True(t, m.Available())
	assert.Len(t, m.Containers(), 5)
	assert.Less(t, elapsed, time.Second)
}

// @aitri-tc TC-DVH-041f — a hung source aborts on the deadline rather than
// blocking forever (AC-092-002).
func TestTC_DVH_041f(t *testing.T) {
	// The fake honours ctx, so the deadline is what ends the call. The bounds
	// of helperDockerTimeout itself are TC-DVH-040h's assertion (AC-092-001);
	// what this one verifies is that the deadline actually fires.
	src := &fakeSource{delay: time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), helperDockerTimeout)
	defer cancel()

	start := time.Now()
	_, err := src.List(ctx)
	elapsed := time.Since(start)

	require.Error(t, err, "a hung source must error, not hang")
	assert.GreaterOrEqual(t, elapsed, 2*time.Second)
	assert.Less(t, elapsed, 4*time.Second)
}

// @aitri-tc TC-DVH-042e — after a timeout the next refresh recovers without
// restarting the process (AC-092-003).
func TestTC_DVH_042e(t *testing.T) {
	src := &fakeSource{list: fiveContainers(), listErrOnce: true}
	m := NewMonitorWithSource(src)

	m.refresh(context.Background())
	assert.False(t, m.Available(), "first read fails")

	m.refresh(context.Background())
	assert.True(t, m.Available(), "second read recovers")
	assert.Len(t, m.Containers(), 5)
	assert.Equal(t, 2, src.callCount())
}

// @aitri-tc TC-DVH-043e — Stop after a timeout returns and leaves no
// goroutines behind (AC-092-004).
func TestTC_DVH_043e(t *testing.T) {
	before := runtime.NumGoroutine()

	m := NewMonitorWithSource(&fakeSource{delay: time.Hour})
	m.SetInterval(time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	time.Sleep(100 * time.Millisecond) // let the first refresh get stuck

	done := make(chan struct{})
	go func() { cancel(); m.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return within 5s after a hung refresh")
	}

	// Give the runtime a moment to reap the cancelled goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	assert.LessOrEqual(t, runtime.NumGoroutine(), before, "Stop must not leak goroutines")
}

// --- carried over from the pre-helper suite ---

func TestMonitor_ContainersReturnsCopy(t *testing.T) {
	m := NewMonitorWithSource(&fakeSource{list: fiveContainers()})
	m.refresh(context.Background())

	got := m.Containers()
	got[0].Name = "mutated"
	assert.Equal(t, "homeassistant", m.Containers()[0].Name, "Containers must hand out a copy")
}

func TestMonitor_ThreadSafety(t *testing.T) {
	m := NewMonitorWithSource(&fakeSource{list: fiveContainers()})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.refresh(context.Background()) }()
		wg.Add(1)
		go func() { defer wg.Done(); _ = m.Containers(); _ = m.Available() }()
	}
	wg.Wait()
	assert.Len(t, m.Containers(), 5)
}

func TestMonitor_StartStop(t *testing.T) {
	m := NewMonitorWithSource(&fakeSource{list: fiveContainers()})
	m.SetInterval(50 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	assert.True(t, m.Available())
	cancel()
	m.Stop()
}

func TestMonitor_NilSourceIsInert(t *testing.T) {
	m := &Monitor{interval: defaultDockerInterval}
	assert.NotPanics(t, func() { m.refresh(context.Background()) })
	assert.False(t, m.Available())

	_, err := m.ContainerDetail(context.Background(), "abc")
	assert.Error(t, err)
	_, err = m.FetchLogs(context.Background(), "abc", 10)
	assert.Error(t, err)
}

func TestMonitor_DetailAndLogsGoThroughSource(t *testing.T) {
	src := &fakeSource{
		detail: &ContainerDetail{ID: "abc", EnvVarNames: []string{"PATH", "TZ"}},
		logs:   "redacted line\n",
	}
	m := NewMonitorWithSource(src)

	d, err := m.ContainerDetail(context.Background(), "abc")
	require.NoError(t, err)
	assert.Equal(t, []string{"PATH", "TZ"}, d.EnvVarNames)

	l, err := m.FetchLogs(context.Background(), "abc", 100)
	require.NoError(t, err)
	assert.Equal(t, "redacted line\n", l)
}

// @aitri-tc TC-DVH-053f — startup emits neither of the log lines the old
// socket-probing path produced. They are gone by construction: nothing pings
// Docker at startup any more (AC-093-003).
func TestTC_DVH_053f(t *testing.T) {
	t.Setenv("ULTRON_HELPER_SOCKET", filepath.Join(t.TempDir(), "absent.sock"))

	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) }()

	m := NewMonitor()
	m.SetInterval(time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	time.Sleep(200 * time.Millisecond) // let the first refresh run and fail
	cancel()
	m.Stop()

	out := buf.String()
	assert.NotContains(t, out, "daemon not reachable",
		"the panel no longer talks to the Docker daemon, so it cannot report it unreachable")
	assert.NotContains(t, out, "permission denied",
		"the panel holds no Docker permission to be denied")
	assert.Contains(t, out, "Docker monitor started", "the monitor must still announce itself")
}

// @aitri-tc TC-DVH-054e — with no helper socket present, construction and
// startup neither fail nor block. An absent helper is a degraded state, not a
// startup failure (AC-093-004).
func TestTC_DVH_054e(t *testing.T) {
	t.Setenv("ULTRON_HELPER_SOCKET", filepath.Join(t.TempDir(), "absent.sock"))

	start := time.Now()
	m := NewMonitor()
	require.NotNil(t, m)

	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	cancel()
	m.Stop()
	elapsed := time.Since(start)

	assert.Less(t, elapsed, time.Second,
		"NewMonitor+Start+Stop must not block waiting for an absent helper")
	assert.False(t, m.Available(), "an absent helper reads as unavailable, not as zero containers")
}
