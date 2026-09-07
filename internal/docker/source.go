// Module:       internal/docker (source)
// Purpose:      The monitor's data source. Since the C2 hardening this is the
//
//	privileged helper, never the Docker socket: the web app runs
//	as a user with no access to Docker at all.
//
// Dependencies: internal/privileged (IPC client).
//
// This package must NEVER import internal/dockerapi. The web app imports this
// package, and dockerapi is the only thing that opens the Docker daemon
// socket — importing it here would pull that socket back into the
// unprivileged binary and undo the whole point of the change. TestTC_DVH_050h
// asserts that against the real dependency graph.
//
// The daemon socket path is deliberately not spelled out anywhere in this
// package: TestTC_DVH_052f greps the web app's tree for it, and a comment is
// source too — the same trap that leaked a dead Tailwind class into app.css in
// commit 84dec6a.
//
// @aitri-trace FR-088, FR-092, FR-093, US-088
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
)

// helperDockerTimeout bounds one whole round trip to the helper, including the
// helper's own fan-out to the daemon. The helper works to a tighter 2.5s
// budget internally, so this deadline is the outer guard, not what normally
// cuts a call off (FR-092).
const helperDockerTimeout = 3 * time.Second

// containerSource is where the monitor gets its data. It is an interface so
// tests can inject a fake without a helper, a socket, or a Docker daemon.
//
// It is read-only by construction: there is no Start, Stop or Restart here,
// and there is no action on the helper that would serve one.
type containerSource interface {
	List(ctx context.Context) ([]ContainerInfo, error)
	Inspect(ctx context.Context, id string) (*ContainerDetail, error)
	Logs(ctx context.Context, id string, lines int) (string, error)
}

// helperSource adapts the privileged helper client to containerSource. It is
// the only place that knows the helper's Docker action names.
type helperSource struct {
	client *privileged.Client
}

// newHelperSource builds the production source from the environment-configured
// helper client.
func newHelperSource() *helperSource {
	return &helperSource{client: privileged.NewClientFromEnv()}
}

// List returns every container with stats merged in for the running ones.
func (h *helperSource) List(ctx context.Context) ([]ContainerInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, helperDockerTimeout)
	defer cancel()

	raw, err := h.client.DockerList(ctx)
	if err != nil {
		return nil, err
	}
	var out []ContainerInfo
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode container list: %w", err)
	}
	return out, nil
}

// Inspect returns one container's detail.
func (h *helperSource) Inspect(ctx context.Context, id string) (*ContainerDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, helperDockerTimeout)
	defer cancel()

	raw, err := h.client.DockerInspect(ctx, id)
	if err != nil {
		return nil, err
	}
	var out ContainerDetail
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode container detail: %w", err)
	}
	return &out, nil
}

// Logs returns a container's last n lines, already redacted helper-side.
func (h *helperSource) Logs(ctx context.Context, id string, lines int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, helperDockerTimeout)
	defer cancel()
	return h.client.DockerLogs(ctx, id, lines)
}
