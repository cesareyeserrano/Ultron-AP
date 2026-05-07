package alerts

import (
	"context"
	"fmt"
	"log"
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

// Engine evaluates alert rules against current system state.
type Engine struct {
	db        *database.DB
	collector *metrics.Collector
	docker    *docker.Monitor
	systemd   *systemd.Monitor
	interval  time.Duration
	onAlert   AlertCallback
	onRich    RichAlertCallback
	onResolve ResolveCallback

	mu           sync.Mutex
	cooldowns    map[string]time.Time // ruleKey -> last triggered
	firingFirst  map[string]time.Time // ruleKey -> first_fired_at (FR-019)
	prevDocker   map[string]string    // containerName -> state
	prevSystemd  map[string]string    // serviceName -> activeState
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

// NewEngine creates an alert engine.
func NewEngine(db *database.DB, collector *metrics.Collector, dockerMon *docker.Monitor, systemdMon *systemd.Monitor, interval time.Duration) *Engine {
	return &Engine{
		db:          db,
		collector:   collector,
		docker:      dockerMon,
		systemd:     systemdMon,
		interval:    interval,
		cooldowns:   make(map[string]time.Time),
		firingFirst: make(map[string]time.Time),
		prevDocker:  make(map[string]string),
		prevSystemd: make(map[string]string),
	}
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
	snapshot := e.collector.Latest()
	if snapshot != nil {
		for _, cfg := range configs {
			e.evaluateMetricRule(cfg, snapshot)
		}
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
