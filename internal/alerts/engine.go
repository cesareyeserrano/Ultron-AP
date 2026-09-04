package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

// AlertCallback is called when a new alert is created.
type AlertCallback func(alert *database.Alert)

// RichAlertCallback is the FR-016/019/024-aware variant. It carries the
// matching AlertConfig and the first_fired_at timestamp so the dispatcher
// can build a notify.Event without round-tripping to the DB. When set, it
// is called INSTEAD of the legacy AlertCallback.
//
// firstFiredAt is the wall clock at which this rule first fired in the
// current breach (set on the very first fire after a cooldown gap; reset
// on cooldown prune). For Docker and Systemd transition alerts the engine
// uses the moment of state transition.
//
// @aitri-trace FR-016 FR-019 FR-024
type RichAlertCallback func(alert *database.Alert, rule *database.AlertConfig, firstFiredAt time.Time)

// ResolveCallback is invoked when a previously-firing rule clears: a
// metric drops back below threshold, a container transitions from exited
// to running, a systemd unit transitions from failed to active. The
// callback receives the rule context plus the fire-window timestamps so
// the dispatcher can render a "✓ RESOLVED" message with the FR-019 active
// duration. There is NO database alert row for resolves — they are
// notification-layer-only events (no schema change, per Phase 1
// no_go_zone).
//
// rule may be nil for transition-based resolves (docker/systemd) where
// the engine has only the source name; in that case sourceID carries the
// "docker:<name>" / "systemd:<name>" identifier so the dispatcher can
// look up the matching ServiceInfo / ContainerInfo for the surface block.
//
// @aitri-trace FR-018 BL-023
type ResolveCallback func(rule *database.AlertConfig, sourceID string, severity string, firstFiredAt, resolvedAt time.Time)

// dockerContainerSource is the slice of *docker.Monitor the engine consumes.
// It exists so transition-detection tests can inject container states without
// a live Docker daemon (AC-004-002).
type dockerContainerSource interface {
	Available() bool
	Containers() []docker.ContainerInfo
}

// systemdServiceSource mirrors dockerContainerSource for *systemd.Monitor
// (AC-004-003).
type systemdServiceSource interface {
	Available() bool
	Services() []systemd.ServiceInfo
}

