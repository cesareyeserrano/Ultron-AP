package systemd

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Command Runner ---

type mockRunner struct {
	output []byte
	err    error
}

func (m *mockRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return m.output, m.err
}

// --- Sample systemctl output ---

const sampleOutput = `accounts-daemon.service loaded active running Accounts Service
apparmor.service loaded active exited Load AppArmor profiles
avahi-daemon.service loaded active running Avahi mDNS/DNS-SD Stack
bluetooth.service loaded active running Bluetooth service
cron.service loaded active running Regular background program processing daemon
dbus.service loaded active running D-Bus System Message Bus
docker.service loaded active running Docker Application Container Engine
ssh.service loaded active running OpenBSD Secure Shell server
nginx.service loaded failed failed A high performance web server
mysql.service loaded inactive dead MySQL Community Server
postfix.service not-found inactive dead postfix.service

LOAD   = Reflects whether the unit definition was properly loaded.
ACTIVE = The high-level unit activation state, i.e. generalization of SUB.
SUB    = The low-level unit activation state, values depend on unit type.

11 loaded units listed.`

// --- Tests: Output Parsing (AC1) ---

func TestParseListUnits_ValidOutput(t *testing.T) {
	services := parseListUnits(sampleOutput)

	require.Len(t, services, 11)

	// Check first service
	assert.Equal(t, "accounts-daemon", services[0].Name)
	assert.Equal(t, "loaded", services[0].LoadState)
	assert.Equal(t, "active", services[0].ActiveState)
	assert.Equal(t, "running", services[0].SubState)
	assert.Equal(t, "Accounts Service", services[0].Description)
}

func TestParseListUnits_TrimsServiceSuffix(t *testing.T) {
	services := parseListUnits(sampleOutput)
	for _, s := range services {
		assert.False(t, strings.HasSuffix(s.Name, ".service"), "name should not have .service suffix: %s", s.Name)
	}
}

func TestParseListUnits_EmptyOutput(t *testing.T) {
	services := parseListUnits("")
	assert.Empty(t, services)
}

func TestParseListUnits_HeaderOnly(t *testing.T) {
	output := `LOAD   = Reflects whether the unit definition was properly loaded.
ACTIVE = The high-level unit activation state.

0 loaded units listed.`
	services := parseListUnits(output)
	assert.Empty(t, services)
}

func TestParseListUnits_MalformedLine(t *testing.T) {
	output := "incomplete line\nnginx.service loaded active running Nginx"
	services := parseListUnits(output)
	require.Len(t, services, 1)
	assert.Equal(t, "nginx", services[0].Name)
}

func TestParseListUnits_NoDescription(t *testing.T) {
	output := "test.service loaded active running"
	services := parseListUnits(output)
	require.Len(t, services, 1)
	assert.Equal(t, "", services[0].Description)
}

func TestParseListUnits_NonServiceUnits(t *testing.T) {
	output := "test.timer loaded active waiting\nnginx.service loaded active running Nginx"
	services := parseListUnits(output)
	require.Len(t, services, 1)
	assert.Equal(t, "nginx", services[0].Name)
}

// --- Tests: Health Status Mapping (AC2) ---

func TestMapServiceHealth_Active(t *testing.T) {
	assert.Equal(t, ServiceActive, MapServiceHealth("active"))
}

func TestMapServiceHealth_Reloading(t *testing.T) {
	assert.Equal(t, ServiceActive, MapServiceHealth("reloading"))
}

func TestMapServiceHealth_Activating(t *testing.T) {
	assert.Equal(t, ServiceActive, MapServiceHealth("activating"))
}

func TestMapServiceHealth_Failed(t *testing.T) {
	assert.Equal(t, ServiceFailed, MapServiceHealth("failed"))
}

func TestMapServiceHealth_Inactive(t *testing.T) {
	assert.Equal(t, ServiceInactive, MapServiceHealth("inactive"))
}

func TestMapServiceHealth_Deactivating(t *testing.T) {
	assert.Equal(t, ServiceInactive, MapServiceHealth("deactivating"))
}

