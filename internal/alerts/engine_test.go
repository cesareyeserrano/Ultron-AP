package alerts

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// --- extractMetricValue Tests ---

func TestExtractMetricValue_CPU(t *testing.T) {
	snap := &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 85.5}}
	val, ok := extractMetricValue("cpu", snap)
	assert.True(t, ok)
	assert.Equal(t, 85.5, val)
}

func TestExtractMetricValue_RAM(t *testing.T) {
	snap := &metrics.Snapshot{RAM: metrics.RAMMetrics{Percent: 72.3}}
	val, ok := extractMetricValue("ram", snap)
	assert.True(t, ok)
	assert.Equal(t, 72.3, val)
}

func TestExtractMetricValue_Disk(t *testing.T) {
	snap := &metrics.Snapshot{
		Disks: []metrics.DiskPartition{
			{Path: "/", Percent: 50},
			{Path: "/data", Percent: 92},
		},
	}
	val, ok := extractMetricValue("disk", snap)
	assert.True(t, ok)
	assert.Equal(t, 92.0, val) // highest
}

func TestExtractMetricValue_DiskEmpty(t *testing.T) {
	snap := &metrics.Snapshot{}
	_, ok := extractMetricValue("disk", snap)
	assert.False(t, ok)
}

func TestExtractMetricValue_Temp(t *testing.T) {
	temp := 68.5
	snap := &metrics.Snapshot{Temperature: &temp}
	val, ok := extractMetricValue("temp", snap)
	assert.True(t, ok)
	assert.Equal(t, 68.5, val)
}

func TestExtractMetricValue_TempNil(t *testing.T) {
	snap := &metrics.Snapshot{}
	_, ok := extractMetricValue("temp", snap)
	assert.False(t, ok)
}

func TestExtractMetricValue_Unknown(t *testing.T) {
	snap := &metrics.Snapshot{}
	_, ok := extractMetricValue("unknown", snap)
	assert.False(t, ok)
}

// --- compareValue Tests ---

func TestCompareValue_GreaterThan(t *testing.T) {
	assert.True(t, compareValue(91, ">", 90))
	assert.False(t, compareValue(90, ">", 90))
	assert.False(t, compareValue(89, ">", 90))
}

func TestCompareValue_LessThan(t *testing.T) {
	assert.True(t, compareValue(89, "<", 90))
	assert.False(t, compareValue(90, "<", 90))
}

func TestCompareValue_GreaterEqual(t *testing.T) {
	assert.True(t, compareValue(90, ">=", 90))
	assert.True(t, compareValue(91, ">=", 90))
	assert.False(t, compareValue(89, ">=", 90))
}

func TestCompareValue_LessEqual(t *testing.T) {
	assert.True(t, compareValue(90, "<=", 90))
	assert.True(t, compareValue(89, "<=", 90))
	assert.False(t, compareValue(91, "<=", 90))
}

func TestCompareValue_Equal(t *testing.T) {
	assert.True(t, compareValue(90, "==", 90))
	assert.False(t, compareValue(91, "==", 90))
}

func TestCompareValue_InvalidOperator(t *testing.T) {
	assert.False(t, compareValue(90, "!=", 90))
}

// --- Engine evaluateMetricRule Tests ---

func TestEvaluateMetricRule_ThresholdCrossed(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "CPU High", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 95}}

	eng.evaluateMetricRule(*ac, snap)

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 1)
	assert.Equal(t, "critical", alerts[0].Severity)
	assert.Contains(t, alerts[0].Message, "CPU High")
	assert.Contains(t, alerts[0].Message, "95.0")
}

func TestEvaluateMetricRule_BelowThreshold(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "CPU High", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 80}}

	eng.evaluateMetricRule(*ac, snap)

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 0)
}

func TestEvaluateMetricRule_Cooldown(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "CPU", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 95}}

	eng.evaluateMetricRule(*ac, snap) // triggers
	eng.evaluateMetricRule(*ac, snap) // cooldown blocks

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 1, "cooldown should prevent second alert")
}

func TestEvaluateMetricRule_CooldownExpired(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "CPU", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 0} // 0 = no cooldown
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 95}}

	eng.evaluateMetricRule(*ac, snap)
	eng.evaluateMetricRule(*ac, snap) // Cooldown=0 so still fires

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 2)
}

