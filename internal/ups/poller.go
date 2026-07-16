// Module:       internal/ups
// Purpose:      Poll loop with backoff + unreachable state, publishing Snapshots (FR-016).
// Dependencies: standard library only.
package ups

import (
	"context"
	"math"
	"strconv"
	"sync"
	"time"
)

// backoffCap bounds the reconnect delay when the UPS stays down.
const backoffCap = 2 * time.Minute

// Poller polls the UPS on an interval and publishes the latest Snapshot. It is
// resilient: a poll error is a valid "unreachable" state, never a panic
// (FR-016 / NFR-016). One Poller owns one Client.
type Poller struct {
	client Client
	cfg    Config

	now  func() time.Time // injectable clock (tests)
	logf func(string, ...any)

	mu       sync.RWMutex
	cur      Snapshot
	lastGood time.Time

	failures   int  // consecutive poll failures (drives backoff)
	loggedDown bool // whether the current outage has been logged (NFR-020)
	reconnects int  // total reconnect attempts (observability/tests)

	store      *Store   // optional history/outage persistence (FR-019/FR-020)
	prevOutage bool     // last observed on-battery state, for transition detection
	alerter    *Alerter // optional power-event alerting (FR-021)

	// Cached history-derived views, recomputed at poll time (every ~10s) rather
	// than on every SSE render tick (every ~5s) so the dashboard never triggers
	// a DB scan (guarded by p.mu). Samples keep their timestamps so the charts
	// area can slice them by the selected window (5m…24h) in memory.
	cachedSamples     []Sample
	cachedEvents      []OutageEvent // newest first — the "cortes" chart tile
	cachedInsights    []Insight
	cachedOnlineSince time.Time // last mains restore (or first sample) — "en red desde hace"
}

// isOutage reports whether a state means the UPS is running on battery (a mains
// outage) — the condition that opens/closes an outage event.
func isOutage(s State) bool { return s == StateOnBattery || s == StateLowBatt }

// NewPoller creates a Poller for the given client and config. Until the first
// successful poll it reports an unreachable snapshot.
func NewPoller(client Client, cfg Config) *Poller {
	return &Poller{
		client: client,
		cfg:    cfg,
		now:    time.Now,
		logf:   logger,
		cur:    Snapshot{State: StateUnreachable, Reachable: false, CutoffV: cfg.BattLowV},
	}
}

// Run polls until ctx is cancelled. It recovers from any panic in the poll path
// so a malformed reply can never crash the host process (NFR-016 AC-036f).
func (p *Poller) Run(ctx context.Context) {
	interval := p.cfg.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	// Poll once immediately so the card is populated without waiting a full
	// interval, then on the ticker.
	p.safePoll(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Backoff: while failing, skip ticks proportional to the failure
			// count so we do not hammer a downed upsd every interval.
			if d := p.currentBackoff(); d > interval {
				time.Sleep(d - interval)
			}
			p.safePoll(ctx)
		}
	}
}

// safePoll runs one poll with panic recovery.
func (p *Poller) safePoll(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			p.logf("ups: recovered from poll panic: %v", r)
			p.markUnreachable()
		}
	}()
	p.pollOnce(ctx)
}

// pollOnce performs a single poll and updates the current snapshot.
func (p *Poller) pollOnce(ctx context.Context) {
	vars, err := p.client.List(ctx)
	if err != nil {
		p.mu.Lock()
		p.failures++
		p.reconnects++
		p.mu.Unlock()
		p.markUnreachable()
		// Once the snapshot has actually flipped to unreachable (past the
		// timeout), let the alerter fire the single "sin comunicación" warning.
		if p.alerter != nil {
			if cur := p.Current(); !cur.Reachable {
				p.alerter.Observe(cur)
			}
		}
		return
	}
	snap := p.buildSnapshot(vars)
	p.mu.Lock()
	p.cur = snap
	p.lastGood = snap.LastGood
	firstRecovery := p.loggedDown
	p.failures = 0
	p.loggedDown = false
	p.mu.Unlock()
	if firstRecovery {
		p.logf("%s ups: reachable again (%s)", p.now().Format(time.RFC3339), snap.State)
	}
	p.persist(snap)
	if p.alerter != nil {
		p.alerter.Observe(snap)
	}
	p.refreshCache()
}

// refreshCache recomputes the history-derived dashboard views (24h battery
// series + insights) once per poll and caches them, so the SSE render path
// never queries the DB.
func (p *Poller) refreshCache() {
	if p.store == nil {
		return
	}
	now := p.now()
	samples, err := p.store.Series(now.Add(-24*time.Hour), now)
	if err != nil {
		p.logf("ups: series: %v", err)
	}
	ins, err := p.store.Insights(now)
	if err != nil {
		p.logf("ups: insights: %v", err)
	}
	since, err := p.store.LastOnlineSince()
	if err != nil {
		p.logf("ups: online-since: %v", err)
	}
	evs, err := p.store.RecentEvents(100)
	if err != nil {
		p.logf("ups: events: %v", err)
	}
	p.mu.Lock()
	p.cachedSamples = samples
	p.cachedEvents = evs
	p.cachedInsights = ins
	p.cachedOnlineSince = since
	p.mu.Unlock()
}

// CachedEvents returns the last-computed outage events, newest first. Cheap:
// no DB access.
func (p *Poller) CachedEvents() []OutageEvent {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cachedEvents
}

// OnlineSinceLabel returns a human duration since mains was last restored (e.g.
// "3 d 4 h"), or "" while on battery, unreachable, or with no history yet.
func (p *Poller) OnlineSinceLabel() string {
	p.mu.RLock()
	cur := p.cur
	since := p.cachedOnlineSince
	p.mu.RUnlock()
	if !cur.Reachable || isOutage(cur.State) || since.IsZero() {
		return ""
	}
	return formatDur(time.Since(since))
}