func TestMapServiceHealth_Unknown(t *testing.T) {
	assert.Equal(t, ServiceInactive, MapServiceHealth("something-else"))
}

// --- Tests: Health indicators in parsed output ---

func TestParseListUnits_HealthMapping(t *testing.T) {
	services := parseListUnits(sampleOutput)

	// Find specific services
	var nginx, mysql, docker ServiceInfo
	for _, s := range services {
		switch s.Name {
		case "nginx":
			nginx = s
		case "mysql":
			mysql = s
		case "docker":
			docker = s
		}
	}

	assert.Equal(t, ServiceFailed, nginx.Health)   // failed → red
	assert.Equal(t, ServiceInactive, mysql.Health) // inactive → grey
	assert.Equal(t, ServiceActive, docker.Health)  // active → green
}

// --- Tests: Failed Filter (AC3) ---

func TestMonitor_FailedFilter(t *testing.T) {
	mock := &mockRunner{output: []byte(sampleOutput)}
	m := newMonitorWithRunner(mock)
	m.refresh(context.Background())

	failed := m.Failed()
	require.Len(t, failed, 1)
	assert.Equal(t, "nginx", failed[0].Name)
	assert.Equal(t, ServiceFailed, failed[0].Health)
}

func TestMonitor_FailedFilter_NoFailures(t *testing.T) {
	output := "docker.service loaded active running Docker\nssh.service loaded active running SSH"
	mock := &mockRunner{output: []byte(output)}
	m := newMonitorWithRunner(mock)
	m.refresh(context.Background())

	failed := m.Failed()
	assert.Empty(t, failed)
}

// --- Tests: Monitor Listing ---

func TestMonitor_ListServices(t *testing.T) {
	mock := &mockRunner{output: []byte(sampleOutput)}
	m := newMonitorWithRunner(mock)
	m.refresh(context.Background())

	services := m.Services()
	assert.Len(t, services, 11)
	assert.True(t, m.Available())
}

func TestMonitor_ServicesReturnsCopy(t *testing.T) {
	mock := &mockRunner{output: []byte(sampleOutput)}
	m := newMonitorWithRunner(mock)
	m.refresh(context.Background())

	s1 := m.Services()
	s2 := m.Services()
	s1[0].Name = "modified"
	assert.NotEqual(t, s1[0].Name, s2[0].Name)
}

// --- Tests: Error Handling ---

func TestMonitor_SystemctlNotAvailable(t *testing.T) {
	m := newMonitorWithRunner(nil)
	assert.False(t, m.Available())
	assert.Empty(t, m.Services())
	assert.Empty(t, m.Failed())
}

func TestMonitor_CommandError_SetsUnavailable(t *testing.T) {
	mock := &mockRunner{err: fmt.Errorf("exec: systemctl: executable file not found in $PATH")}
	m := newMonitorWithRunner(mock)
	m.refresh(context.Background())

	assert.False(t, m.Available())
	assert.Empty(t, m.Services())
}

// --- Tests: Monitor Goroutine Lifecycle (AC4) ---

func TestMonitor_StartStop(t *testing.T) {
	mock := &mockRunner{output: []byte(sampleOutput)}
	m := newMonitorWithRunner(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	services := m.Services()
	assert.NotEmpty(t, services)
	assert.True(t, m.Available())

	m.Stop()
	// After stop, services remain cached
	assert.NotEmpty(t, m.Services())
}

func TestMonitor_ThreadSafety(t *testing.T) {
	mock := &mockRunner{output: []byte(sampleOutput)}
	m := newMonitorWithRunner(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	defer m.Stop()

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			_ = m.Services()
			_ = m.Failed()
			_ = m.Available()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// --- Tests: 100+ services ---

func TestMonitor_ManyServices(t *testing.T) {
	var lines []string
	for i := 0; i < 120; i++ {
		lines = append(lines, fmt.Sprintf("svc%d.service loaded active running Service %d", i, i))
	}
	output := strings.Join(lines, "\n")

	mock := &mockRunner{output: []byte(output)}
	m := newMonitorWithRunner(mock)
	m.refresh(context.Background())

	services := m.Services()
	assert.Len(t, services, 120)
}