// Engine evaluates alert rules against current system state.
type Engine struct {
	db        *database.DB
	collector *metrics.Collector
	docker    dockerContainerSource
	systemd   systemdServiceSource
	interval  time.Duration
	onAlert   AlertCallback
	onRich    RichAlertCallback
	onResolve ResolveCallback

	mu           sync.Mutex
	cooldowns    map[string]time.Time // ruleKey -> last triggered
	firingFirst  map[string]time.Time // ruleKey -> first_fired_at (FR-019)
	prevDocker   map[string]string    // containerName -> state
	prevSystemd  map[string]string    // serviceName -> activeState
	sustained    map[int64]*sustainedWindow
	processedNet map[string]struct{}
	recentAlerts []database.Alert
	recentMu     sync.RWMutex

	// Per-source cooldown overrides for state-transition rules (Docker /
	// Systemd). Zero means "use default 15 min". Atomic so the runtime
	// settings page can mutate them without locking the eval loop.
	// (BL-001 / BG-023: fixes drift from FR-004 AC-002.)
	dockerCooldownNs  atomic.Int64
	systemdCooldownNs atomic.Int64

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// defaultTransitionCooldown is the historical literal — applied when the
// runtime setter has not yet been called and the env config is also empty.
const defaultTransitionCooldown = 15 * time.Minute

type sustainedSample struct {
	at        time.Time
	breaching bool
}

type sustainedWindow struct {
	duration time.Duration
	interval time.Duration
	samples  []sustainedSample
}

// add records a per-tick breach observation and reports whether the breach has
// been sustained for at least w.duration. Confirmation is span-based (current
// sample minus run start), so it tolerates clock jitter and non-interval-aligned
// sampling; the window keeps only its boundary samples to stay O(1) in memory.
//
// @aitri-trace FR-016 FR-017 US-016 US-017 AC-016-001 AC-016-002 AC-016-003 AC-016-004 AC-017-001 AC-017-002
// @aitri-trace NFR-005 NFR-006 NFR-007 NFR-008 TC-SAW-016h TC-SAW-016f TC-SAW-017h TC-SAW-017f
func (w *sustainedWindow) add(ruleID int64, at time.Time, breaching bool) bool {
	if w.duration <= 0 {
		return breaching
	}
	if len(w.samples) > 0 {
		prev := w.samples[len(w.samples)-1].at
		if at.Sub(prev) > 2*w.interval {
			w.samples = w.samples[:0]
			log.Printf("alerts: sustained reset rule_id=%d reason=sample_gap gap=%s", ruleID, at.Sub(prev))
		}
	}
	if !breaching {
		w.samples = w.samples[:0]
		return false
	}
	// Track the breach run as just its boundary samples: samples[0] is the
	// run start (first breaching sample since the last reset) and the final
	// entry is the most recent sample (used for gap detection). Capping at two
	// entries keeps memory O(1) during a long sustained breach (FR-017) while
	// preserving the run-start needed to measure the elapsed span (FR-016).
	switch len(w.samples) {
	case 0, 1:
		w.samples = append(w.samples, sustainedSample{at: at, breaching: true})
	default:
		w.samples[len(w.samples)-1] = sustainedSample{at: at, breaching: true}
	}
	// Confirm once the breach has persisted for at least the configured
	// duration, measured as the span from the run start to the current sample.
	// This is jitter-tolerant: it does not require a sample timestamp to land
	// exactly on the cutoff (at - duration), which the previous implementation
	// did and which therefore never confirmed under non-aligned sampling.
	return at.Sub(w.samples[0].at) >= w.duration
}

// NewEngine creates an alert engine.
func NewEngine(db *database.DB, collector *metrics.Collector, dockerMon *docker.Monitor, systemdMon *systemd.Monitor, interval time.Duration) *Engine {
	e := &Engine{
		db:           db,
		collector:    collector,
		interval:     interval,
		cooldowns:    make(map[string]time.Time),
		firingFirst:  make(map[string]time.Time),
		prevDocker:   make(map[string]string),
		prevSystemd:  make(map[string]string),
		sustained:    make(map[int64]*sustainedWindow),
		processedNet: make(map[string]struct{}),
	}
	// Assign through nil checks so a nil *Monitor never becomes a non-nil
	// interface value (the evaluate loop guards on e.docker != nil).
	if dockerMon != nil {
		e.docker = dockerMon
	}
	if systemdMon != nil {
		e.systemd = systemdMon
	}
	return e
}

// SetAlertCallback sets the legacy callback invoked when an alert is created.
// Used for backwards compat with code paths that only need the bare Alert.
func (e *Engine) SetAlertCallback(cb AlertCallback) {
	e.onAlert = cb
}

// SetRichAlertCallback sets the FR-016/019/024-aware callback. When set,
// fires emit the rich callback INSTEAD of the legacy AlertCallback so each
// fire reaches the dispatcher exactly once.
//
// @aitri-trace FR-016 FR-019 FR-024
func (e *Engine) SetRichAlertCallback(cb RichAlertCallback) {
	e.onRich = cb
}

// SetResolveCallback sets the FR-018 resolve-event callback. When set,
// the engine emits a resolve event each time a previously-firing rule's
// condition becomes false again (metric drops below threshold, container
// returns to running, systemd unit returns to active).
//
// @aitri-trace FR-018 BL-023
func (e *Engine) SetResolveCallback(cb ResolveCallback) {
	e.onResolve = cb
}

// SetTransitionCooldowns sets the cooldown windows for Docker container
// state-change and Systemd service state-change alerts. Each duration must
// be >= 1 minute; non-positive values are silently ignored so a corrupt
// settings row cannot disable cooldowns and flood notifications.
//
// @aitri-trace BG-023 BL-001 FR-004
func (e *Engine) SetTransitionCooldowns(dockerCooldown, systemdCooldown time.Duration) {
	if dockerCooldown >= time.Minute {
		e.dockerCooldownNs.Store(int64(dockerCooldown))
	}
	if systemdCooldown >= time.Minute {
		e.systemdCooldownNs.Store(int64(systemdCooldown))
	}
}

// dockerCooldown returns the configured Docker transition cooldown,
// falling back to defaultTransitionCooldown when no value was set.
func (e *Engine) dockerCooldown() time.Duration {
	if v := e.dockerCooldownNs.Load(); v > 0 {
		return time.Duration(v)
	}
	return defaultTransitionCooldown
}

// systemdCooldown returns the configured Systemd transition cooldown,
// falling back to defaultTransitionCooldown when no value was set.
func (e *Engine) systemdCooldown() time.Duration {
	if v := e.systemdCooldownNs.Load(); v > 0 {
		return time.Duration(v)
	}
	return defaultTransitionCooldown
}

// Start begins the evaluation loop.
func (e *Engine) Start(ctx context.Context) {
	ctx, e.cancel = context.WithCancel(ctx)

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.run(ctx)
	}()

	log.Printf("Alert engine started (interval=%v)", e.interval)
}

