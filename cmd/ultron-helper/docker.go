// Module:       cmd/ultron-helper (docker)
// Purpose:      The helper's read-only Docker surface. This is the only place
//
//	in the product where the Docker daemon is reached at all, and
//	the only place where dockerapi's raw types are mapped onto the
//	docker.ContainerInfo / ContainerDetail the panel renders.
//
// Dependencies: internal/dockerapi (transport), internal/docker (wire models),
//
//	internal/logfilter (redaction).
//
// The mapping deliberately lives here rather than in internal/docker: the web
// app imports internal/docker, so a mapping there would drag internal/dockerapi
// into the unprivileged binary and defeat FR-093. The helper is the single
// place both packages may meet.
//
// @aitri-trace FR-089, FR-090, FR-095, NFR-092, US-089, US-090, US-095
package main

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/dockerapi"
	"github.com/cesareyeserrano/ultron-ap/internal/logfilter"
)

// dockerStatsConcurrency caps how many stats calls run against the daemon at
// once. It matches the bound the panel used before the move, so a host with
// many running containers cannot stampede the daemon on every tick.
const dockerStatsConcurrency = 16

// dockerOpTimeout bounds one whole helper-side Docker operation, including the
// parallel stats fan-out. It sits below the 3s the web app allows for the
// round trip (FR-092), so the caller's deadline is not what cuts us off.
const dockerOpTimeout = 2500 * time.Millisecond

// dockerClient is the transport used by the handlers below. It is a package
// variable so tests can point it at a fake daemon.
var dockerClient = dockerapi.New(dockerapi.DefaultSocketPath, 2*time.Second)

// handleDockerList returns every container with per-container stats already
// merged in for the running ones.
//
// The stats fan-out happens HERE rather than in the panel on purpose: the
// panel needs both the list and the stats on every tick, so separate actions
// would cost 1+N IPC round trips per refresh instead of one (ADR-002).
//
// Returns the panel's ContainerInfo slice, or an error.
//
// @aitri-trace FR-088, FR-089, AC-088-001, AC-088-002
func handleDockerList(ctx context.Context) ([]docker.ContainerInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, dockerOpTimeout)
	defer cancel()

	raw, err := dockerClient.Containers(ctx)
	if err != nil {
		return nil, err
	}

	infos := make([]docker.ContainerInfo, len(raw))
	for i, c := range raw {
		infos[i] = toContainerInfo(c)
	}

	// Each goroutine writes a distinct index, so no mutex is needed for the
	// slice itself; the semaphore is what bounds daemon concurrency.
	sem := make(chan struct{}, dockerStatsConcurrency)
	var wg sync.WaitGroup
	for i, c := range raw {
		if c.State != "running" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, id string) {
			defer wg.Done()
			defer func() { <-sem }()
			st, err := dockerClient.Stats(ctx, id)
			if err != nil {
				// A single container's stats failing must not fail the list —
				// the row still renders, just without usage figures.
				return
			}
			infos[idx].CPUPercent = st.CPUPercent()
			infos[idx].MemUsage = st.MemoryStats.Usage
			infos[idx].MemLimit = st.MemoryStats.Limit
			infos[idx].MemPercent = st.MemPercent()
		}(i, c.ID)
	}
	wg.Wait()
	return infos, nil
}

// handleDockerInspect returns the detail view's data for one container.
// Environment variable VALUES are dropped here, inside the privileged
// process, so they never cross the IPC boundary at all.
//
// @aitri-trace FR-090, AC-090-001, AC-090-004
func handleDockerInspect(ctx context.Context, id string) (*docker.ContainerDetail, error) {
	if err := dockerapi.ValidateID(id); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, dockerOpTimeout)
	defer cancel()

	insp, err := dockerClient.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}

	detail := &docker.ContainerDetail{ID: id}

	// Map iteration order is random in Go; sort the port keys so the rendered
	// detail does not reshuffle between refreshes.
	keys := make([]string, 0, len(insp.NetworkSettings.Ports))
	for k := range insp.NetworkSettings.Ports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, b := range insp.NetworkSettings.Ports[k] {
			detail.Ports = append(detail.Ports, docker.PortMapping{
				HostPort:      b.HostPort,
				ContainerPort: k,
				Protocol:      dockerapi.ProtoOf(k),
			})
		}
	}

	for _, m := range insp.Mounts {
		detail.Volumes = append(detail.Volumes, docker.VolumeMount{
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
		})
	}

	detail.EnvVarNames = dockerapi.EnvNames(insp.Config.Env)
	return detail, nil
}

// handleDockerLogs returns a container's last n lines, redacted and capped by
// the SAME logfilter policy journalctl output goes through. Reusing
// finalizeLog is deliberate: a second, parallel redaction path is how the two
// drift and one ends up laxer than the other (NFR-097).
//
// @aitri-trace FR-095, AC-095-002, AC-095-003, AC-095-004
func handleDockerLogs(ctx context.Context, id string, lines int) (string, error) {
	if err := dockerapi.ValidateID(id); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, dockerOpTimeout)
	defer cancel()

	out, err := dockerClient.Logs(ctx, id, lines)
	if err != nil {
		return "", err
	}
	return finalizeLog(out, nil, logfilter.PolicyJournal)
}

// toContainerInfo maps one raw daemon entry onto the panel's model, reusing
// the panel's own health classification so the two cannot drift.
func toContainerInfo(c dockerapi.Container) docker.ContainerInfo {
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	} else {
		name = c.ID
		if len(name) > 12 {
			name = name[:12]
		}
	}

	exitCode := 0
	if c.State == "exited" || c.State == "dead" {
		exitCode = docker.ParseExitCode(c.Status)
	}

	return docker.ContainerInfo{
		ID:        c.ID,
		Name:      name,
		Image:     c.Image,
		State:     c.State,
		Status:    c.Status,
		Health:    docker.MapHealthStatus(c.State, exitCode),
		CreatedAt: time.Unix(c.Created, 0),
	}
}