// CachedSamples returns the last-computed 24h sample series (timestamps kept)
// for the dashboard charts (FR-019). Cheap: no DB access; the slice is replaced
// wholesale on each poll and never mutated, so it is safe to read.
func (p *Poller) CachedSamples() []Sample {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cachedSamples
}

// CachedInsights returns the last-computed UPS insights (FR-022). Cheap: no DB access.
func (p *Poller) CachedInsights() []Insight {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cachedInsights
}

// SetAlerter attaches the power-event alerter (FR-021). Optional.
func (p *Poller) SetAlerter(a *Alerter) { p.alerter = a }

// SetStore attaches the persistence store and reconciles any outage event left
// open by a previous run so a restart mid-outage does not double-count it.
func (p *Poller) SetStore(s *Store) {
	p.store = s
	if s != nil {
		if open, err := s.ReconcileOpenOnBoot(); err == nil {
			p.prevOutage = open
		}
	}
}

// persist writes the sample and opens/closes outage events on OL↔OB transitions.
// No-op when no store is attached.
func (p *Poller) persist(snap Snapshot) {
	if p.store == nil {
		return
	}
	if err := p.store.WriteSample(snap); err != nil {
		p.logf("ups: %v", err)
	}
	outage := isOutage(snap.State)
	switch {
	case outage && !p.prevOutage:
		if err := p.store.OpenEvent(snap.LastGood); err != nil {
			p.logf("ups: %v", err)
		} else {
			p.logf("%s ups: mains outage started (%s)", p.now().Format(time.RFC3339), snap.State)
		}
	case !outage && p.prevOutage:
		if dur, err := p.store.CloseOpenEvent(snap.LastGood); err != nil {
			p.logf("ups: %v", err)
		} else {
			p.logf("%s ups: mains restored after %s", p.now().Format(time.RFC3339), dur)
		}
	}
	p.prevOutage = outage
}

// markUnreachable transitions the snapshot to the unreachable state once the
// failure has persisted past the configured timeout, keeping the last good
// snapshot until then. Logging is bounded to once per outage (NFR-020).
func (p *Poller) markUnreachable() {
	p.mu.Lock()
	defer p.mu.Unlock()
	timeout := p.cfg.UnreachableTimeout
	if timeout <= 0 {
		timeout = defaultUnreachableTimeout
	}
	down := p.lastGood.IsZero() || p.now().Sub(p.lastGood) >= timeout
	if down {
		p.cur = Snapshot{State: StateUnreachable, Reachable: false, CutoffV: p.cfg.BattLowV, LastGood: p.lastGood}
		if !p.loggedDown {
			p.logf("%s ups: unreachable after %d failed poll(s)", p.now().Format(time.RFC3339), p.failures)
			p.loggedDown = true
		}
	}
}

// currentBackoff returns the reconnect delay for the current failure streak,
// capped at backoffCap. Non-decreasing in the failure count (TC-002e).
func (p *Poller) currentBackoff() time.Duration {
	p.mu.RLock()
	f := p.failures
	p.mu.RUnlock()
	if f <= 0 {
		return 0
	}
	base := p.cfg.PollInterval
	if base <= 0 {
		base = defaultPollInterval
	}
	d := time.Duration(math.Min(float64(base)*math.Pow(2, float64(f-1)), float64(backoffCap)))
	return d
}

// PollNow performs a single synchronous poll and returns the resulting
// snapshot. Used by the SSE-independent paths and by tests to drive the mock
// deterministically through states.
func (p *Poller) PollNow(ctx context.Context) Snapshot {
	p.safePoll(ctx)
	return p.Current()
}

// Current returns the latest snapshot (safe for concurrent readers such as SSE).
func (p *Poller) Current() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cur
}

// Purge deletes samples and closed events older than the configured retention
// (FR-024). No-op when no store is attached. Returns rows removed.
func (p *Poller) Purge() (int64, error) {
	if p.store == nil {
		return 0, nil
	}
	ns, err := p.store.PruneSamples(p.cfg.RetentionDays)
	if err != nil {
		return ns, err
	}
	ne, err := p.store.PruneEvents(p.cfg.RetentionDays)
	return ns + ne, err
}

// Reconnects returns the total reconnect attempts (observability/tests).
func (p *Poller) Reconnects() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.reconnects
}

// buildSnapshot converts a raw NUT variable map into a Snapshot, deriving the
// state and the estimated battery %.
func (p *Poller) buildSnapshot(vars map[string]string) Snapshot {
	raw := vars["ups.status"]
	s := Snapshot{
		State:     ParseStatus(raw),
		RawStatus: raw,
		Beeper:    vars["ups.beeper.status"],
		CutoffV:   p.cfg.BattLowV,
		InLowV:    p.cfg.InputVLow,
		InHighV:   p.cfg.InputVHigh,
		LastGood:  p.now(),
		Reachable: true,
	}
	s.LoadPct = parseFloatPtr(vars["ups.load"])
	s.InputV = parseFloatPtr(vars["input.voltage"])
	s.InputHz = parseFloatPtr(vars["input.frequency"])
	s.BatteryV = parseFloatPtr(vars["battery.voltage"])
	if s.BatteryV != nil {
		est := EstimateBatteryPct(*s.BatteryV, p.cfg.BattLowV, p.cfg.BattHighV)
		s.BattPctEst = &est
	}
	s.DelayShut = parseIntPtr(vars["ups.delay.shutdown"])
	s.DelayStart = parseIntPtr(vars["ups.delay.start"])
	return s
}

func parseFloatPtr(s string) *float64 {
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseIntPtr(s string) *int {
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}
