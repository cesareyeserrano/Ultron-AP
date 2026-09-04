package privileged

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeHelper listens on a Unix socket and records the first request it
// receives, replying {"ok":true}. It stands in for cmd/ultron-helper so the
// test proves the client routes actions over the socket (AC-011-002).
func fakeHelper(t *testing.T) (socketPath string, got chan Request) {
	t.Helper()

	// Unix socket paths are capped (~104 bytes on darwin); t.TempDir() can
	// exceed that, so create a short-lived dir directly under /tmp.
	dir, err := os.MkdirTemp("/tmp", "ultron-helper-test")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	socketPath = filepath.Join(dir, "helper.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	got = make(chan Request, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadBytes('\n')
		if err != nil {
			return
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			return
		}
		got <- req
		resp, _ := json.Marshal(Response{OK: true})
		_, _ = conn.Write(append(resp, '\n'))
	}()
	return socketPath, got
}

// @aitri-tc TC-011a — a host-level service action is routed through the
// privileged helper's Unix socket as a structured request (AC-011-002).
func TestServiceAction_RoutesThroughUnixSocket(t *testing.T) {
	socketPath, got := fakeHelper(t)

	client := NewClient(socketPath, 2*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.ServiceAction(ctx, "restart", "nginx.service"); err != nil {
		t.Fatalf("ServiceAction over unix socket: %v", err)
	}

	select {
	case req := <-got:
		if req.Action != "systemctl" {
			t.Fatalf("helper received action %q, want systemctl", req.Action)
		}
		var p systemctlPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if p.Action != "restart" || p.Name != "nginx.service" {
			t.Fatalf("payload = %+v, want restart nginx.service", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("helper never received the request over the socket")
	}
}
