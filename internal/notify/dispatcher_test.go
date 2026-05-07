package notify

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
	"github.com/cesareyeserrano/ultron-ap/internal/notify/cause"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDispatcher_StartStop(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	d.Start()
	time.Sleep(10 * time.Millisecond)
	d.Stop()
}

func TestDispatcher_Dispatch_NoChannels(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	d.Start()
	defer d.Stop()

	// Should not panic even with no notification channels configured
	d.Dispatch(&database.Alert{Severity: "info", Message: "test", Source: "test"})
	time.Sleep(50 * time.Millisecond)
}

func TestDispatcher_QueueFull(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	// Don't start — queue will fill up but not be consumed

	// Fill queue
	for i := 0; i < 100; i++ {
		d.Dispatch(&database.Alert{Severity: "info", Message: "test", Source: "test"})
	}

	// This should drop without blocking
	d.Dispatch(&database.Alert{Severity: "info", Message: "dropped", Source: "test"})

	assert.Len(t, d.queue, 100) // Queue still at capacity
}

func TestDispatcher_BuildNotifiers_NoConfig(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)

	notifiers := d.buildNotifiers()
	assert.Empty(t, notifiers)
}

func TestDispatcher_BuildNotifiers_TelegramEnabled(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "telegram",
		Enabled: true,
		Config:  `{"bot_token":"123:ABC","chat_id":"456"}`,
	})

	d := NewDispatcher(db)
	notifiers := d.buildNotifiers()
	assert.Len(t, notifiers, 1)
	assert.Equal(t, "telegram", notifiers[0].Name())
}

func TestDispatcher_BuildNotifiers_TelegramDisabled(t *testing.T) {
	db := setupTestDB(t)
	db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "telegram",
		Enabled: false,
		Config:  `{"bot_token":"123:ABC","chat_id":"456"}`,
	})

	d := NewDispatcher(db)
	notifiers := d.buildNotifiers()
	assert.Empty(t, notifiers)
}

// fakeMetricsReader is an in-memory MetricsReader for tests. Returns
// the slice it was constructed with.
type fakeMetricsReader struct {
	hist []metrics.Snapshot
}

func (f *fakeMetricsReader) History(n int) []metrics.Snapshot {
	out := make([]metrics.Snapshot, len(f.hist))
	copy(out, f.hist)
	return out
}

// TestTC_TMU_022i covers FR-022 dispatcher wiring: when a resource fire
// arrives and the metrics ring buffer holds a snapshot ~5min in the past,
// populateEventDefaults populates Event.Trend with the prior CPU value.
//
// @aitri-trace FR-022 AC-022-001 TC-TMU-022i
func TestTC_TMU_022i_DispatcherPopulatesTrendForResourceFire(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)

	now := time.Now()
	d.SetMetricsReader(&fakeMetricsReader{
		hist: []metrics.Snapshot{
			{Timestamp: now.Add(-5 * time.Minute), CPU: metrics.CPUMetrics{TotalPercent: 78}},
			{Timestamp: now.Add(-30 * time.Second), CPU: metrics.CPUMetrics{TotalPercent: 90}},
		},
	})

	val := 92.0
	cfgID := int64(7)
	evt := &Event{
		Alert:   &database.Alert{ConfigID: &cfgID, Severity: "critical", Source: "cpu", Value: &val},
		Rule:    &database.AlertConfig{ID: 7, Metric: "cpu", Operator: ">", Threshold: 80, Severity: "critical"},
		Kind:    EventFire,
		Surface: SurfaceResource,
	}
	d.populateEventDefaults(evt)

	if evt.Trend == nil {
		t.Fatalf("Trend not populated; want non-nil")
	}
	if evt.Trend.Prior != 78 {
		t.Errorf("Prior = %v; want 78", evt.Trend.Prior)
	}
	if evt.Trend.Current != 92 {
		t.Errorf("Current = %v; want 92", evt.Trend.Current)
	}
	if evt.Trend.Unit != "%" {
		t.Errorf("Unit = %q; want '%%'", evt.Trend.Unit)
	}
}

// TestTC_TMU_022i_NoSampleInWindow confirms the dispatcher leaves Trend
// nil when no snapshot is in the 4m30s–5m30s window — the renderer then
// omits the trend block entirely.
//
// @aitri-trace FR-022 AC-022-002 TC-TMU-022i-no-sample
func TestTC_TMU_022i_NoSampleInWindowLeavesTrendNil(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)

	now := time.Now()
	d.SetMetricsReader(&fakeMetricsReader{
		hist: []metrics.Snapshot{
			{Timestamp: now.Add(-30 * time.Second), CPU: metrics.CPUMetrics{TotalPercent: 90}},
		},
	})

	val := 92.0
	evt := &Event{
		Alert:   &database.Alert{Severity: "critical", Source: "cpu", Value: &val},
		Rule:    &database.AlertConfig{Metric: "cpu", Operator: ">", Threshold: 80, Severity: "critical"},
		Kind:    EventFire,
		Surface: SurfaceResource,
	}
	d.populateEventDefaults(evt)

	if evt.Trend != nil {
		t.Fatalf("Trend = %+v; want nil (no sample in 4m30-5m30 window)", evt.Trend)
	}
}

// TestTC_TMU_022i_NonResourceSurfaceNoTrend confirms docker/systemd fires
// never get Trend populated, even when the ring buffer holds samples.
//
// @aitri-trace FR-022 TC-TMU-022i-non-resource
func TestTC_TMU_022i_NonResourceSurfaceNoTrend(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	now := time.Now()
	d.SetMetricsReader(&fakeMetricsReader{
		hist: []metrics.Snapshot{
			{Timestamp: now.Add(-5 * time.Minute), CPU: metrics.CPUMetrics{TotalPercent: 50}},
		},
	})

	val := 1.0
	evt := &Event{
		Alert:   &database.Alert{Severity: "warning", Source: "docker:nginx", Value: &val},
		Kind:    EventFire,
		Surface: SurfaceDocker,
	}
	d.populateEventDefaults(evt)

	if evt.Trend != nil {
		t.Fatalf("docker fire got Trend = %+v; want nil", evt.Trend)
	}
}

