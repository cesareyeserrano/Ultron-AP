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

// Engine evaluates alert rules against current system state.
type Engine struct {
	db        *database.DB
	collector *metrics.Collector
	docker    *docker.Monitor
	systemd   *systemd.Monitor
	interval  time.Duration
	onAlert   AlertCallback

	mu           sync.Mutex
	cooldowns    map[string]time.Time // ruleKey -> last triggered
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
		prevDocker:  make(map[string]string),
		prevSystemd: make(map[string]string),
	}
}

// SetAlertCallback sets a callback invoked when an alert is created.
func (e *Engine) SetAlertCallback(cb AlertCallback) {
	e.onAlert = cb
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
}

func (e *Engine) evaluateMetricRule(cfg database.AlertConfig, snap *metrics.Snapshot) {
	value, ok := extractMetricValue(cfg.Metric, snap)
	if !ok {
		return
	}

	if !compareValue(value, cfg.Operator, cfg.Threshold) {
		return
	}

	// Check cooldown
	key := fmt.Sprintf("metric:%d", cfg.ID)
	e.mu.Lock()
	last, exists := e.cooldowns[key]
	if exists && time.Since(last) < time.Duration(cfg.CooldownMinutes)*time.Minute {
		e.mu.Unlock()
		return
	}
	e.cooldowns[key] = time.Now()
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
	} else if e.onAlert != nil {
		e.onAlert(alert)
	}
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

		// Detect transition to bad state
		if prev != c.State && (c.State == "exited" || c.Health == docker.HealthError) {
			key := fmt.Sprintf("docker:%s", c.Name)
			e.mu.Lock()
			last, exists := e.cooldowns[key]
			if exists && time.Since(last) < e.dockerCooldown() {
				e.mu.Unlock()
				continue
			}
			e.cooldowns[key] = time.Now()
			e.mu.Unlock()

			alert := &database.Alert{
				Severity: "warning",
				Message:  fmt.Sprintf("Container %s changed to %s", c.Name, c.State),
				Source:   "docker:" + c.Name,
			}
			if err := e.db.CreateAlert(alert); err != nil {
				log.Printf("alerts: failed to create docker alert: %v", err)
			} else if e.onAlert != nil {
				e.onAlert(alert)
			}
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

		// Detect transition to failed
		if prev != "failed" && svc.ActiveState == "failed" {
			key := fmt.Sprintf("systemd:%s", svc.Name)
			e.mu.Lock()
			last, exists := e.cooldowns[key]
			if exists && time.Since(last) < e.systemdCooldown() {
				e.mu.Unlock()
				continue
			}
			e.cooldowns[key] = time.Now()
			e.mu.Unlock()

			alert := &database.Alert{
				Severity: "critical",
				Message:  fmt.Sprintf("Service %s entered failed state", svc.Name),
				Source:   "systemd:" + svc.Name,
			}
			if err := e.db.CreateAlert(alert); err != nil {
				log.Printf("alerts: failed to create systemd alert: %v", err)
			} else if e.onAlert != nil {
				e.onAlert(alert)
			}
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
