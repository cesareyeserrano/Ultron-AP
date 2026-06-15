package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/logfilter"
	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
)

var serviceNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.@\-]+$`)

// allowedUIDs holds the set of caller UIDs the helper will accept on its
// Unix socket. It is populated once in main() from environment variables
// and the resolved 'ultron' user account, then read-only for the lifetime
// of the process. An empty set means "accept any UID" (legacy / dev-mode
// behaviour with a loud startup warning) — production callers always
// configure at least one entry via ULTRON_HELPER_ALLOWED_UID/UIDS or by
// having the 'ultron' user resolvable.
//
// @aitri-trace BG-021 BL-013
var allowedUIDs map[uint32]struct{}

// failClosedNoAllowlist reports whether the helper must refuse a connection
// outright because SO_PEERCRED enforcement is compiled in but no caller-UID
// allowlist could be resolved. Serving every UID in that state is the BG-043
// fail-open hole; refusing is the fail-closed posture.
func failClosedNoAllowlist(supported bool, allow map[uint32]struct{}) bool {
	return supported && len(allow) == 0
}

func main() {
	socket := strings.TrimSpace(os.Getenv("ULTRON_HELPER_SOCKET"))
	if socket == "" {
		socket = privileged.DefaultSocketPath
	}

	allowedUIDs = resolveAllowedUIDs()
	if !peerCredSupported {
		log.Printf("warning: SO_PEERCRED not supported on this platform — socket-mode permissions are the only auth")
	} else if len(allowedUIDs) == 0 {
		log.Printf("warning: no caller UID allowlist configured (no ULTRON_HELPER_ALLOWED_UID/UIDS set, 'ultron' user not resolvable) — failing closed: all connections will be refused until an allowlist is configured (BG-043)")
	} else {
		uids := make([]uint32, 0, len(allowedUIDs))
		for u := range allowedUIDs {
			uids = append(uids, u)
		}
		log.Printf("ultron-helper: SO_PEERCRED enforcement on, allowed_uids=%v", uids)
	}

	if err := os.MkdirAll("/run", 0o755); err != nil {
		log.Fatalf("create /run: %v", err)
	}
	_ = os.Remove(socket)

	l, err := net.Listen("unix", socket)
	if err != nil {
		log.Fatalf("listen %s: %v", socket, err)
	}
	defer l.Close()
	if err := os.Chmod(socket, 0o660); err != nil {
		log.Printf("warning: chmod socket: %v", err)
	}
	if group := strings.TrimSpace(os.Getenv("ULTRON_HELPER_SOCKET_GROUP")); group != "" {
		if g, err := user.LookupGroup(group); err == nil {
			if gid, err := strconv.Atoi(g.Gid); err == nil {
				if err := os.Chown(socket, 0, gid); err != nil {
					log.Printf("warning: chown socket group=%s gid=%d failed: %v", group, gid, err)
				}
			}
		} else {
			log.Printf("warning: cannot resolve group %q: %v", group, err)
		}
	} else if gidStr := strings.TrimSpace(os.Getenv("ULTRON_HELPER_SOCKET_GID")); gidStr != "" {
		if gid, err := strconv.Atoi(gidStr); err == nil {
			if err := os.Chown(socket, 0, gid); err != nil {
				log.Printf("warning: chown socket gid=%d failed: %v", gid, err)
			}
		}
	}

	log.Printf("ultron-helper listening on %s", socket)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		_ = l.Close()
		_ = os.Remove(socket)
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("accept error: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	if peerCredSupported {
		// BG-043: fail closed. If peercred enforcement is compiled in but no
		// allowlist could be resolved, refuse the connection rather than
		// serving every UID that can reach the socket.
		if failClosedNoAllowlist(peerCredSupported, allowedUIDs) {
			log.Printf("rejecting helper connection: no caller UID allowlist configured (fail-closed)")
			writeResp(conn, privileged.Response{OK: false, Message: "forbidden"})
			return
		}
		uid, err := getPeerUID(conn)
		if err != nil {
			log.Printf("peercred lookup failed, rejecting connection: %v", err)
			writeResp(conn, privileged.Response{OK: false, Message: "auth failed"})
			return
		}
		if _, ok := allowedUIDs[uid]; !ok {
			log.Printf("rejected helper connection from unauthorised uid=%d", uid)
			writeResp(conn, privileged.Response{OK: false, Message: "forbidden"})
			return
		}
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		writeResp(conn, privileged.Response{OK: false, Message: "invalid request"})
		return
	}
	var req privileged.Request
	if err := json.Unmarshal(line, &req); err != nil {
		writeResp(conn, privileged.Response{OK: false, Message: "invalid json"})
		return
	}

	resp := dispatch(req)
	writeResp(conn, resp)
}

// resolveAllowedUIDs builds the helper's caller-UID allowlist from (in
// precedence order):
//  1. ULTRON_HELPER_ALLOWED_UIDS — comma-separated UIDs (e.g. "1000,1001")
//  2. ULTRON_HELPER_ALLOWED_UID  — single UID (e.g. "1000")
//  3. user.Lookup("ultron")      — derive from the parent project's user
//
// Returns an empty map (meaning "no enforcement") only if none of the above
// resolves; the caller logs a loud warning in that case.
//
// @aitri-trace BG-021 BL-013
func resolveAllowedUIDs() map[uint32]struct{} {
	out := make(map[uint32]struct{}, 2)
	if csv := strings.TrimSpace(os.Getenv("ULTRON_HELPER_ALLOWED_UIDS")); csv != "" {
		for _, part := range strings.Split(csv, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			u, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				log.Printf("warning: ULTRON_HELPER_ALLOWED_UIDS entry %q is not a UID, ignoring: %v", part, err)
				continue
			}
			out[uint32(u)] = struct{}{}
		}
	}
	if v := strings.TrimSpace(os.Getenv("ULTRON_HELPER_ALLOWED_UID")); v != "" {
		if u, err := strconv.ParseUint(v, 10, 32); err == nil {
			out[uint32(u)] = struct{}{}
		} else {
			log.Printf("warning: ULTRON_HELPER_ALLOWED_UID=%q is not a UID: %v", v, err)
		}
	}
	if len(out) == 0 {
		// Fall back to the parent project's user account.
		if u, err := user.Lookup("ultron"); err == nil {
			if uid, err := strconv.ParseUint(u.Uid, 10, 32); err == nil {
				out[uint32(uid)] = struct{}{}
			}
		}
	}
	return out
}

func writeResp(conn net.Conn, resp privileged.Response) {
	enc := json.NewEncoder(conn)
	_ = enc.Encode(resp)
}

func dispatch(req privileged.Request) privileged.Response {
	switch req.Action {
	case "ping":
		return privileged.Response{OK: true, Message: "pong"}
	case "systemctl":
		var p struct {
			Action string `json:"action"`
			Name   string `json:"name"`
		}
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return privileged.Response{OK: false, Message: "invalid systemctl payload"}
		}
		if err := handleSystemctl(p.Action, p.Name); err != nil {
			return privileged.Response{OK: false, Message: err.Error()}
		}
		return privileged.Response{OK: true, Message: "ok"}
	case "logs":
		var p struct {
			Source string `json:"source"`
			Lines  int    `json:"lines"`
		}
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return privileged.Response{OK: false, Message: "invalid logs payload"}
		}
		out, err := handleLogs(p.Source, p.Lines)
		if err != nil {
			return privileged.Response{OK: false, Message: err.Error()}
		}
		b, _ := json.Marshal(out)
		return privileged.Response{OK: true, Payload: b}
	case "shutdown":
		var p struct {
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return privileged.Response{OK: false, Message: "invalid shutdown payload"}
		}
		if err := handleShutdown(p.Mode); err != nil {
			return privileged.Response{OK: false, Message: err.Error()}
		}
		return privileged.Response{OK: true, Message: "ok"}
	default:
		return privileged.Response{OK: false, Message: "unknown action"}
	}
}

func handleSystemctl(action, name string) error {
	switch action {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("invalid service action")
	}
	if !serviceNameRe.MatchString(name) {
		return fmt.Errorf("invalid service name")
	}
	if _, err := run(context.Background(), 20*time.Second, "systemctl", action, name); err != nil {
		return fmt.Errorf("systemctl %s %s failed: %w", action, name, err)
	}
	return nil
}

func handleShutdown(mode string) error {
	var args []string
	switch mode {
	case "restart":
		args = []string{"-r", "now"}
	case "poweroff":
		args = []string{"-h", "now"}
	default:
		return fmt.Errorf("invalid shutdown mode")
	}
	cmd := exec.Command("shutdown", args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("shutdown failed: %w", err)
	}
	return nil
}

func handleLogs(source string, lines int) (string, error) {
	if lines < 1 || lines > 500 {
		lines = 100
	}
	switch source {
	case "ultron-ap":
		out, err := run(context.Background(), 10*time.Second, "journalctl", "-u", "ultron-ap", "-n", strconv.Itoa(lines), "--no-pager")
		return finalizeLog(out, err, logfilter.PolicyJournal)
	case "docker":
		out, err := run(context.Background(), 10*time.Second, "journalctl", "-u", "docker", "-n", strconv.Itoa(lines), "--no-pager")
		return finalizeLog(out, err, logfilter.PolicyJournal)
	case "kernel":
		out, err := run(context.Background(), 10*time.Second, "dmesg", "-T")
		if err != nil {
			return "", err
		}
		parts := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(parts) > lines {
			parts = parts[len(parts)-lines:]
		}
		return finalizeLog([]byte(strings.Join(parts, "\n")), nil, logfilter.PolicyNone)
	case "cpu":
		out, err := run(context.Background(), 10*time.Second, "ps", "-eo", "pid,comm,%cpu,%mem", "--sort=-%cpu")
		return finalizeLog(out, err, logfilter.PolicyNone)
	case "memory":
		out, err := run(context.Background(), 10*time.Second, "ps", "-eo", "pid,comm,rss,%mem", "--sort=-rss")
		return finalizeLog(out, err, logfilter.PolicyNone)
	case "pironman":
		return runFirstServiceLogs(lines, "pironman5-service", "pironman5")
	case "homeassistant":
		return runFirstServiceLogs(lines, "home-assistant@homeassistant", "homeassistant")
	default:
		if strings.HasPrefix(source, "service:") {
			unit := strings.TrimSpace(strings.TrimPrefix(source, "service:"))
			if unit == "" || !serviceNameRe.MatchString(unit) {
				return "", fmt.Errorf("invalid service log source")
			}
			out, err := run(context.Background(), 10*time.Second, "journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager")
			return finalizeLog(out, err, logfilter.PolicyJournal)
		}
		return "", fmt.Errorf("invalid log source")
	}
}

// finalizeLog applies the selected redaction policy and the byte cap to
// privileged command output before it crosses the IPC boundary back to
// the unprivileged web process. err is propagated unchanged so the
// caller still sees underlying systemctl/journalctl/dmesg failures.
func finalizeLog(out []byte, err error, policy logfilter.Policy) (string, error) {
	if err != nil {
		return "", err
	}
	return string(logfilter.Filter(out, policy, 0)), nil
}

func runFirstServiceLogs(lines int, units ...string) (string, error) {
	var lastErr error
	for _, unit := range units {
		out, err := run(context.Background(), 10*time.Second, "journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager")
		if err == nil {
			return finalizeLog(out, nil, logfilter.PolicyJournal)
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no service unit configured")
	}
	return "", lastErr
}

func run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(out.String())
			if msg != "" {
				return nil, fmt.Errorf("%s", msg)
			}
			return nil, err
		}
		return out.Bytes(), nil
	case <-runCtx.Done():
		if cmd.Process != nil {
			if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			}
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		msg := strings.TrimSpace(out.String())
		if msg != "" {
			return nil, fmt.Errorf("timeout: %s", msg)
		}
		return nil, fmt.Errorf("timeout after %v", timeout)
	}
}
