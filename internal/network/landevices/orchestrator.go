// Package landevices is the public surface of the lan-devices feature: a
// single Orchestrator goroutine that schedules ICMP sweeps, pairs responders
// with their MACs from the kernel ARP cache, resolves vendors against the
// embedded OUI table, and upserts results into SQLite via the store.
//
// FR-035 state-machine logic lives in the store (single transactional write
// path); FR-038 self-throttle logic lives here in the scheduler.
//
// @aitri-trace FR-031 FR-038 US-031 US-038 TC-LD-009h TC-LD-009f TC-LD-009e
package landevices

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices/arp"
	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices/oui"
	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices/store"
	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices/subnet"
	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices/sweep"
)

// Defaults map to the FRs:
//
//	BaseCadence    — FR-031 default 5 min
//	MinCadence     — FR-031/FR-038 floor (1 min)
//	MaxCadence     — FR-038 cap (30 min)
//	BudgetWallClock — FR-038 trigger (3 s)
//	ThrottleStreak  — consecutive over-budget cycles before throttling (2)
//	RestoreWindow   — in-budget time before restoring (30 min)
//	MissThreshold   — FR-035 default (3 sweeps)
const (
	BaseCadence     = 5 * time.Minute
	MinCadence      = 1 * time.Minute
	MaxCadence      = 30 * time.Minute
	BudgetWallClock = 3 * time.Second
	ThrottleStreak  = 2
	RestoreWindow   = 30 * time.Minute
	MissThreshold   = 3
)

// Status is the orchestrator's externally-visible state. Returned by Status().
type Status struct {
	Subnet              string        `json:"subnet"`
	Interface           string        `json:"interface"`
	SubnetStatus        string        `json:"subnet_status"`
	LastSweepAt         time.Time     `json:"last_sweep_at"`
	LastSweepDuration   time.Duration `json:"last_sweep_duration_ms"`
	LastSweepResponders int           `json:"last_sweep_responders"`
	OverrunCount        int           `json:"overrun_count"`
	SelfThrottled       bool          `json:"self_throttled"`
	CurrentCadence      time.Duration `json:"current_cadence_ms"`
	DeviceCount         int           `json:"device_count"`
	Disabled            bool          `json:"disabled"`
}

// Config wires every seam the orchestrator needs. In production main.go
// builds this with real paths; tests inject fakes.
type Config struct {
	Store         *store.Store
	Transport     sweep.Transport
	RoutePath     string                 // /proc/net/route
	ArpPath       string                 // /proc/net/arp
	IfaceResolver subnet.IfaceResolver   // production: subnet.DefaultIfaceResolver
	Cadence       time.Duration          // default BaseCadence
	Workers       int                    // default 32
	Now           func() time.Time       // default time.Now
	Logger        func(format string, args ...interface{}) // optional
}

// Orchestrator owns the sweep loop. Use Start/Stop for lifecycle.
type Orchestrator struct {
	cfg Config

	mu     sync.Mutex
	status Status

	// scheduler state
	baseCadence       time.Duration
	currentCadence    time.Duration
	overBudgetStreak  int
	throttledSince    time.Time
	inBudgetSince     time.Time

	// concurrency
	cycleInFlight bool
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// New builds an orchestrator from cfg. It does not start the loop.
func New(cfg Config) *Orchestrator {
	if cfg.Cadence <= 0 {
		cfg.Cadence = BaseCadence
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 32
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.IfaceResolver == nil {
		cfg.IfaceResolver = subnet.DefaultIfaceResolver
	}
	if cfg.Transport == nil {
		// BG-018: production uses an unprivileged SOCK_DGRAM ICMP socket;
		// tests inject sweep.fakeTransport via Config.
		cfg.Transport = sweep.DefaultTransport()
	}
	if cfg.Logger == nil {
		cfg.Logger = func(string, ...interface{}) {}
	}
	return &Orchestrator{
		cfg:            cfg,
		baseCadence:    cfg.Cadence,
		currentCadence: cfg.Cadence,
	}
}

// Status returns a copy of the current orchestrator state.
func (o *Orchestrator) Status() Status {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.status
	st.CurrentCadence = o.currentCadence
	if n, err := o.cfg.Store.Count(); err == nil {
		st.DeviceCount = n
	}
	return st
}

// Start launches the orchestrator goroutine. Returns immediately.
func (o *Orchestrator) Start(ctx context.Context) {
	cctx, cancel := context.WithCancel(ctx)
	o.cancel = cancel
	o.wg.Add(1)
	go o.run(cctx)
}

// Stop signals the orchestrator to exit and waits for the goroutine.
func (o *Orchestrator) Stop() {
	if o.cancel != nil {
		o.cancel()
	}
	o.wg.Wait()
}

func (o *Orchestrator) run(ctx context.Context) {
	defer o.wg.Done()
	// First cycle fires immediately so the device list populates without
	// waiting a full cadence after boot.
	o.tryCycle(ctx)
	for {
		o.mu.Lock()
		next := o.currentCadence
		o.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(next):
			o.tryCycle(ctx)
		}
	}
}

// tryCycle is the cadence-tick entry point. Skips overlapping cycles per
// FR-031 AC-003. Adjusts cadence afterwards per FR-038.
func (o *Orchestrator) tryCycle(ctx context.Context) {
	o.mu.Lock()
	if o.cycleInFlight {
		o.status.OverrunCount++
		o.mu.Unlock()
		o.cfg.Logger("lan-devices: sweep overrun, skipping cycle (count=%d)", o.status.OverrunCount)
		return
	}
	o.cycleInFlight = true
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.cycleInFlight = false
		o.mu.Unlock()
	}()

	dur, err := o.runOneCycle(ctx)
	if err != nil {
		o.cfg.Logger("lan-devices: cycle error: %v", err)
		return
	}
	o.adjustCadence(dur)
}

