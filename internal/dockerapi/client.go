// Module:       internal/dockerapi
// Purpose:      Minimal read-only HTTP client for the Docker Engine API over
//
//	its Unix socket. Replaces github.com/docker/docker, which
//	dragged moby/* and opencontainers/* in for what amounts to
//	four GET requests.
//
// Dependencies: standard library only (net, net/http, encoding/json).
//
// SECURITY: this package is HELPER-ONLY. It is the single place in the tree
// that opens /var/run/docker.sock, and no package reachable from
// cmd/ultron-ap may import it — TestTC_DVH_050h asserts that against the real
// dependency graph. Every request it issues is a GET; there is deliberately no
// method parameter anywhere in the API, so adding a write would require
// editing this file as well as the caller.
//
// @aitri-trace FR-089, FR-090, US-089, US-090
package dockerapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// DefaultSocketPath is the Docker daemon's Unix socket on the Pi.
const DefaultSocketPath = "/var/run/docker.sock"

// defaultTimeout bounds a single call to the daemon. It is deliberately below
// the 3s budget the web app allows for a whole helper round trip (FR-092), so
// the helper's fan-out still fits inside the caller's deadline.
const defaultTimeout = 2 * time.Second

// maxResponseBytes caps any single daemon response. A compromised or wedged
// daemon must not be able to exhaust the memory of a process running as root.
const maxResponseBytes = 32 << 20 // 32 MiB

// ErrInvalidID is returned when a container id fails validation. It is
// returned BEFORE any request is built, so a rejected id never reaches the
// socket (AC-089-003).
var ErrInvalidID = errors.New("invalid container id")

// ErrNotFound is returned when the daemon answers 404 for a container.
var ErrNotFound = errors.New("container not found")

// containerIDRe constrains a container id to the shape Docker actually issues
// (a hex digest) plus the punctuation names may carry. The first character
// must be alphanumeric, mirroring serviceNameRe in the helper. The class
// excludes '/', '%' and a leading '.', so neither "../../info" nor
// "%2e%2e%2f" can escape its path segment.
//
// @aitri-trace NFR-092, AC-089-003
var containerIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// Client talks to the Docker Engine API over a Unix socket.
type Client struct {
	http *http.Client
}

// New builds a Client bound to socketPath.
//
// Params:
//   - socketPath: the daemon socket; DefaultSocketPath when empty.
//   - timeout:    per-request ceiling; defaultTimeout when <= 0.
//
// Returns a Client that is safe for concurrent use.
func New(socketPath string, timeout time.Duration) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	// The host in the URL is a placeholder: DialContext ignores the address
	// entirely and always connects to the socket.
	dialer := &net.Dialer{Timeout: timeout}
	return &Client{
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// ValidateID reports whether id is an acceptable container identifier.
// Exported so the helper's dispatch can reject early with the same rule.
func ValidateID(id string) error {
	if !containerIDRe.MatchString(id) {
		return ErrInvalidID
	}
	return nil
}

// get issues a GET against the daemon and returns the (capped) body.
// http.MethodGet is a literal here and nowhere is a method accepted as an
// argument — that is the mechanical half of the read-only guarantee.
//
// Params:
//   - ctx:  cancellation/deadline for the call.
//   - path: absolute API path, already escaped by the caller.
//
// Returns the response body, ErrNotFound on 404, or an error.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build docker request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker unavailable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read docker response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("docker read failed: status %d", resp.StatusCode)
	}
	return body, nil
}

// Containers lists every container, running or not.
// Mirrors GET /containers/json?all=1.
func (c *Client) Containers(ctx context.Context) ([]Container, error) {
	body, err := c.get(ctx, "/containers/json?all=1")
	if err != nil {
		return nil, err
	}
	var out []Container
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode container list: %w", err)
	}
	return out, nil
}

// Stats returns a single non-streaming resource sample for one container.
// Mirrors GET /containers/{id}/stats?stream=false.
func (c *Client) Stats(ctx context.Context, id string) (*StatsSnapshot, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, "/containers/"+id+"/stats?stream=false&one-shot=true")
	if err != nil {
		return nil, err
	}
	var out StatsSnapshot
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}
	return &out, nil
}

// Inspect returns the subset of the container's inspect document the panel
// uses. Mirrors GET /containers/{id}/json.
func (c *Client) Inspect(ctx context.Context, id string) (*Inspect, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, "/containers/"+id+"/json")
	if err != nil {
		return nil, err
	}
	var out Inspect
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode inspect: %w", err)
	}
	return &out, nil
}

// Logs returns the last n lines of a container's combined output, already
// de-multiplexed from Docker's framed stream.
// Mirrors GET /containers/{id}/logs?stdout=1&stderr=1&tail=n.
func (c *Client) Logs(ctx context.Context, id string, lines int) ([]byte, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	if lines < 1 || lines > 500 {
		lines = 100
	}
	body, err := c.get(ctx, "/containers/"+id+"/logs?stdout=1&stderr=1&tail="+strconv.Itoa(lines))
	if err != nil {
		return nil, err
	}
	return Demux(body), nil
}