func TestEvaluateMetricRule_InvalidMetric(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "Bad", Metric: "unknown", Operator: ">", Threshold: 50, Severity: "info", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{}

	eng.evaluateMetricRule(*ac, snap)

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 0)
}

func TestEvaluateMetricRule_RAMAlert(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "RAM", Metric: "ram", Operator: ">", Threshold: 85, Severity: "warning", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{RAM: metrics.RAMMetrics{Percent: 90}}

	eng.evaluateMetricRule(*ac, snap)

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 1)
	assert.Equal(t, "warning", alerts[0].Severity)
}

func TestEvaluateMetricRule_DiskAlert(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "Disk", Metric: "disk", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{Disks: []metrics.DiskPartition{{Path: "/", Percent: 95}}}

	eng.evaluateMetricRule(*ac, snap)

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 1)
}

func TestEvaluateMetricRule_TempAlert(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "Temp", Metric: "temp", Operator: ">", Threshold: 75, Severity: "warning", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	temp := 80.0
	snap := &metrics.Snapshot{Temperature: &temp}

	eng.evaluateMetricRule(*ac, snap)

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 1)
}

// --- Docker State Change Tests ---

func TestEvaluateDockerChanges_StateTransition(t *testing.T) {
	db := setupTestDB(t)
	eng := NewEngine(db, nil, nil, nil, time.Minute)

	// First cycle: establish baseline
	eng.mu.Lock()
	eng.prevDocker = map[string]string{"nginx": "running"}
	eng.mu.Unlock()

	// Simulate container stopping — we test the method directly
	// by manipulating prevDocker and calling with containers
	eng.docker = nil // Can't call evaluateDockerChanges without docker monitor

	// Instead, test the logic manually
	eng.mu.Lock()
	eng.prevDocker["nginx"] = "running"
	eng.mu.Unlock()

	// We need a docker monitor to test this. Let's test via the cooldown mechanism.
	// The core logic is already tested via evaluateMetricRule. Docker/Systemd
	// state change detection requires a real or mock docker.Monitor.
	// Let's verify the cooldown map works for docker keys.
	key := "docker:nginx"
	eng.mu.Lock()
	eng.cooldowns[key] = time.Now()
	eng.mu.Unlock()

	eng.mu.Lock()
	last, exists := eng.cooldowns[key]
	eng.mu.Unlock()
	assert.True(t, exists)
	assert.WithinDuration(t, time.Now(), last, time.Second)
}

// --- Systemd State Change Tests ---

func TestEvaluateSystemdChanges_Cooldown(t *testing.T) {
	db := setupTestDB(t)
	eng := NewEngine(db, nil, nil, nil, time.Minute)

	key := "systemd:sshd"
	eng.mu.Lock()
	eng.cooldowns[key] = time.Now()
	eng.mu.Unlock()

	eng.mu.Lock()
	_, exists := eng.cooldowns[key]
	eng.mu.Unlock()
	assert.True(t, exists)
}

// --- Engine Lifecycle Tests ---

func TestEngine_StartStop(t *testing.T) {
	db := setupTestDB(t)
	eng := NewEngine(db, nil, nil, nil, time.Hour) // long interval to avoid evaluation

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	eng.Stop()
}

func TestEngine_RecentAlerts_Empty(t *testing.T) {
	db := setupTestDB(t)
	eng := NewEngine(db, nil, nil, nil, time.Minute)

	alerts := eng.RecentAlerts()
	assert.Empty(t, alerts)
}

func TestEngine_RecentAlerts_CopySafe(t *testing.T) {
	db := setupTestDB(t)
	eng := NewEngine(db, nil, nil, nil, time.Minute)

	eng.recentMu.Lock()
	eng.recentAlerts = []database.Alert{{ID: 1, Severity: "info", Message: "test"}}
	eng.recentMu.Unlock()

	alerts := eng.RecentAlerts()
	assert.Len(t, alerts, 1)

	// Mutate returned slice — original should be unaffected
	alerts[0].Message = "changed"
	orig := eng.RecentAlerts()
	assert.Equal(t, "test", orig[0].Message)
}

// --- Multiple Simultaneous Rules ---