// runOneCycle does the actual subnet → sweep → arp → upsert work and
// returns the wall-clock cycle duration (used by adjustCadence for FR-038).
func (o *Orchestrator) runOneCycle(ctx context.Context) (time.Duration, error) {
	start := o.cfg.Now()

	sub, err := subnet.Detect(o.cfg.RoutePath, o.cfg.IfaceResolver)
	if err != nil && sub.Status == "" {
		return 0, err
	}
	o.mu.Lock()
	o.status.Subnet = sub.CIDR
	o.status.Interface = sub.Iface
	o.status.SubnetStatus = string(sub.Status)
	o.mu.Unlock()

	if sub.CIDR == "" {
		// No subnet to sweep — record the cycle as zero-cost and return.
		return time.Since(start), nil
	}

	res, err := sweep.Sweep(ctx, sweep.Config{
		CIDR:      sub.CIDR,
		Transport: o.cfg.Transport,
		Workers:   o.cfg.Workers,
		Timeout:   1 * time.Second,
		HostFilter: func(ip string) bool {
			return ip != sub.HostIP // don't probe ourselves
		},
	})
	if err != nil {
		return time.Since(start), err
	}

	cache, cacheErr := arp.ReadCache(o.cfg.ArpPath)
	if cacheErr != nil && !errors.Is(cacheErr, arp.ErrARPUnavailable) {
		o.cfg.Logger("lan-devices: arp read error: %v", cacheErr)
	}
	pairs := arp.PairResponders(res.Responders, cache, cacheErr)

	observations := make([]store.Observation, 0, len(pairs))
	for _, p := range pairs {
		if p.MAC == "" {
			continue // can't persist without a MAC (it's the PK)
		}
		observations = append(observations, store.Observation{
			MAC:    p.MAC,
			IP:     p.IP,
			Vendor: oui.Vendor(p.MAC),
		})
	}
	if err := o.cfg.Store.ApplySweep(o.cfg.Now(), observations); err != nil {
		return time.Since(start), err
	}

	finishedAt := o.cfg.Now()
	dur := finishedAt.Sub(start)

	o.mu.Lock()
	o.status.LastSweepAt = finishedAt
	o.status.LastSweepDuration = dur
	o.status.LastSweepResponders = len(res.Responders)
	o.mu.Unlock()
	return dur, nil
}

// adjustCadence implements FR-038: 2 consecutive over-budget cycles → 2×
// cadence (capped at MaxCadence); RestoreWindow of in-budget cycles → restore.
func (o *Orchestrator) adjustCadence(dur time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()

	overBudget := dur > BudgetWallClock
	now := o.cfg.Now()

	if overBudget {
		o.overBudgetStreak++
		o.inBudgetSince = time.Time{}
		if o.overBudgetStreak >= ThrottleStreak {
			doubled := o.currentCadence * 2
			if doubled > MaxCadence {
				doubled = MaxCadence
			}
			if doubled < MinCadence {
				doubled = MinCadence
			}
			if doubled != o.currentCadence {
				o.currentCadence = doubled
			}
			if !o.status.SelfThrottled {
				o.status.SelfThrottled = true
				o.throttledSince = now
			}
		}
		return
	}

	// In-budget cycle.
	o.overBudgetStreak = 0
	if !o.status.SelfThrottled {
		// Not throttled — nothing to track.
		return
	}
	if o.inBudgetSince.IsZero() {
		o.inBudgetSince = now
		return
	}
	if now.Sub(o.inBudgetSince) >= RestoreWindow {
		o.currentCadence = o.baseCadence
		o.status.SelfThrottled = false
		o.inBudgetSince = time.Time{}
	}
}
