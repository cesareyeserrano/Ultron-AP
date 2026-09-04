package ups

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"go/build"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// fakeUpsd is an in-process NUT server for exercising the real tcpClient.
type fakeUpsd struct {
	addr        string
	mu          sync.Mutex
	commands    []string   // every command line received
	clientAddrs []net.Addr // remote addr of each accepted connection
}

// startFakeUpsd starts a fake upsd on 127.0.0.1 serving the given variables.
func startFakeUpsd(t *testing.T, vars map[string]string) *fakeUpsd {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeUpsd{addr: ln.Addr().String()}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.clientAddrs = append(f.clientAddrs, conn.RemoteAddr())
			f.mu.Unlock()
			go f.handle(conn, vars)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeUpsd) handle(conn net.Conn, vars map[string]string) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		f.mu.Lock()
		f.commands = append(f.commands, line)
		f.mu.Unlock()
		switch {
		case strings.HasPrefix(line, "USERNAME"), strings.HasPrefix(line, "PASSWORD"):
			fmt.Fprint(conn, "OK\n")
		case strings.HasPrefix(line, "LIST VAR"):
			fmt.Fprint(conn, "BEGIN LIST VAR powest\n")
			for k, v := range vars {
				fmt.Fprintf(conn, "VAR powest %s \"%s\"\n", k, v)
			}
			fmt.Fprint(conn, "END LIST VAR powest\n")
		case strings.HasPrefix(line, "LOGOUT"):
			fmt.Fprint(conn, "OK Goodbye\n")
			return
		}
	}
}

func (f *fakeUpsd) cmds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

// TC-UPS-001h (FR-016): client parses LIST VAR into a variable map.
func TestTC_UPS_001h_ParseListVar(t *testing.T) {
	// @aitri-tc TC-UPS-001h
	f := startFakeUpsd(t, map[string]string{
		"ups.status":      "OL",
		"ups.load":        "2",
		"input.voltage":   "122.0",
		"battery.voltage": "27.1",
	})
	c := NewClient(Config{Addr: f.addr, UPSName: "powest"})
	vars, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := map[string]string{"ups.status": "OL", "ups.load": "2", "input.voltage": "122.0", "battery.voltage": "27.1"}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("vars[%q] = %q, want %q", k, vars[k], v)
		}
	}
}

// TC-UPS-003f (FR-016): client exposes no write path and issues only read commands.
func TestTC_UPS_003f_ReadOnly(t *testing.T) {
	// @aitri-tc TC-UPS-003f
	f := startFakeUpsd(t, map[string]string{"ups.status": "OL"})
	c := NewClient(Config{Addr: f.addr, UPSName: "powest", User: "ultron", Pass: "x"})
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	// Every command the client sent must be a read/auth command — never a write.
	for _, cmd := range f.cmds() {
		u := strings.ToUpper(cmd)
		if !(strings.HasPrefix(u, "USERNAME") || strings.HasPrefix(u, "PASSWORD") ||
			strings.HasPrefix(u, "LIST") || strings.HasPrefix(u, "GET")) {
			t.Errorf("client sent non-read command: %q", cmd)
		}
		for _, forbidden := range []string{"SET ", "INSTCMD", "FSD", "LOAD.OFF", "SHUTDOWN"} {
			if strings.Contains(u, forbidden) {
				t.Errorf("client sent forbidden command %q in %q", forbidden, cmd)
			}
		}
	}
	// The Client interface must expose no write method.
	it := reflect.TypeOf((*Client)(nil)).Elem()
	for i := 0; i < it.NumMethod(); i++ {
		name := it.Method(i).Name
		if name != "List" && name != "Close" {
			t.Errorf("Client exposes unexpected method %q (must be read-only)", name)
		}
	}
	// The production package must not import os/exec (never execs upsc).
	assertNoForbiddenImports(t)
}

// TC-UPS-004e (FR-016): parser stays stable over production-scale volume.
func TestTC_UPS_004e_ParserScale(t *testing.T) {
	// @aitri-tc TC-UPS-004e
	const n = 8640 // one day at 10s
	var buf bytes.Buffer
	buf.WriteString("BEGIN LIST VAR powest\n")
	buf.WriteString("VAR powest ups.status \"OL\"\n")
	buf.WriteString("VAR powest battery.voltage \"27.1\"\n")
	buf.WriteString("END LIST VAR powest\n")
	body := buf.Bytes()
	var last map[string]string
	for i := 0; i < n; i++ {
		vars, err := parseListVar(bufio.NewReader(bytes.NewReader(body)), "powest")
		if err != nil {
			t.Fatalf("parse #%d: %v", i, err)
		}
		last = vars
	}
	if last["ups.status"] != "OL" || last["battery.voltage"] != "27.1" {
		t.Fatalf("final map wrong: %v", last)
	}
}

// TC-UPS-037h (NFR-017): the UPS module reaches NUT only via a localhost TCP dial.
func TestTC_UPS_037h_LocalhostOnly(t *testing.T) {
	// @aitri-tc TC-UPS-037h
	f := startFakeUpsd(t, map[string]string{"ups.status": "OL"})
	c := NewClient(Config{Addr: f.addr, UPSName: "powest"})
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.clientAddrs) == 0 {
		t.Fatal("no connection observed")
	}
	for _, a := range f.clientAddrs {
		host, _, _ := net.SplitHostPort(a.String())
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			t.Errorf("connection from non-loopback address %s", a)
		}
	}
}

// TC-UPS-038e (NFR-017): NoNewPrivileges is unaffected — the package pulls in no
// privileged code path (no os/exec, no privileged helper import).
func TestTC_UPS_038e_NoPrivilegedImports(t *testing.T) {
	// @aitri-tc TC-UPS-038e
	assertNoForbiddenImports(t)
}

// TC-UPS-039f (NFR-017): the UPS module makes no privileged-helper call — proven
// architecturally by the absence of any import of the privileged package.
func TestTC_UPS_039f_NoHelperImport(t *testing.T) {
	// @aitri-tc TC-UPS-039f
	assertNoForbiddenImports(t)
}

// assertNoForbiddenImports fails if the production ups package imports os/exec
// or the privileged helper package — the architectural guarantee behind
// NFR-017/NFR-018 (no new privileges, no shutdown path via exec).
func assertNoForbiddenImports(t *testing.T) {
	t.Helper()
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	forbidden := []string{"os/exec", "internal/privileged"}
	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("ups imports forbidden package %q (violates NFR-017/018)", imp)
			}
		}
	}
}

// TC-UPS-054f (NFR-022): the mock is off by default — with ULTRON_UPS_MOCK unset,
// NewClient returns the real TCP client, never the mock.
func TestTC_UPS_054f_MockOffByDefault(t *testing.T) {
	// @aitri-tc TC-UPS-054f
	t.Setenv("ULTRON_UPS_MOCK", "")
	cfg := Load()
	if cfg.Mock != "" {
		t.Fatalf("Mock = %q, want empty by default", cfg.Mock)
	}
	c := NewClient(cfg)
	if _, ok := c.(*tcpClient); !ok {
		t.Fatalf("NewClient returned %T, want *tcpClient when mock is unset", c)
	}
	if _, ok := c.(*mockClient); ok {
		t.Fatal("NewClient returned a mockClient with no mock configured")
	}
}
