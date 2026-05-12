package alerts

import (
	"testing"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/stretchr/testify/require"
)

func strPtr(v string) *string { return &v }

func insertSamples(t *testing.T, db *database.DB, target, kind string, start time.Time, rtts []float64, statuses []string) {
	t.Helper()
	for i := range rtts {
		rtt := rtts[i]
		status := "ok"
		if i < len(statuses) && statuses[i] != "" {
			status = statuses[i]
		}
		var rttPtr *float64
		if status == "ok" {
			rttPtr = &rtt
		}
		require.NoError(t, db.InsertNetSample(database.NetSample{
			TS:     start.Add(time.Duration(i) * 5 * time.Second),
			Target: target,
			Kind:   kind,
			RTTMs:  rttPtr,
			Status: status,
		}))
	}
}

func TestTC_NA_071h(t *testing.T) {
	// @aitri-tc TC-NA-071h
	db := setupTestDB(t)
	target := "gateway"
	cfg := database.AlertConfig{ID: 71, Name: "Gateway latency", Metric: "latency", Operator: ">", Threshold: 100, Target: &target, SustainedDuration: 120, Severity: "warning", Enabled: true, CooldownMinutes: 15}
	insertSamples(t, db, target, "icmp", time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC), repeatFloat(125, 24), nil)
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	require.Equal(t, "latency", alerts[0].Source)
	require.Contains(t, alerts[0].Message, "gateway")
	require.Contains(t, alerts[0].Message, "125")
}

func TestTC_NA_071e(t *testing.T) {
	// @aitri-tc TC-NA-071e
	db := setupTestDB(t)
	target := "8.8.8.8"
	cfg := database.AlertConfig{ID: 72, Name: "Google latency", Metric: "latency", Operator: ">", Threshold: 100, Target: &target, SustainedDuration: 120, Severity: "warning", Enabled: true, CooldownMinutes: 15}
	insertSamples(t, db, target, "icmp", time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC), repeatFloat(140, 240), nil)
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
}

func TestTC_NA_072h(t *testing.T) {
	// @aitri-tc TC-NA-072h
	db := setupTestDB(t)
	target := "gateway"
	cfg := database.AlertConfig{ID: 721, Name: "Gateway loss", Metric: "loss", Operator: ">", Threshold: 5, Target: &target, SustainedDuration: 60, Severity: "warning", Enabled: true, CooldownMinutes: 15}
	statuses := []string{"timeout", "ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok"}
	insertSamples(t, db, target, "icmp", time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC), repeatFloat(20, 12), statuses)
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	require.Equal(t, "loss", alerts[0].Source)
	require.Contains(t, alerts[0].Message, "5")
}

func TestTC_NA_072e(t *testing.T) {
	// @aitri-tc TC-NA-072e
	w := &sustainedWindow{duration: 60 * time.Second, interval: 5 * time.Second}
	start := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	for i, v := range []bool{true, false, true, false, true, false, true, false, true, false, true, false} {
		require.False(t, w.add(722, start.Add(time.Duration(i)*5*time.Second), v))
	}
}

func TestTC_NA_073h(t *testing.T) {
	// @aitri-tc TC-NA-073h
	db := setupTestDB(t)
	cfg := database.AlertConfig{ID: 731, Name: "DNS failure", Metric: "dns_failure_rate", Operator: ">", Threshold: 20, SustainedDuration: 120, Severity: "warning", Enabled: true, CooldownMinutes: 15}
	statuses := make([]string, 24)
	for i := 0; i < 8; i++ {
		statuses[i] = "error"
	}
	insertSamples(t, db, "1.1.1.1", "dns", time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC), repeatFloat(10, 24), statuses)
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	require.Contains(t, alerts[0].Message, "1.1.1.1")
}

func TestTC_NA_073e(t *testing.T) {
	// @aitri-tc TC-NA-073e
	db := setupTestDB(t)
	cfg := database.AlertConfig{ID: 732, Name: "DNS failure", Metric: "dns_failure_rate", Operator: ">", Threshold: 20, SustainedDuration: 120, Severity: "warning", Enabled: true, CooldownMinutes: 15}
	insertSamples(t, db, "1.1.1.1", "dns", time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC), repeatFloat(10, 24), nil)
	insertSamples(t, db, "9.9.9.9", "dns", time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC), repeatFloat(10, 24), alternateStatus("error", "ok", 24))
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	require.Contains(t, alerts[0].Message, "9.9.9.9")
}

func TestTC_NA_073f(t *testing.T) {
	// @aitri-tc TC-NA-073f
	db := setupTestDB(t)
	cfg := database.AlertConfig{ID: 733, Name: "DNS failure", Metric: "dns_failure_rate", Operator: ">", Threshold: 20, SustainedDuration: 120, Severity: "warning", Enabled: true, CooldownMinutes: 15}
	insertSamples(t, db, "1.1.1.1", "dns", time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC), []float64{10}, []string{"error"})
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Empty(t, alerts)
}

