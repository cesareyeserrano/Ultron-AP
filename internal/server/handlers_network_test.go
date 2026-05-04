package server

import (
	"math"
	"testing"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

func ptr(f float64) *float64 { return &f }

func TestComputeRTTSeries_Empty(t *testing.T) {
	series, minMs, maxMs, avgMs := computeRTTSeries(nil)
	if series != nil {
		t.Errorf("series = %v, want nil", series)
	}
	if minMs != 0 || maxMs != 0 || avgMs != 0 {
		t.Errorf("expected all zeros, got min=%v max=%v avg=%v", minMs, maxMs, avgMs)
	}
}

func TestComputeRTTSeries_ReversesAndReturnsStats(t *testing.T) {
	// DB rows are newest-first; the function must reverse them.
	base := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	rows := []database.NetSample{
		{TS: base.Add(20 * time.Second), RTTMs: ptr(30)},
		{TS: base.Add(10 * time.Second), RTTMs: ptr(20)},
		{TS: base, RTTMs: ptr(10)},
	}
	series, minMs, maxMs, avgMs := computeRTTSeries(rows)
	if got := []float64{10, 20, 30}; !floatsEq(series, got) {
		t.Errorf("series = %v, want %v", series, got)
	}
	if minMs != 10 || maxMs != 30 {
		t.Errorf("min=%v max=%v, want 10/30", minMs, maxMs)
	}
	if math.Abs(avgMs-20) > 1e-9 {
		t.Errorf("avg = %v, want 20", avgMs)
	}
}

func TestComputeRTTSeries_FailedSamplesAsNaN_NotInStats(t *testing.T) {
	base := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	rows := []database.NetSample{
		{TS: base.Add(2 * time.Second), RTTMs: nil}, // failed
		{TS: base.Add(time.Second), RTTMs: ptr(10)},
		{TS: base, RTTMs: ptr(20)},
	}
	series, minMs, maxMs, avgMs := computeRTTSeries(rows)
	if len(series) != 3 || !math.IsNaN(series[2]) {
		t.Fatalf("expected NaN at end, got %v", series)
	}
	if minMs != 10 || maxMs != 20 {
		t.Errorf("min=%v max=%v should ignore NaN", minMs, maxMs)
	}
	if math.Abs(avgMs-15) > 1e-9 {
		t.Errorf("avg = %v, want 15 (only valid samples)", avgMs)
	}
}

func TestComputeRTTSeries_AllFailed(t *testing.T) {
	rows := []database.NetSample{
		{TS: time.Now(), RTTMs: nil},
		{TS: time.Now(), RTTMs: nil},
	}
	series, minMs, maxMs, avgMs := computeRTTSeries(rows)
	if len(series) != 2 {
		t.Fatalf("series len = %d, want 2", len(series))
	}
	if minMs != 0 || maxMs != 0 || avgMs != 0 {
		t.Errorf("all-failed window should report 0/0/0, got %v/%v/%v", minMs, maxMs, avgMs)
	}
}

func floatsEq(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.IsNaN(a[i]) != math.IsNaN(b[i]) {
			return false
		}
		if !math.IsNaN(a[i]) && math.Abs(a[i]-b[i]) > 1e-9 {
			return false
		}
	}
	return true
}