// Stop cancels the evaluation loop.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	log.Println("Alert engine stopped")
}

// RecentAlerts returns the most recent alerts cached in memory.
func (e *Engine) RecentAlerts() []database.Alert {
	e.recentMu.RLock()
	defer e.recentMu.RUnlock()
	result := make([]database.Alert, len(e.recentAlerts))
	copy(result, e.recentAlerts)
	return result
}

func (e *Engine) run(ctx context.Context) {
	// Wait one interval before first evaluation to let collectors gather data
	select {
	case <-ctx.Done():
		return
	case <-time.After(e.interval):
	}

	e.evaluate()

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.evaluate()
		}
	}
}

func (e *Engine) evaluate() {
	// BG-003: a previous revision built a context.WithTimeout here and then
	// discarded it, leaving a misleading "10s protection" comment. The DB
	// layer does not yet accept a context, so no timeout is actually applied
	// today — removed to avoid giving false safety. If a slow query becomes a
	// real issue, propagate a context through database.DB.ListEnabledAlertConfigs
	// / CreateAlert / ListAlerts and reintroduce WithTimeout at that layer.
	configs, err := e.db.ListEnabledAlertConfigs()
	if err != nil {
		log.Printf("alerts: failed to load configs: %v", err)
		return
	}

	// Evaluate metric-based rules
	var snapshot *metrics.Snapshot
	if e.collector != nil {
		snapshot = e.collector.Latest()
	}
	if snapshot != nil {
		for _, cfg := range configs {
			e.evaluateMetricRule(cfg, snapshot)
		}
	}

	for _, cfg := range configs {
		e.evaluateNetworkRule(cfg)
	}

	// Evaluate Docker state changes
	if e.docker != nil && e.docker.Available() {
		e.evaluateDockerChanges()
	}

	// Evaluate Systemd state changes
	if e.systemd != nil && e.systemd.Available() {
		e.evaluateSystemdChanges()
	}

	// Refresh recent alerts cache
	alerts, err := e.db.ListAlerts(50)
	if err != nil {
		log.Printf("alerts: failed to refresh cache: %v", err)
		return
	}
	e.recentMu.Lock()
	e.recentAlerts = alerts
	e.recentMu.Unlock()

	e.pruneCooldowns()
}

// cooldownRetention is the upper bound on how long an entry stays in
// the cooldowns map after its last trigger. metric:* keys are bounded
// by the count of configured rules and don't leak — but docker:* and
// systemd:* keys are keyed by transient container/service names, so
// the map grew unbounded as containers were created and removed. The
// retention covers any practical cooldown setting (max useful
// cooldown is on the order of an hour).
const cooldownRetention = 24 * time.Hour

