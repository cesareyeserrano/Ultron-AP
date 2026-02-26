package privileged

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

const DefaultSocketPath = "/run/ultron-helper.sock"

var ErrUnavailable = errors.New("privileged helper unavailable")

type Request struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Response struct {
	OK      bool            `json:"ok"`
	Message string          `json:"message,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type systemctlPayload struct {
	Action string `json:"action"`
	Name   string `json:"name"`
}

type logsPayload struct {
	Source string `json:"source"`
	Lines  int    `json:"lines"`
}

type shutdownPayload struct {
	Mode string `json:"mode"`
}

type Client struct {
	socketPath string
	timeout    time.Duration
}

func NewClient(socketPath string, timeout time.Duration) *Client {
	if strings.TrimSpace(socketPath) == "" {
		socketPath = DefaultSocketPath
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{socketPath: socketPath, timeout: timeout}
}

func NewClientFromEnv() *Client {
	socket := strings.TrimSpace(os.Getenv("ULTRON_HELPER_SOCKET"))
	timeout := 5 * time.Second
	if v := strings.TrimSpace(os.Getenv("ULTRON_HELPER_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			timeout = d
		}
	}
	return NewClient(socket, timeout)
}

func (c *Client) call(ctx context.Context, action string, payload any) (*Response, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: client is nil", ErrUnavailable)
	}

	var rawPayload json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		rawPayload = b
	}

	reqData, err := json.Marshal(Request{Action: action, Payload: rawPayload})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	effectiveTimeout := c.timeout
	if deadline, ok := ctx.Deadline(); ok {
		if until := time.Until(deadline); until > 0 {
			effectiveTimeout = until
		}
	}

	dialer := net.Dialer{Timeout: effectiveTimeout}
	conn, err := dialer.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		if isUnavailableErr(err) {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return nil, fmt.Errorf("connect helper: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(effectiveTimeout))
	if _, err := conn.Write(append(reqData, '\n')); err != nil {
		return nil, fmt.Errorf("write helper request: %w", err)
	}

	dec := json.NewDecoder(conn)
	var resp Response
	if err := dec.Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode helper response: %w", err)
	}
	if !resp.OK {
		msg := strings.TrimSpace(resp.Message)
		if msg == "" {
			msg = "privileged operation failed"
		}
		return nil, errors.New(msg)
	}
	return &resp, nil
}

func isUnavailableErr(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.ENOENT) || errors.Is(opErr.Err, syscall.ECONNREFUSED) || errors.Is(opErr.Err, syscall.EPERM) {
			return true
		}
	}
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPERM)
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.call(ctx, "ping", nil)
	return err
}

func (c *Client) ServiceAction(ctx context.Context, action, name string) error {
	_, err := c.call(ctx, "systemctl", systemctlPayload{Action: action, Name: name})
	return err
}

func (c *Client) SystemLogs(ctx context.Context, source string, lines int) (string, error) {
	resp, err := c.call(ctx, "logs", logsPayload{Source: source, Lines: lines})
	if err != nil {
		return "", err
	}
	var out string
	if len(resp.Payload) > 0 {
		if err := json.Unmarshal(resp.Payload, &out); err != nil {
			return "", fmt.Errorf("decode logs payload: %w", err)
		}
	}
	return out, nil
}

func (c *Client) Shutdown(ctx context.Context, mode string) error {
	_, err := c.call(ctx, "shutdown", shutdownPayload{Mode: mode})
	return err
}
