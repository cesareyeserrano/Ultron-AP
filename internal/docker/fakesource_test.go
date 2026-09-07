package docker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// fakeSource stands in for the privileged helper. It replaces the old Docker
// SDK mock: the monitor no longer speaks to a daemon, so a fake daemon client
// would be testing a collaborator that does not exist any more.
type fakeSource struct {
	mu sync.Mutex

	list    []ContainerInfo
	listErr error
	// listErrOnce makes the FIRST call fail and later ones succeed, which is
	// how a transient helper hiccup is modelled.
	listErrOnce bool
	calls       int

	detail    *ContainerDetail
	detailErr error

	logs    string
	logsErr error

	delay time.Duration
}

func (f *fakeSource) List(ctx context.Context) ([]ContainerInfo, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	delay := f.delay
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErrOnce && n == 1 {
		return nil, errors.New("transient helper failure")
	}
	if f.listErr != nil && !f.listErrOnce {
		return nil, f.listErr
	}
	out := make([]ContainerInfo, len(f.list))
	copy(out, f.list)
	return out, nil
}

func (f *fakeSource) Inspect(_ context.Context, id string) (*ContainerDetail, error) {
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	return f.detail, nil
}

func (f *fakeSource) Logs(_ context.Context, _ string, _ int) (string, error) {
	if f.logsErr != nil {
		return "", f.logsErr
	}
	return f.logs, nil
}

func (f *fakeSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fiveContainers is the shape the Pi actually runs: four up, one exited clean.
func fiveContainers() []ContainerInfo {
	return []ContainerInfo{
		{ID: "aaa1", Name: "homeassistant", Image: "ha:latest", State: "running", Health: HealthRunning},
		{ID: "bbb2", Name: "hermes", Image: "hermes:1", State: "running", Health: HealthRunning},
		{ID: "ccc3", Name: "openclaw", Image: "openclaw:1", State: "running", Health: HealthRunning},
		{ID: "ddd4", Name: "ledger", Image: "ledger:1", State: "exited", Status: "Exited (0) 3 hours ago", Health: HealthStopped},
		{ID: "eee5", Name: "mosquitto", Image: "eclipse-mosquitto:2", State: "running", Health: HealthRunning},
	}
}