// pruneCooldowns drops cooldowns map entries older than the retention
// window. Called at the end of each evaluate() pass so the cost is
// amortised over the eval cadence (default 5 s).
//
// @aitri-trace BG-028 BL-003 FR-004
func (e *Engine) pruneCooldowns() {
	cutoff := time.Now().Add(-cooldownRetention)
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, last := range e.cooldowns {
		if last.Before(cutoff) {
			delete(e.cooldowns, k)
		}
	}
	// Reap firingFirst entries beyond the same retention window so the map
	// can't grow unbounded as docker/systemd source names churn.
	//
	// @aitri-trace FR-019
	for k, ts := range e.firingFirst {
		if ts.Before(cutoff) {
			delete(e.firingFirst, k)
		}
	}
}

func (e *Engine) evaluateMetricRule(cfg database.AlertConfig, snap *metrics.Snapshot) {
	value, ok := extractMetricValue(cfg.Metric, snap)
	if !ok {
		return
	}

	key := fmt.Sprintf("metric:%d", cfg.ID)
	now := time.Now()
	breaching := compareValue(value, cfg.Operator, cfg.Threshold)
	if cfg.SustainedDuration > 0 {
		e.mu.Lock()
		w := e.sustained[cfg.ID]
		if w == nil || w.duration != time.Duration(cfg.SustainedDuration)*time.Second {
			w = &sustainedWindow{duration: time.Duration(cfg.SustainedDuration) * time.Second, interval: e.interval}
			e.sustained[cfg.ID] = w
		}
		breaching = w.add(cfg.ID, now, breaching)
		e.mu.Unlock()
	}

	if !breaching {
		// Not currently breaching — if the rule was previously firing,
		// this is a resolve transition (FR-018).
		e.mu.Lock()
		firstFiredAt, wasFiring := e.firingFirst[key]
		if wasFiring {
			delete(e.firingFirst, key)
			delete(e.cooldowns, key)
		}
		e.mu.Unlock()
		if wasFiring {
			cfgCopy := cfg
			e.emitResolve(&cfgCopy, "metric:"+cfg.Metric, cfg.Severity, firstFiredAt, now)
		}
		return
	}

	// Currently breaching — fire (subject to cooldown).
	e.mu.Lock()
	last, exists := e.cooldowns[key]
	if exists && now.Sub(last) < time.Duration(cfg.CooldownMinutes)*time.Minute {
		e.mu.Unlock()
		return
	}
	e.cooldowns[key] = now
	// Record first_fired_at for FR-019 elapsed-since-breach. Set only on
	// the first fire after a cooldown gap; existing entries are preserved.
	firstFiredAt, hadFirst := e.firingFirst[key]
	if !hadFirst {
		firstFiredAt = now
		e.firingFirst[key] = now
	}
	e.mu.Unlock()

	// Create alert
	alert := &database.Alert{
		ConfigID: &cfg.ID,
		Severity: cfg.Severity,
		Message:  fmt.Sprintf("%s: %.1f %s %.1f", cfg.Name, value, cfg.Operator, cfg.Threshold),
		Source:   cfg.Metric,
		Value:    &value,
	}
	if err := e.db.CreateAlert(alert); err != nil {
		log.Printf("alerts: failed to create alert: %v", err)
		return
	}
	cfgCopy := cfg
	e.emitFire(alert, &cfgCopy, firstFiredAt)
}

// evaluateNetworkRule evaluates FR-022 network alert rules using the existing
// SQLite-backed NetSample and NetEvent streams.
//
// @aitri-trace FR-ID: FR-071 FR-072 FR-073 FR-074 FR-075, US-ID: US-071 US-072 US-073 US-074 US-075, AC-ID: AC-071-001 AC-072-001 AC-073-001 AC-074-001 AC-075-001, TC-ID: TC-NA-071h TC-NA-072h TC-NA-073h TC-NA-074h TC-NA-075h
func (e *Engine) evaluateNetworkRule(cfg database.AlertConfig) {
	switch cfg.Metric {
	case "latency":
		e.evaluateNetThreshold(cfg, "latency")
	case "loss":
		e.evaluateNetThreshold(cfg, "loss")
	case "dns_failure_rate":
		e.evaluateDNSFailureRule(cfg)
	case "wan_outage", "public_ip_change":
		e.evaluateNetEvents(cfg)
	}
}