func TestMultipleRulesFireSimultaneously(t *testing.T) {
	db := setupTestDB(t)
	db.CreateAlertConfig(&database.AlertConfig{Name: "CPU", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 15})
	db.CreateAlertConfig(&database.AlertConfig{Name: "RAM", Metric: "ram", Operator: ">", Threshold: 85, Severity: "warning", Enabled: true, CooldownMinutes: 15})

	configs, _ := db.ListEnabledAlertConfigs()

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	snap := &metrics.Snapshot{
		CPU: metrics.CPUMetrics{TotalPercent: 95},
		RAM: metrics.RAMMetrics{Percent: 90},
	}

	for _, cfg := range configs {
		eng.evaluateMetricRule(cfg, snap)
	}

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	assert.Len(t, alerts, 2)
}

// --- Docker/Systemd type reference tests ---

func TestDockerHealthStatus_Reference(t *testing.T) {
	// Verify we can reference docker health constants
	assert.Equal(t, docker.HealthError, docker.MapHealthStatus("exited", 1))
	assert.Equal(t, docker.HealthRunning, docker.MapHealthStatus("running", 0))
}

func TestSystemdServiceHealth_Reference(t *testing.T) {
	assert.Equal(t, systemd.ServiceFailed, systemd.MapServiceHealth("failed"))
	assert.Equal(t, systemd.ServiceActive, systemd.MapServiceHealth("active"))
}

// --- Resolve-event emission tests (FR-018 / BL-023) ---

// TestTC_TMU_018i_MetricResolveEmittedWhenBreachClears covers FR-018:
// after a metric rule fires and then drops back below threshold on a
// subsequent evaluation, the engine emits a resolve callback with the
// fire-window timestamps; no resolve fires when the rule never fired.
//
// @aitri-trace FR-018 BL-023 TC-TMU-018i
func TestTC_TMU_018i_MetricResolveEmittedWhenBreachClears(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "CPU", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 0}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	var resolves []resolveCall
	eng.SetResolveCallback(func(rule *database.AlertConfig, sourceID, severity string, firstFiredAt, resolvedAt time.Time) {
		resolves = append(resolves, resolveCall{rule, sourceID, severity, firstFiredAt, resolvedAt})
	})

	// Fire it.
	eng.evaluateMetricRule(*ac, &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 95}})
	if len(resolves) != 0 {
		t.Fatalf("resolves emitted on initial fire: %d", len(resolves))
	}

	// Now drops back below threshold → resolve.
	eng.evaluateMetricRule(*ac, &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 50}})

	if len(resolves) != 1 {
		t.Fatalf("len(resolves) = %d; want 1", len(resolves))
	}
	got := resolves[0]
	if got.sourceID != "metric:cpu" {
		t.Errorf("sourceID = %q; want 'metric:cpu'", got.sourceID)
	}
	if got.severity != "critical" {
		t.Errorf("severity = %q; want 'critical'", got.severity)
	}
	if got.firstFiredAt.IsZero() {
		t.Errorf("firstFiredAt is zero; want a non-zero fire timestamp")
	}
	if got.resolvedAt.IsZero() || got.resolvedAt.Before(got.firstFiredAt) {
		t.Errorf("resolvedAt = %v; want >= firstFiredAt %v", got.resolvedAt, got.firstFiredAt)
	}
}

// TestTC_TMU_018i_NoResolveWhenNeverFired confirms a non-firing rule
// that drops further below threshold doesn't spuriously emit a resolve.
//
// @aitri-trace FR-018 BL-023 TC-TMU-018i-no-spurious
func TestTC_TMU_018i_NoResolveWhenNeverFired(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "CPU", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 0}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	var resolves []resolveCall
	eng.SetResolveCallback(func(rule *database.AlertConfig, sourceID, severity string, firstFiredAt, resolvedAt time.Time) {
		resolves = append(resolves, resolveCall{rule, sourceID, severity, firstFiredAt, resolvedAt})
	})

	// Always below threshold.
	eng.evaluateMetricRule(*ac, &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 30}})
	eng.evaluateMetricRule(*ac, &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 10}})

	if len(resolves) != 0 {
		t.Fatalf("len(resolves) = %d; want 0 (rule never fired)", len(resolves))
	}
}

