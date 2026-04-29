// Package store is the SQLite persistence layer for net_* tables.
//
// SKELETON-ONLY. The single-writer goroutine, batching, and read queries are
// declared but not implemented — see spec/04_IMPLEMENTATION_MANIFEST.json.
package store

import (
	"database/sql"
	"errors"
	"time"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/store: skeleton-only — not implemented")

// Metric is a series metric requested from /api/network/series.
type Metric string

const (
	MetricRTT        Metric = "rtt"
	MetricJitter     Metric = "jitter"
	MetricLoss       Metric = "loss"
	MetricThroughput Metric = "throughput"
)

// Window is a series query window.
type Window struct {
	From time.Time
	To   time.Time
}

// SeriesPoint is one downsampled point in a Series response.
type SeriesPoint struct {
	TS   time.Time
	Min  float64
	Mean float64
	P95  float64
	Max  float64
}

// Series is the result of /api/network/series.
type Series struct {
	Points     []SeriesPoint
	Resolution string // "raw" | "minute" | "hour" | "day"
}

// Sample mirrors the root-package Sample but is duplicated here to keep this
// package free of import cycles. Conversion lives in the orchestrator.
type Sample struct {
	TargetID int64
	TS       time.Time
	RTTMs    sql.NullFloat64
	JitterMs sql.NullFloat64
	LossPct  sql.NullFloat64
	Status   string
}

// Speedtest is one row of net_speedtests.
//
// @aitri-trace FR-ID: FR-024, FR-025
type Speedtest struct {
	TS                       time.Time
	Trigger                  string // "manual" | "scheduled"
	DownMbps                 sql.NullFloat64
	UpMbps                   sql.NullFloat64
	IdleRTTMs                sql.NullFloat64
	LoadedRTTDownMs          sql.NullFloat64
	LoadedRTTUpMs            sql.NullFloat64
	BufferbloatAddedDownMs   sql.NullFloat64
	BufferbloatAddedUpMs     sql.NullFloat64
	BufferbloatGrade         sql.NullString
	BytesUsed                int64
	Status                   string
}

// Event is one row of net_events.
type Event struct {
	TS          time.Time
	TSEnd       sql.NullTime
	Kind        string
	PayloadJSON string
}

// Target is one row of net_targets.
type Target struct {
	ID       int64
	Label    string
	Host     string
	Kind     string
	Cadence  time.Duration
	Enabled  bool
	MetaJSON string
}

// Store is the persistence surface used by the orchestrator and HTTP handlers.
//
// @aitri-trace FR-ID: FR-021
type Store interface {
	InsertSample(s Sample) error
	InsertSpeedtest(r Speedtest) error
	RecordEvent(e Event) error
	ListSamples(targetID int64, from, to time.Time) ([]Sample, error)
	ListSeries(targetID int64, metric Metric, window Window) (Series, error)
	Targets() ([]Target, error)
	Close() error
}

// Open returns a Store backed by the parent *sql.DB. Migrations run elsewhere
// (parent internal/database.Migrate hooks 0007_network.sql).
func Open(db *sql.DB) (Store, error) {
	if db == nil {
		return nil, errors.New("network-monitoring/store: nil *sql.DB")
	}
	return &skeletonStore{db: db}, nil
}

type skeletonStore struct {
	db *sql.DB
}

func (s *skeletonStore) InsertSample(Sample) error                                 { return ErrSkeleton }
func (s *skeletonStore) InsertSpeedtest(Speedtest) error                            { return ErrSkeleton }
func (s *skeletonStore) RecordEvent(Event) error                                    { return ErrSkeleton }
func (s *skeletonStore) ListSamples(int64, time.Time, time.Time) ([]Sample, error)  { return nil, ErrSkeleton }
func (s *skeletonStore) ListSeries(int64, Metric, Window) (Series, error)           { return Series{}, ErrSkeleton }
func (s *skeletonStore) Targets() ([]Target, error)                                 { return nil, ErrSkeleton }
func (s *skeletonStore) Close() error                                               { return nil }