func (e *Engine) evaluateNetThreshold(cfg database.AlertConfig, metric string) {
	if cfg.Target == nil || *cfg.Target == "" {
		return
	}
	duration := time.Duration(cfg.SustainedDuration) * time.Second
	if duration < 0 {
		return
	}
	limit := samplesLimit(duration, e.interval)
	samples, err := e.db.RecentNetSamples(*cfg.Target, limit)
	if err != nil {
		log.Printf("alerts: network samples query failed rule_id=%d metric=%s target=%s err=%v", cfg.ID, metric, *cfg.Target, err)
		return
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].TS.Before(samples[j].TS) })
	var value float64
	var ok bool
	switch metric {
	case "latency":
		value, ok = sustainedLatencyValue(cfg, samples, duration, e.interval)
	case "loss":
		value, ok = sustainedLossValue(cfg, samples, duration, e.interval)
	}
	log.Printf("ts=%s rule_id=%d metric=%s target=%s value=%.1f threshold=%.1f sustained=%d in_window=%t fired=%t",
		time.Now().Format(time.RFC3339), cfg.ID, metric, *cfg.Target, value, cfg.Threshold, cfg.SustainedDuration, ok, false)
	if !ok {
		return
	}
	e.fireConfiguredRule(cfg, metric, value, fmt.Sprintf("%s: target=%s current=%.1f threshold %.1f sustained=%ds", cfg.Name, *cfg.Target, value, cfg.Threshold, cfg.SustainedDuration))
}

func (e *Engine) evaluateDNSFailureRule(cfg database.AlertConfig) {
	duration := time.Duration(cfg.SustainedDuration) * time.Second
	limit := samplesLimit(duration, e.interval) * 4
	samples, err := e.db.RecentNetSamplesByKind("dns", limit)
	if err != nil {
		log.Printf("alerts: dns samples query failed rule_id=%d err=%v", cfg.ID, err)
		return
	}
	if len(samples) < 2 {
		log.Printf("ts=%s rule_id=%d metric=dns_failure_rate target=- value=0 threshold=%.1f sustained=%d in_window=false fired=false reason=insufficient_dns_samples",
			time.Now().Format(time.RFC3339), cfg.ID, cfg.Threshold, cfg.SustainedDuration)
		return
	}
	value, resolver, ok := dnsFailureValue(cfg, samples, duration)
	if !ok {
		return
	}
	e.fireConfiguredRule(cfg, "dns_failure_rate", value, fmt.Sprintf("%s: resolver=%s failure_rate=%.1f threshold %.1f sustained=%ds", cfg.Name, resolver, value, cfg.Threshold, cfg.SustainedDuration))
}

func (e *Engine) evaluateNetEvents(cfg database.AlertConfig) {
	events, err := e.db.RecentNetEvents(50)
	if err != nil {
		log.Printf("alerts: net events query failed rule_id=%d metric=%s err=%v", cfg.ID, cfg.Metric, err)
		return
	}
	sort.Slice(events, func(i, j int) bool { return events[i].TS.Before(events[j].TS) })
	for _, ev := range events {
		key := fmt.Sprintf("%d:%d:%s", cfg.ID, ev.ID, ev.Kind)
		if _, done := e.processedNet[key]; done {
			continue
		}
		switch cfg.Metric {
		case "wan_outage":
			e.processedNet[key] = struct{}{}
			e.handleWANEvent(cfg, ev)
		case "public_ip_change":
			if ev.Kind == "public_ip_changed" {
				e.processedNet[key] = struct{}{}
				e.handlePublicIPEvent(cfg, ev)
			}
		}
	}
	e.pruneProcessedNet(events)
}

