// Module: ai
// Purpose: Provider-agnostic AI explanation service for the ai-insights feature.
//          Reads already-collected telemetry, redacts secrets, calls an external
//          OpenAI-compatible LLM, and returns a grounded probable-cause +
//          remediation. Read-only: this package imports NO action/control code
//          (no privileged/docker-controls/systemd-controls) so it structurally
//          cannot take an action (FR-016, no_go_zone B).
// Dependencies: net/http, internal/database (config), read-only telemetry readers.
package ai

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors surfaced by the service and mapped to HTTP status by handlers.
var (
	// ErrNotConfigured means AI is disabled or missing endpoint/key (FR-019 → 409).
	ErrNotConfigured = errors.New("ai not configured")
	// ErrInsufficient means there is not enough telemetry to explain (FR-016 → 422).
	ErrInsufficient = errors.New("insufficient telemetry to explain")
)

// ScopeKind selects what an explanation is about.
type ScopeKind string

const (
	ScopeSystem  ScopeKind = "system"
	ScopeInsight ScopeKind = "insight"
)

// Scope is the explanation request target.
type Scope struct {
	Kind      ScopeKind
	InsightID string // rule id of the fired insight when Kind == ScopeInsight
}

// Explanation is the structured result returned to callers (FR-016, FR-024).
type Explanation struct {
	Cause        string   `json:"cause"`
	Remediation  string   `json:"remediation"`
	CitedSignals []string `json:"cited_signals"`
	Unverified   bool     `json:"unverified"`
	LatencyMS    int64    `json:"latency_ms"`
}

// ConfigStore reads the persisted AI settings at call time so a Settings change
// takes effect on the next request without a restart (FR-020).
type ConfigStore interface {
	GetAISettings() (Config, error)
}

// ConfigFunc adapts a plain function to ConfigStore.
type ConfigFunc func() (Config, error)

// GetAISettings implements ConfigStore.
func (f ConfigFunc) GetAISettings() (Config, error) { return f() }

// Config is the runtime AI configuration (mirror of database.AISettings, kept
// local so the ai package does not force a database import on its consumers).
type Config struct {
	Enabled       bool
	EndpointURL   string
	Model         string
	APIKey        string
	TelegramPush  bool
	TimeoutMS     int
	AllowInsecure bool
}

// Service generates AI explanations. It is safe for concurrent use.
type Service struct {
	cfgStore     ConfigStore
	sources      Sources
	client       *Client
	extraSecrets func() []string
	now          func() time.Time
}

// New builds a Service. extraSecrets may be nil; when set it supplies additional
// secret values (e.g. notification tokens) to redact from every prompt (FR-023).
func New(cfgStore ConfigStore, sources Sources, hc HTTPDoer, extraSecrets func() []string) *Service {
	return &Service{
		cfgStore:     cfgStore,
		sources:      sources,
		client:       NewClient(hc),
		extraSecrets: extraSecrets,
		now:          time.Now,
	}
}

// Enabled reports whether AI is configured and usable right now (FR-019).
//
// @aitri-trace FR-ID: FR-019, US-ID: US-019, AC-ID: AC-019-1h, TC-ID: TC-AI-019e
func (s *Service) Enabled() bool {
	cfg, err := s.cfgStore.GetAISettings()
	if err != nil {
		return false
	}
	return cfg.usable()
}

func (c Config) usable() bool {
	return c.Enabled && c.EndpointURL != "" && c.APIKey != ""
}

// Explain produces a grounded explanation for the given scope. It enforces the
// configured timeout, redacts secrets before the prompt leaves the process, and
// never performs a control action.
//
// @aitri-trace FR-ID: FR-016, US-ID: US-016, AC-ID: AC-016-1h, TC-ID: TC-AI-016h
func (s *Service) Explain(ctx context.Context, scope Scope) (*Explanation, error) {
	cfg, err := s.cfgStore.GetAISettings()
	if err != nil {
		return nil, ErrNotConfigured
	}
	if !cfg.usable() {
		return nil, ErrNotConfigured
	}

	snap := Collect(s.sources, scope)
	if !snap.Sufficient(scope) {
		return nil, ErrInsufficient
	}

	secrets := s.gatherSecrets(cfg)
	userPrompt := buildPrompt(snap, scope, secrets)

	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(DefaultTimeoutMS) * time.Millisecond
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := s.now()
	content, err := s.client.Complete(callCtx, cfg, systemPrompt, userPrompt)
	latency := s.now().Sub(start).Milliseconds()
	if err != nil {
		return nil, err
	}

	exp := parseExplanation(content)
	exp.LatencyMS = latency
	return exp, nil
}

// Test issues a minimal request to verify reachability without saving anything
// (FR-025). It returns the resolved model on success.
//
// @aitri-trace FR-ID: FR-025, US-ID: US-018, AC-ID: AC-018-1h, TC-ID: TC-AI-020h
func (s *Service) Test(ctx context.Context, cfg Config) (string, error) {
	if cfg.EndpointURL == "" || cfg.APIKey == "" {
		return "", ErrNotConfigured
	}
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(DefaultTimeoutMS) * time.Millisecond
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := s.client.Complete(callCtx, cfg, systemPrompt, "ping")
	if err != nil {
		return "", err
	}
	return cfg.Model, nil
}

// gatherSecrets returns the values that must never reach the provider or logs:
// the AI key plus any extra provider secrets (FR-023).
func (s *Service) gatherSecrets(cfg Config) []string {
	secrets := []string{cfg.APIKey}
	if s.extraSecrets != nil {
		secrets = append(secrets, s.extraSecrets()...)
	}
	return secrets
}
