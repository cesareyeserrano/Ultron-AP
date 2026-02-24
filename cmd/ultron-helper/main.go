package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
)

var serviceNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.@\-]+$`)

const pironmanAPIBaseURL = "http://127.0.0.1:34001/api/v1.0/"

type applyJob struct {
	cfg  privileged.PironmanConfig
	done chan error
}

var (
	applyQueue     chan applyJob
	applyQueueOnce sync.Once
	pmHTTPClient   = &http.Client{Timeout: 6 * time.Second}
)

type pmAPIResponse struct {
	Status bool            `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  string          `json:"error"`
}

func main() {
	socket := strings.TrimSpace(os.Getenv("ULTRON_HELPER_SOCKET"))
	if socket == "" {
		socket = privileged.DefaultSocketPath
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
	startApplyWorker()

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

func startApplyWorker() {
	applyQueueOnce.Do(func() {
		applyQueue = make(chan applyJob, 64)
		go func() {
			for {
				first, ok := <-applyQueue
				if !ok {
					return
				}

				latest := first
				waiters := []chan error{first.done}
				for {
					select {
					case next := <-applyQueue:
						latest = next
						waiters = append(waiters, next.done)
					default:
						goto EXECUTE
					}
				}

			EXECUTE:
				err := handlePironmanApplyNow(latest.cfg)
				for _, done := range waiters {
					done <- err
					close(done)
				}
			}
		}()
	})
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

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
	case "pironman.read":
		out, err := readPironmanConfig(context.Background())
		if err != nil {
			return privileged.Response{OK: false, Message: fmt.Sprintf("pironman read failed: %v", err)}
		}
		b, _ := json.Marshal(strings.TrimSpace(string(out)))
		return privileged.Response{OK: true, Payload: b}
	case "pironman.apply":
		var cfg privileged.PironmanConfig
		if err := json.Unmarshal(req.Payload, &cfg); err != nil {
			return privileged.Response{OK: false, Message: "invalid pironman payload"}
		}
		if err := handlePironmanApply(cfg); err != nil {
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
		return string(out), err
	case "docker":
		out, err := run(context.Background(), 10*time.Second, "journalctl", "-u", "docker", "-n", strconv.Itoa(lines), "--no-pager")
		return string(out), err
	case "kernel":
		out, err := run(context.Background(), 10*time.Second, "dmesg", "-T")
		if err != nil {
			return "", err
		}
		parts := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(parts) > lines {
			parts = parts[len(parts)-lines:]
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", fmt.Errorf("invalid log source")
	}
}

func handlePironmanApply(cfg privileged.PironmanConfig) error {
	startApplyWorker()
	done := make(chan error, 1)
	job := applyJob{cfg: cfg, done: done}
	select {
	case applyQueue <- job:
	case <-time.After(1 * time.Second):
		return fmt.Errorf("pironman apply queue overloaded")
	}
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		return fmt.Errorf("pironman apply timed out waiting for worker")
	}
}

func handlePironmanApplyNow(cfg privileged.PironmanConfig) error {
	start := time.Now()
	fail := func(err error) error {
		log.Printf("pironman apply failed after %v: %v", time.Since(start).Round(time.Millisecond), err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	color := strings.TrimSpace(cfg.RGBColor)
	if color == "" {
		return fail(fmt.Errorf("invalid rgb color"))
	}
	if !strings.HasPrefix(color, "#") {
		color = "#" + color
	}

	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-rgb-color", map[string]any{"color": color}); err != nil {
		return fail(fmt.Errorf("set-rgb-color: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-rgb-brightness", map[string]any{"brightness": cfg.RGBBrightness}); err != nil {
		return fail(fmt.Errorf("set-rgb-brightness: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-rgb-style", map[string]any{"style": cfg.RGBStyle}); err != nil {
		return fail(fmt.Errorf("set-rgb-style: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-rgb-speed", map[string]any{"speed": cfg.RGBSpeed}); err != nil {
		return fail(fmt.Errorf("set-rgb-speed: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-rgb-enable", map[string]any{"enable": cfg.RGBEnable}); err != nil {
		return fail(fmt.Errorf("set-rgb-enable: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-fan-mode", map[string]any{"fan_mode": cfg.FanMode}); err != nil {
		return fail(fmt.Errorf("set-fan-mode: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-fan-led", map[string]any{"led": cfg.FanLED}); err != nil {
		return fail(fmt.Errorf("set-fan-led: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-oled-enable", map[string]any{"enable": cfg.OLEDEnable}); err != nil {
		return fail(fmt.Errorf("set-oled-enable: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-oled-rotation", map[string]any{"rotation": cfg.OLEDRotation}); err != nil {
		return fail(fmt.Errorf("set-oled-rotation: %w", err))
	}
	if _, err := pironmanAPICall(ctx, http.MethodPost, "set-oled-sleep-timeout", map[string]any{"timeout": cfg.OLEDSleep}); err != nil {
		return fail(fmt.Errorf("set-oled-sleep-timeout: %w", err))
	}
	log.Printf("pironman apply completed in %v", time.Since(start).Round(time.Millisecond))
	return nil
}

func readPironmanConfig(ctx context.Context) ([]byte, error) {
	apiCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	data, err := pironmanAPICall(apiCtx, http.MethodGet, "get-config", nil)
	if err == nil {
		return []byte(strings.TrimSpace(string(data))), nil
	}
	// Fallback only if dashboard API is unavailable.
	out, cliErr := run(ctx, 8*time.Second, "/usr/local/bin/pironman5", "-c")
	if cliErr != nil {
		return nil, fmt.Errorf("api failed: %v; cli failed: %v", err, cliErr)
	}
	return out, nil
}

func pironmanAPICall(ctx context.Context, method, endpoint string, payload any) (json.RawMessage, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, pironmanAPIBaseURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := pmHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBody))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("http %d: %s", res.StatusCode, msg)
	}

	var parsed pmAPIResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if !parsed.Status {
		msg := strings.TrimSpace(parsed.Error)
		if msg == "" {
			msg = "status=false"
		}
		return nil, errors.New(msg)
	}
	return parsed.Data, nil
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