// pruneProcessedNet drops dedup markers for events that have aged out of the
// recent window. RecentNetEvents only ever returns the newest events, so an
// event ID no longer in the current batch can never reappear and its marker is
// dead weight — without this the map grows unbounded for the process lifetime
// (BL-026). Like the other alert-engine maps, processedNet is owned by the
// single eval/run goroutine; callers must not touch it concurrently.
func (e *Engine) pruneProcessedNet(events []database.NetEvent) {
	if len(e.processedNet) == 0 {
		return
	}
	current := make(map[int64]struct{}, len(events))
	for _, ev := range events {
		current[ev.ID] = struct{}{}
	}
	for key := range e.processedNet {
		id, ok := eventIDFromNetKey(key)
		if !ok {
			continue
		}
		if _, live := current[id]; !live {
			delete(e.processedNet, key)
		}
	}
}

// eventIDFromNetKey extracts the event ID from a "ruleID:eventID:kind"
// processedNet key. Kinds never contain ':', so a SplitN of 3 is safe.
func eventIDFromNetKey(key string) (int64, bool) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) < 2 {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func (e *Engine) handleWANEvent(cfg database.AlertConfig, ev database.NetEvent) {
	switch ev.Kind {
	case "wan_down":
		e.fireConfiguredRule(cfg, "wan_outage", 0, "WAN DOWN — "+ev.Detail)
	case "wan_up":
		key := fmt.Sprintf("metric:%d", cfg.ID)
		e.mu.Lock()
		firstFiredAt, wasFiring := e.firingFirst[key]
		if wasFiring {
			delete(e.firingFirst, key)
			delete(e.cooldowns, key)
		}
		e.mu.Unlock()
		if wasFiring {
			cfgCopy := cfg
			e.emitResolve(&cfgCopy, "wan_outage", "info", firstFiredAt, ev.TS)
			log.Printf("ts=%s rule_id=%d metric=wan_outage event=resolve duration=%s", time.Now().Format(time.RFC3339), cfg.ID, ev.TS.Sub(firstFiredAt))
		}
	}
}

func (e *Engine) handlePublicIPEvent(cfg database.AlertConfig, ev database.NetEvent) {
	var payload struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.Unmarshal([]byte(ev.Detail), &payload); err != nil {
		log.Printf("alerts: public_ip_changed detail parse failed rule_id=%d err=%v", cfg.ID, err)
		return
	}
	if payload.Old == "" || payload.New == "" || payload.Old == payload.New {
		return
	}
	e.fireConfiguredRule(cfg, "public_ip_change", 0, fmt.Sprintf("Public IP changed old=%s new=%s at=%s", payload.Old, payload.New, ev.TS.Format(time.RFC3339)))
}

func (e *Engine) fireConfiguredRule(cfg database.AlertConfig, source string, value float64, message string) {
	key := fmt.Sprintf("metric:%d", cfg.ID)
	now := time.Now()
	cooldown := time.Duration(cfg.CooldownMinutes) * time.Minute
	e.mu.Lock()
	last, exists := e.cooldowns[key]
	if exists && now.Sub(last) < cooldown {
		e.mu.Unlock()
		return
	}
	e.cooldowns[key] = now
	firstFiredAt, hadFirst := e.firingFirst[key]
	if !hadFirst {
		firstFiredAt = now
		e.firingFirst[key] = now
	}
	e.mu.Unlock()
	alert := &database.Alert{
		ConfigID: &cfg.ID,
		Severity: cfg.Severity,
		Message:  message,
		Source:   source,
	}
	if source != "wan_outage" && source != "public_ip_change" {
		alert.Value = &value
	}
	if err := e.db.CreateAlert(alert); err != nil {
		log.Printf("alerts: failed to create network alert: %v", err)
		return
	}
	cfgCopy := cfg
	e.emitFire(alert, &cfgCopy, firstFiredAt)
}

// emitFire dispatches a fire event to whichever callback is wired. The rich
// callback is preferred; falling back to the legacy callback when only the
// legacy one is set.
//
// @aitri-trace FR-016 FR-019 FR-024
func (e *Engine) emitFire(alert *database.Alert, rule *database.AlertConfig, firstFiredAt time.Time) {
	if e.onRich != nil {
		e.onRich(alert, rule, firstFiredAt)
		return
	}
	if e.onAlert != nil {
		e.onAlert(alert)
	}
}

// emitResolve dispatches a resolve event when a previously-firing rule's
// condition becomes false. Silently no-op when no resolve callback is
// wired (legacy callers that only set SetAlertCallback never see resolves).
//
// @aitri-trace FR-018 BL-023
func (e *Engine) emitResolve(rule *database.AlertConfig, sourceID, severity string, firstFiredAt, resolvedAt time.Time) {
	if e.onResolve == nil {
		return
	}
	e.onResolve(rule, sourceID, severity, firstFiredAt, resolvedAt)
}

func (e *Engine) evaluateDockerChanges() {
	containers := e.docker.Containers()
	current := make(map[string]string, len(containers))

	for _, c := range containers {
		current[c.Name] = c.State

		prev, existed := e.prevDocker[c.Name]
		if !existed {
			continue // First cycle for this container, skip
		}

		key := fmt.Sprintf("docker:%s", c.Name)
		now := time.Now()
		isBadState := c.State == "exited" || c.Health == docker.HealthError

		// Resolve detection: container was firing AND has now returned
		// to a healthy state (running). FR-018.
		if !isBadState && c.State == "running" {
			e.mu.Lock()
			firstFiredAt, wasFiring := e.firingFirst[key]
			if wasFiring {
				delete(e.firingFirst, key)
				delete(e.cooldowns, key)
			}
			e.mu.Unlock()
			if wasFiring {
				e.emitResolve(nil, "docker:"+c.Name, "warning", firstFiredAt, now)
			}
		}

		// Detect transition to bad state
		if prev != c.State && isBadState {
			e.mu.Lock()
			last, exists := e.cooldowns[key]
			if exists && now.Sub(last) < e.dockerCooldown() {
				e.mu.Unlock()
				continue
			}
			e.cooldowns[key] = now
			firstFiredAt, hadFirst := e.firingFirst[key]
			if !hadFirst {
				firstFiredAt = now
				e.firingFirst[key] = now
			}
			e.mu.Unlock()

			alert := &database.Alert{
				Severity: "warning",
				Message:  fmt.Sprintf("Container %s changed to %s", c.Name, c.State),
				Source:   "docker:" + c.Name,
			}
			if err := e.db.CreateAlert(alert); err != nil {
				log.Printf("alerts: failed to create docker alert: %v", err)
				continue
			}
			e.emitFire(alert, nil, firstFiredAt)
		}
	}

	e.mu.Lock()
	e.prevDocker = current
	e.mu.Unlock()
}

func (e *Engine) evaluateSystemdChanges() {
	services := e.systemd.Services()
	current := make(map[string]string, len(services))

	for _, svc := range services {
		current[svc.Name] = svc.ActiveState

		prev, existed := e.prevSystemd[svc.Name]
		if !existed {
			continue
		}

		key := fmt.Sprintf("systemd:%s", svc.Name)
		now := time.Now()

		// Resolve detection: unit was firing (failed) and is now back
		// to active. FR-018.
		if svc.ActiveState == "active" {
			e.mu.Lock()
			firstFiredAt, wasFiring := e.firingFirst[key]
			if wasFiring {
				delete(e.firingFirst, key)
				delete(e.cooldowns, key)
			}
			e.mu.Unlock()
			if wasFiring {
				e.emitResolve(nil, "systemd:"+svc.Name, "critical", firstFiredAt, now)
			}
		}

		// Detect transition to failed
		if prev != "failed" && svc.ActiveState == "failed" {
			e.mu.Lock()
			last, exists := e.cooldowns[key]
			if exists && now.Sub(last) < e.systemdCooldown() {
				e.mu.Unlock()
				continue
			}
			e.cooldowns[key] = now
			firstFiredAt, hadFirst := e.firingFirst[key]
			if !hadFirst {
				firstFiredAt = now
				e.firingFirst[key] = now
			}
			e.mu.Unlock()

			alert := &database.Alert{
				Severity: "critical",
				Message:  fmt.Sprintf("Service %s entered failed state", svc.Name),
				Source:   "systemd:" + svc.Name,
			}
			if err := e.db.CreateAlert(alert); err != nil {
				log.Printf("alerts: failed to create systemd alert: %v", err)
				continue
			}
			e.emitFire(alert, nil, firstFiredAt)
		}
	}

	e.mu.Lock()
	e.prevSystemd = current
	e.mu.Unlock()
}

// extractMetricValue extracts the numeric value for a metric type from a snapshot.
func extractMetricValue(metric string, snap *metrics.Snapshot) (float64, bool) {
	switch metric {
	case "cpu":
		return snap.CPU.TotalPercent, true
	case "ram":
		return snap.RAM.Percent, true
	case "disk":
		if len(snap.Disks) > 0 {
			// Use the highest disk usage
			max := 0.0
			for _, d := range snap.Disks {
				if d.Percent > max {
					max = d.Percent
				}
			}
			return max, true
		}
		return 0, false
	case "temp":
		if snap.Temperature != nil {
			return *snap.Temperature, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func samplesLimit(duration, interval time.Duration) int {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if duration <= 0 {
		return 2
	}
	return int(math.Ceil(float64(duration)/float64(interval))) + 2
}

func sustainedLatencyValue(cfg database.AlertConfig, samples []database.NetSample, duration, interval time.Duration) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	if duration <= 0 {
		last := samples[len(samples)-1]
		if last.RTTMs == nil || last.Status != "ok" {
			return 0, false
		}
		return *last.RTTMs, compareValue(*last.RTTMs, cfg.Operator, cfg.Threshold)
	}
	start := samples[0].TS
	lastTS := start
	var lastValue float64
	for i, s := range samples {
		if i > 0 && s.TS.Sub(lastTS) > 2*interval {
			return 0, false
		}
		lastTS = s.TS
		if s.RTTMs == nil || s.Status != "ok" || !compareValue(*s.RTTMs, cfg.Operator, cfg.Threshold) {
			return 0, false
		}
		lastValue = *s.RTTMs
	}
	if lastTS.Sub(start) < duration-interval {
		return 0, false
	}
	return lastValue, true
}

func sustainedLossValue(cfg database.AlertConfig, samples []database.NetSample, duration, interval time.Duration) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].TS.Before(samples[j].TS) })
	if duration > 0 && samples[len(samples)-1].TS.Sub(samples[0].TS) < duration-interval {
		return 0, false
	}
	failures := 0
	for i, s := range samples {
		if i > 0 && s.TS.Sub(samples[i-1].TS) > 2*interval {
			return 0, false
		}
		if s.Status != "ok" {
			failures++
		}
	}
	loss := float64(failures) / float64(len(samples)) * 100
	return loss, compareValue(loss, cfg.Operator, cfg.Threshold)
}

func dnsFailureValue(cfg database.AlertConfig, samples []database.NetSample, duration time.Duration) (float64, string, bool) {
	type counts struct{ total, fail int }
	byTarget := map[string]counts{}
	now := time.Time{}
	for _, s := range samples {
		if s.TS.After(now) {
			now = s.TS
		}
	}
	for _, s := range samples {
		if duration > 0 && now.Sub(s.TS) > duration {
			continue
		}
		c := byTarget[s.Target]
		c.total++
		if s.Status != "ok" {
			c.fail++
		}
		byTarget[s.Target] = c
	}
	var worstTarget string
	var worst float64
	for target, c := range byTarget {
		if c.total < 2 {
			continue
		}
		rate := float64(c.fail) / float64(c.total) * 100
		if rate > worst {
			worst = rate
			worstTarget = target
		}
	}
	if worstTarget == "" {
		log.Printf("alerts: metric=dns_failure_rate reason=insufficient_dns_samples")
		return 0, "", false
	}
	return worst, worstTarget, compareValue(worst, cfg.Operator, cfg.Threshold)
}

// compareValue evaluates value <operator> threshold.
func compareValue(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	default:
		return false
	}
}
