// Package alerts adapts network samples and events into the parent alert engine
// (FR-022). It does not fork the engine — it only emits rule events.
//
// SKELETON-ONLY. Rule evaluation, sustained-window logic and cooldown tracking
// are not implemented — see spec/04_IMPLEMENTATION_MANIFEST.json technical_debt.
package alerts

import (
	"context"
	"errors"
)

// ErrSkeleton signals that a function is declared but not yet implemented.
var ErrSkeleton = errors.New("network-monitoring/alerts: skeleton-only — not implemented")

// RuleKind is one of the FR-022 network-class rule kinds.
type RuleKind string

const (
	RuleLatency          RuleKind = "latency"
	RuleLoss             RuleKind = "loss"
	RuleWANDown          RuleKind = "wan_down"
	RuleDNSFailRate      RuleKind = "dns_fail_rate"
	RulePublicIPChanged  RuleKind = "public_ip_changed"
	RuleBufferbloat      RuleKind = "bufferbloat"
)

// Severity is the alert severity column on net_alert_rules.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Engine is the parent alert engine surface used by the adapter.
type Engine interface {
	Emit(ctx context.Context, ruleID string, payload map[string]any) error
}

// Adapter forwards network rule firings to the parent Engine.
//
// @aitri-trace FR-ID: FR-022
type Adapter struct {
	engine Engine
}

// New returns a skeleton Adapter.
func New(engine Engine) *Adapter { return &Adapter{engine: engine} }

// Emit forwards a network alert to the parent engine.
func (a *Adapter) Emit(ctx context.Context, ruleID string, payload map[string]any) error {
	if a == nil || a.engine == nil {
		return ErrSkeleton
	}
	return a.engine.Emit(ctx, ruleID, payload)
}
