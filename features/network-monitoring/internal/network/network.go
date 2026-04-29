// Package network is the orchestrator for the Ultron-AP Network Monitoring feature.
//
// SKELETON-ONLY. Public types, interfaces and constructor signatures are present
// to allow downstream packages to compile and tests to import. Behavior is not
// implemented yet — see spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package network

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
// All technical_debt entries reference this sentinel.
var ErrSkeleton = errors.New("network-monitoring: skeleton-only — not implemented")

// Sample is a single probe observation.
//
// @aitri-trace FR-ID: FR-016, FR-017, FR-021
type Sample struct {
	TargetID int64
	Kind     ProbeKind
	TS       time.Time
	RTTMs    float64
	JitterMs float64
	LossPct  float64
	Status   SampleStatus
}

// SampleStatus is one of: ok, timeout, unreachable, servfail, nxdomain.
type SampleStatus string

const (
	StatusOK          SampleStatus = "ok"
	StatusTimeout     SampleStatus = "timeout"
	StatusUnreachable SampleStatus = "unreachable"
	StatusServfail    SampleStatus = "servfail"
	StatusNXDomain    SampleStatus = "nxdomain"
)

// ProbeKind categorises the probe that produced a Sample.
type ProbeKind string

const (
	ProbeICMP ProbeKind = "icmp"
	ProbeUDP  ProbeKind = "udp"
	ProbeDNS  ProbeKind = "dns"
)

// Target is a configured probe target row from net_targets.
//
// @aitri-trace FR-ID: FR-016, FR-017
type Target struct {
	ID       int64
	Label    string
	Host     string
	Kind     ProbeKind
	Cadence  time.Duration
	Enabled  bool
	MetaJSON string
}

// Event is a sparse, structured network event row from net_events.
//
// @aitri-trace FR-ID: FR-018, FR-026
type Event struct {
	TS          time.Time
	TSEnd       *time.Time
	Kind        EventKind
	PayloadJSON string
}

// EventKind enumerates valid kinds for net_events.kind.
type EventKind string

const (
	EventOutage             EventKind = "outage"
	EventPublicIPChanged    EventKind = "public_ip_changed"
	EventPathChanged        EventKind = "path_changed"
	EventTargetUnreachable  EventKind = "target_unreachable"
	EventBreakerEngaged     EventKind = "breaker_engaged"
	EventBreakerReleased    EventKind = "breaker_released"
	EventSpeedtestBlocked   EventKind = "speedtest_blocked"
)

// Status is the orchestrator health snapshot exposed via /api/network/health.
//
// @aitri-trace FR-ID: FR-023, NFR-008
type Status struct {
	Collector    string // "ok" | "degraded" | "down"
	Workers      []WorkerStatus
	LastSampleTS time.Time
}

// WorkerStatus is per-worker health for /api/network/health.
type WorkerStatus struct {
	Name  string
	State string // "running" | "paused" | "errored"
}

// HelperClient is the FR-011 privileged-helper Unix-socket client surface used
// by network probes and discovery. Defined as an interface so callers in this
// package depend on a contract, not the parent helper concrete type.
type HelperClient interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// AlertsEngine is the parent FR-004 alert engine surface used by net alert adapter.
type AlertsEngine interface {
	Emit(ctx context.Context, ruleID string, payload map[string]any) error
}

// Deps is the dependency bundle passed by the parent at boot.
//
// @aitri-trace FR-ID: FR-021, FR-022, FR-023
type Deps struct {
	DB     *sql.DB
	Helper HelperClient
	Alerts AlertsEngine
	Logger *slog.Logger
	Clock  func() time.Time
}

// Service is the public lifecycle surface of the network feature.
//
// @aitri-trace FR-ID: FR-018, FR-019, FR-023
type Service interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status() Status
}

// New constructs the network Service. It does not start workers; call Start.
func New(deps Deps) Service {
	return &service{deps: deps}
}

type service struct {
	deps Deps
}

func (s *service) Start(ctx context.Context) error {
	return ErrSkeleton
}

func (s *service) Stop(ctx context.Context) error {
	return ErrSkeleton
}

func (s *service) Status() Status {
	return Status{Collector: "down"}
}