func TestTC_NA_074h(t *testing.T) {
	// @aitri-tc TC-NA-074h
	db := setupTestDB(t)
	cfg := database.AlertConfig{ID: 741, Name: "WAN outage", Metric: "wan_outage", Operator: "==", Threshold: 0, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.InsertNetEvent(database.NetEvent{TS: time.Date(2026, 5, 11, 10, 3, 0, 0, time.UTC), Kind: "wan_down", Detail: "3 consecutive failures; gateway still ok"}))
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	require.Equal(t, "wan_outage", alerts[0].Source)
	require.Equal(t, "critical", alerts[0].Severity)
}

func TestTC_NA_074e(t *testing.T) {
	// @aitri-tc TC-NA-074e
	db := setupTestDB(t)
	cfg := database.AlertConfig{ID: 742, Name: "WAN outage", Metric: "wan_outage", Operator: "==", Threshold: 0, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.InsertNetEvent(database.NetEvent{TS: time.Date(2026, 5, 11, 10, 3, 0, 0, time.UTC), Kind: "wan_down", Detail: "down"}))
	require.NoError(t, db.InsertNetEvent(database.NetEvent{TS: time.Date(2026, 5, 11, 10, 7, 0, 0, time.UTC), Kind: "wan_up", Detail: "recovered"}))
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	resolves := 0
	eng.SetResolveCallback(func(rule *database.AlertConfig, sourceID, severity string, firstFiredAt, resolvedAt time.Time) {
		resolves++
		require.Equal(t, "info", severity)
	})
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	require.Equal(t, 1, resolves)
}

func TestTC_NA_074f(t *testing.T) {
	// @aitri-tc TC-NA-074f
	db := setupTestDB(t)
	require.NoError(t, db.InsertNetEvent(database.NetEvent{TS: time.Now(), Kind: "wan_down", Detail: "down"}))
	configs, err := db.ListEnabledAlertConfigs()
	require.NoError(t, err)
	require.Empty(t, configs)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Empty(t, alerts)
}

func TestTC_NA_075h(t *testing.T) {
	// @aitri-tc TC-NA-075h
	db := setupTestDB(t)
	cfg := database.AlertConfig{ID: 751, Name: "IP changed", Metric: "public_ip_change", Operator: "==", Threshold: 0, Severity: "info", Enabled: true, CooldownMinutes: 60}
	require.NoError(t, db.InsertNetEvent(database.NetEvent{TS: time.Date(2026, 5, 11, 11, 0, 0, 0, time.UTC), Kind: "public_ip_changed", Detail: `{"old":"1.2.3.4","new":"5.6.7.8"}`}))
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	require.Contains(t, alerts[0].Message, "1.2.3.4")
	require.Contains(t, alerts[0].Message, "5.6.7.8")
}

func TestTC_NA_075e(t *testing.T) {
	// @aitri-tc TC-NA-075e
	db := setupTestDB(t)
	cfg := database.AlertConfig{ID: 752, Name: "IP changed", Metric: "public_ip_change", Operator: "==", Threshold: 0, Severity: "info", Enabled: true, CooldownMinutes: 60}
	require.NoError(t, db.InsertNetEvent(database.NetEvent{TS: time.Now(), Kind: "public_ip_changed", Detail: `{"old":"","new":"5.6.7.8"}`}))
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateNetworkRule(cfg)
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Empty(t, alerts)
}

func TestTC_NA_075f(t *testing.T) {
	// @aitri-tc TC-NA-075f
	db := setupTestDB(t)
	require.NoError(t, db.InsertNetEvent(database.NetEvent{TS: time.Now(), Kind: "public_ip_changed", Detail: `{"old":"1.2.3.4","new":"5.6.7.8"}`}))
	configs, err := db.ListEnabledAlertConfigs()
	require.NoError(t, err)
	require.Empty(t, configs)
}

func TestTC_NA_076h(t *testing.T) {
	// @aitri-tc TC-NA-076h
	db := setupTestDB(t)
	cfg := &database.AlertConfig{Name: "CPU", Metric: "cpu", Operator: ">", Threshold: 80, SustainedDuration: 0, Severity: "critical", Enabled: true, CooldownMinutes: 15}
	require.NoError(t, db.CreateAlertConfig(cfg))
	eng := NewEngine(db, nil, nil, nil, 5*time.Second)
	eng.evaluateMetricRule(*cfg, &metrics.Snapshot{CPU: metrics.CPUMetrics{TotalPercent: 81}})
	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
}

func TestTC_NA_076e(t *testing.T) {
	// @aitri-tc TC-NA-076e
	w := &sustainedWindow{duration: 180 * time.Second, interval: 5 * time.Second}
	start := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 36; i++ {
		require.False(t, w.add(762, start.Add(time.Duration(i)*5*time.Second), true))
	}
	require.True(t, w.add(762, start.Add(36*5*time.Second), true))
}

func TestTC_NA_076f(t *testing.T) {
	// @aitri-tc TC-NA-076f
	w := &sustainedWindow{duration: 300 * time.Second, interval: 5 * time.Second}
	start := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	require.False(t, w.add(763, start, true))
	require.False(t, w.add(763, start.Add(5*time.Second), true))
	require.False(t, w.add(763, start.Add(25*time.Second), true))
	require.Len(t, w.samples, 1)
}

func repeatFloat(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func alternateStatus(a, b string, n int) []string {
	out := make([]string, n)
	for i := range out {
		if i%2 == 0 {
			out[i] = a
		} else {
			out[i] = b
		}
	}
	return out
}
