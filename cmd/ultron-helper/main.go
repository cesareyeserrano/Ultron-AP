package main

import (
	"bufio"
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

	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
)

var serviceNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.@\-]+$`)

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
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))

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
		out, err := run(context.Background(), 8*time.Second, "/usr/local/bin/pironman5", "-c")
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
	rgbEnable := "off"
	if cfg.RGBEnable {
		rgbEnable = "on"
	}
	oledEnable := "off"
	if cfg.OLEDEnable {
		oledEnable = "on"
	}
	args := []string{
		"restart",
		"-rc", cfg.RGBColor,
		"-rb", strconv.Itoa(cfg.RGBBrightness),
		"-rs", cfg.RGBStyle,
		"-rp", strconv.Itoa(cfg.RGBSpeed),
		"-re", rgbEnable,
		"-gm", strconv.Itoa(cfg.FanMode),
		"-fl", cfg.FanLED,
		"-oe", oledEnable,
		"-or", strconv.Itoa(cfg.OLEDRotation),
		"-os", strconv.Itoa(cfg.OLEDSleep),
	}
	if _, err := run(context.Background(), 10*time.Second, "/usr/local/bin/pironman5", args...); err != nil {
		return fmt.Errorf("pironman apply failed: %w", err)
	}
	return nil
}

func run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, err
	}
	return out, nil
}