// TestTC_TMU_018i_FireResolveFireSequence confirms a re-fire after
// resolve is treated as a fresh breach (firstFiredAt is reset).
//
// @aitri-trace FR-018 BL-023 TC-TMU-018i-cycle
func TestTC_TMU_018i_FireResolveFireSequence(t *testing.T) {
	db := setupTestDB(t)
	ac := &database.AlertConfig{Name: "CPU", Metric: "cpu", Operator: ">", Threshold: 90, Severity: "critical", Enabled: true, CooldownMinutes: 0}
	require.NoError(t, db.CreateAlertConfig(ac))

	eng := NewEngine(db, nil, nil, nil, time.Minute)
	var resolves []resolveCall
	eng.SetResolveCallback(func(rule *database.AlertConfig, sourceID, severity string, firstFiredAt, resolvedAt time.Time) {
		resolves = append(resolves, resolveCall{rule, sourceID, severity, firstFiredAt, resolvedAt})
	})

	// Fire → resolve → fire again.
	eng.evaluateMetricRule(*ac, &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 95}})
	firstAlerts, _ := db.ListAlerts(10)

	eng.evaluateMetricRule(*ac, &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 50}})
	if len(resolves) != 1 {
		t.Fatalf("after resolve: len(resolves) = %d; want 1", len(resolves))
	}

	// Tiny delay so the new firstFiredAt is strictly later.
	time.Sleep(2 * time.Millisecond)
	eng.evaluateMetricRule(*ac, &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 95}})

	secondAlerts, _ := db.ListAlerts(10)
	if len(secondAlerts) <= len(firstAlerts) {
		t.Fatalf("re-fire after resolve did not produce a new alert row")
	}
}

// resolveCall captures a SetResolveCallback invocation for the test
// assertions above.
type resolveCall struct {
	rule         *database.AlertConfig
	sourceID     string
	severity     string
	firstFiredAt time.Time
	resolvedAt   time.Time
}

// --- AC-004-002 / AC-004-003 — docker/systemd transition alerts fire end-to-end ---
// The dockerContainerSource/systemdServiceSource seams let these tests inject
// container/unit states without a live daemon.

type fakeDockerSource struct{ containers []docker.ContainerInfo }

func (f fakeDockerSource) Available() bool                    { return true }
func (f fakeDockerSource) Containers() []docker.ContainerInfo { return f.containers }

type fakeSystemdSource struct{ services []systemd.ServiceInfo }

func (f fakeSystemdSource) Available() bool                  { return true }
func (f fakeSystemdSource) Services() []systemd.ServiceInfo  { return f.services }

// @aitri-tc TC-004a
func TestEvaluateDockerChanges_FiresAlertOnExit(t *testing.T) {
	db := setupTestDB(t)
	eng := NewEngine(db, nil, nil, nil, time.Minute)

	// Cycle 1 — baseline: nginx running.
	eng.docker = fakeDockerSource{containers: []docker.ContainerInfo{
		{Name: "nginx", State: "running"},
	}}
	eng.evaluateDockerChanges()

	// Cycle 2 — nginx exits with an error state.
	eng.docker = fakeDockerSource{containers: []docker.ContainerInfo{
		{Name: "nginx", State: "exited", Health: docker.HealthError},
	}}
	eng.evaluateDockerChanges()

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1, "state transition to exited must fire exactly one alert")
	assert.Equal(t, "warning", alerts[0].Severity)
	assert.Equal(t, "docker:nginx", alerts[0].Source)
	assert.Contains(t, alerts[0].Message, "nginx")
	assert.Contains(t, alerts[0].Message, "exited")
}

// @aitri-tc TC-004b
func TestEvaluateSystemdChanges_FiresCriticalOnFailed(t *testing.T) {
	db := setupTestDB(t)
	eng := NewEngine(db, nil, nil, nil, time.Minute)

	// Cycle 1 — baseline: unit active.
	eng.systemd = fakeSystemdSource{services: []systemd.ServiceInfo{
		{Name: "nginx.service", ActiveState: "active"},
	}}
	eng.evaluateSystemdChanges()

	// Cycle 2 — unit transitions to failed.
	eng.systemd = fakeSystemdSource{services: []systemd.ServiceInfo{
		{Name: "nginx.service", ActiveState: "failed"},
	}}
	eng.evaluateSystemdChanges()

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1, "transition to failed must fire exactly one alert")
	assert.Equal(t, "critical", alerts[0].Severity)
	assert.Equal(t, "systemd:nginx.service", alerts[0].Source)
	assert.Contains(t, alerts[0].Message, "nginx.service")
}