// fakeSystemdReader / fakeDockerReader are the test injection points for
// the FR-020 / FR-021 surface block populators.
type fakeSystemdReader struct{ services []systemd.ServiceInfo }

func (f *fakeSystemdReader) Services() []systemd.ServiceInfo { return f.services }

type fakeDockerReader struct{ containers []docker.ContainerInfo }

func (f *fakeDockerReader) Containers() []docker.ContainerInfo { return f.containers }

// fakeCauseDeriver is a no-op CauseDeriver so the populator runs without
// the real cause package's subprocess machinery.
type fakeCauseDeriver struct{}

func (fakeCauseDeriver) Resource(ctx context.Context, metricID string, surf cause.ResourceData) (*cause.Cause, error) {
	return nil, nil
}
func (fakeCauseDeriver) Systemd(ctx context.Context, unit string) (*cause.Cause, error) {
	return nil, nil
}
func (fakeCauseDeriver) Docker(ctx context.Context, container string, exitCode int) (*cause.Cause, error) {
	return nil, nil
}

// TestTC_TMU_020i covers FR-020 dispatcher wiring: a systemd-surface fire
// for nginx.service ('failed') populates Event.Systemd with the unit name,
// active state, and active-since timestamp pulled from the systemd monitor.
//
// @aitri-trace FR-020 AC-020-001 TC-TMU-020i
func TestTC_TMU_020i_DispatcherPopulatesSystemdSurface(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	since := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	d.SetSystemdReader(&fakeSystemdReader{services: []systemd.ServiceInfo{
		{Name: "nginx.service", ActiveState: "failed", Since: since},
	}})
	d.SetCauseDeriver(fakeCauseDeriver{})

	val := 1.0
	evt := &Event{
		Alert:   &database.Alert{Severity: "critical", Source: "systemd:nginx.service", Value: &val},
		Kind:    EventFire,
		Surface: SurfaceSystemd,
	}
	d.populateEventDefaults(evt)

	if evt.Systemd == nil {
		t.Fatalf("Systemd not populated")
	}
	if evt.Systemd.Unit != "nginx.service" {
		t.Errorf("Unit = %q; want 'nginx.service'", evt.Systemd.Unit)
	}
	if evt.Systemd.ActiveState != "failed" {
		t.Errorf("ActiveState = %q; want 'failed'", evt.Systemd.ActiveState)
	}
	if !evt.Systemd.ActiveEnter.Equal(since) {
		t.Errorf("ActiveEnter = %v; want %v", evt.Systemd.ActiveEnter, since)
	}
}

// TestTC_TMU_021i covers FR-021 dispatcher wiring: a docker-surface fire
// for 'mealie' (state=exited, status='Exited (137) 5 minutes ago')
// populates Event.Docker with name, image, state, and parsed exit code 137.
//
// @aitri-trace FR-021 AC-021-001 TC-TMU-021i
func TestTC_TMU_021i_DispatcherPopulatesDockerSurfaceWithExitCode(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	d.SetDockerReader(&fakeDockerReader{containers: []docker.ContainerInfo{
		{Name: "mealie", Image: "ghcr.io/mealie-recipes/mealie:v1.6.0",
			State: "exited", Status: "Exited (137) 5 minutes ago"},
	}})
	d.SetCauseDeriver(fakeCauseDeriver{})

	val := 1.0
	evt := &Event{
		Alert:   &database.Alert{Severity: "warning", Source: "docker:mealie", Value: &val},
		Kind:    EventFire,
		Surface: SurfaceDocker,
	}
	d.populateEventDefaults(evt)

	if evt.Docker == nil {
		t.Fatalf("Docker not populated")
	}
	if evt.Docker.Container != "mealie" {
		t.Errorf("Container = %q; want 'mealie'", evt.Docker.Container)
	}
	if evt.Docker.Image != "ghcr.io/mealie-recipes/mealie:v1.6.0" {
		t.Errorf("Image = %q; unexpected", evt.Docker.Image)
	}
	if !evt.Docker.HasExitCode || evt.Docker.ExitCode != 137 {
		t.Errorf("HasExitCode/ExitCode = %v/%d; want true/137", evt.Docker.HasExitCode, evt.Docker.ExitCode)
	}
}

// TestTC_TMU_021i_NoStatusExitCode covers the running-state path: when
// the container is not exited, no exit code should be parsed.
//
// @aitri-trace FR-021 AC-021-001 TC-TMU-021i-running
func TestTC_TMU_021i_RunningContainerHasNoExitCode(t *testing.T) {
	db := setupTestDB(t)
	d := NewDispatcher(db)
	d.SetDockerReader(&fakeDockerReader{containers: []docker.ContainerInfo{
		{Name: "mealie", Image: "img", State: "running", Status: "Up 3 hours"},
	}})
	d.SetCauseDeriver(fakeCauseDeriver{})

	val := 1.0
	evt := &Event{
		Alert:   &database.Alert{Severity: "warning", Source: "docker:mealie", Value: &val},
		Kind:    EventFire,
		Surface: SurfaceDocker,
	}
	d.populateEventDefaults(evt)

	if evt.Docker == nil {
		t.Fatalf("Docker not populated")
	}
	if evt.Docker.HasExitCode {
		t.Errorf("HasExitCode = true; want false for running state")
	}
}
