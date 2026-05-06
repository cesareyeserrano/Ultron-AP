// Package insights is the public surface of the insights-engine feature: a
// declarative rules evaluator that consumes a metrics.Snapshot on the parent
// FR-001 5 s tick and emits a current-verdict array for the dashboard SSE
// channel.
//
// Design priorities (per 02_SYSTEM_DESIGN.md):
//
//  1. No new ticker — Eval is called inline by the SSE broker on each tick.
//  2. No new datastore — uses the parent SQLite DB via internal/insights/store.
//  3. Strict isolation from internal/notify and internal/alerts (NFR-021).
//  4. Pre-compiled rule closures (NFR-016) — zero-alloc steady state.
//  5. Per-rule hysteresis (FR-046) — flap-prone rules held at last stable value.
//
// @aitri-trace FR-039 FR-042 FR-046 NFR-021 US-039 US-042 US-046
// TC-IE-001h TC-IE-001f TC-IE-001e TC-IE-004h TC-IE-004f TC-IE-004e TC-IE-008h TC-IE-008f TC-IE-008e TC-IE-011e
package insights

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/help/contract"
	"github.com/cesareyeserrano/ultron-ap/internal/insights/lang"
	"github.com/cesareyeserrano/ultron-ap/internal/insights/rules"
	"github.com/cesareyeserrano/ultron-ap/internal/insights/store"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
)

// Severity strings for the wire format. They mirror store.Severity so the API
// layer can emit them without an extra mapping.
type Severity = store.Severity

// Verdict is one currently-active diagnostic. Returned by Active and shipped
// over the SSE channel.
type Verdict struct {
	RuleID          string    `json:"rule_id"`
	Title           string    `json:"title"`
	Severity        Severity  `json:"severity"`
	VerdictText     string    `json:"verdict_text"`
	Recommendation  string    `json:"recommendation"`
	Links           []string  `json:"links"`
	FirstEmittedAt  time.Time `json:"first_emitted_at"`
	LastEvaluatedAt time.Time `json:"last_evaluated_at"`
}

// LogFunc is the structured-log emitter; the engine wires this to log.Printf.
type LogFunc func(format string, args ...interface{})

// Config wires every seam the engine needs.
type Config struct {
	Store  *store.Store
	Logger LogFunc
	// Now is the time source; defaults to time.Now. Tests inject deterministic clocks.
	Now func() time.Time
	// FlapWindow is the FR-046 detection window. Default 10 s.
	FlapWindow time.Duration
	// FlapThreshold is the transition count that engages hysteresis. Default 5.
	FlapThreshold int
}

// Service is the engine. Use New + Start + Stop for lifecycle.
type Service struct {
	cfg Config

	mu sync.RWMutex
	// compiledRules is the in-memory working set, ordered by severity.
	compiledRules []*compiledRule
	// active maps rule id → verdict for the current published set.
	active map[string]Verdict
	// missingVarSeen rate-limits "skipped-missing-var" log lines (one per rule per var per process).
	missingVarSeen sync.Map // key: ruleID + "\x00" + varName

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// lastEvalAt is updated on every successful evaluation tick.
	lastEvalAt time.Time
	// lastSnapshotMissing flags whether the last tick was skipped (used for
	// the "snapshot-missing" once-per-incident log semantics).
	lastSnapshotMissing bool
}

// compiledRule is the engine's per-rule working state. State.firstEmittedAt
// is in-memory only — verdict history is not persisted (per no-go-zone).
type compiledRule struct {
	rule     store.Rule
	compiled *lang.Compiled
	state    ruleState
}

type ruleState struct {
	lastValue       bool
	firstEmittedAt  time.Time
	lastEvaluatedAt time.Time
	// FR-046 hysteresis state.
	windowStart    time.Time
	transitions    int
	flapHeld       bool
	flapHeldValue  bool
	flapLogged     bool
}

// New constructs an engine but does not load rules — call LoadBundled first.
func New(cfg Config) *Service {
	if cfg.Logger == nil {
		cfg.Logger = func(string, ...interface{}) {}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.FlapWindow <= 0 {
		cfg.FlapWindow = 10 * time.Second
	}
	if cfg.FlapThreshold <= 0 {
		cfg.FlapThreshold = 5
	}
	return &Service{
		cfg:    cfg,
		active: map[string]Verdict{},
	}
}

// LoadBundled seeds the rules table from the embedded JSON and builds the
// in-memory compiled rule set. Called once at startup.
func (s *Service) LoadBundled() error {
	if s.cfg.Store == nil {
		return fmt.Errorf("insights: store not configured")
	}
	loaded, err := rules.LoadBundled(rules.LogFunc(s.cfg.Logger))
	if err != nil {
		return fmt.Errorf("insights: load bundled: %w", err)
	}
	for _, r := range loaded {
		if err := s.cfg.Store.SeedRule(store.Rule{
			ID:             r.ID,
			Title:          r.Title,
			ConditionJSON:  r.ConditionRaw,
			Severity:       store.Severity(r.Severity),
			Verdict:        r.Verdict,
			Recommendation: r.Recommendation,
			Links:          r.Links,
			Source:         "bundled",
		}); err != nil {
			s.cfg.Logger("insights: seed rule_id=%s reason=%v", r.ID, err)
		}
	}
	return s.RefreshFromStore()
}

// RefreshFromStore reloads the compiled rule set from the rules table. The
// engine calls this on startup and on each enable/disable mutation that needs
// to take effect on the next tick.
func (s *Service) RefreshFromStore() error {
	rows, err := s.cfg.Store.LoadAll()
	if err != nil {
		return err
	}
	compiled := make([]*compiledRule, 0, len(rows))
	for _, r := range rows {
		c, err := lang.Compile(r.ConditionJSON)
		if err != nil {
			s.cfg.Logger("insights: compile rule_id=%s reason=%v", r.ID, err)
			continue
		}
		compiled = append(compiled, &compiledRule{
			rule:     r,
			compiled: c,
		})
	}
	// Stable sort: critical → warn → info → id.
	sort.SliceStable(compiled, func(i, j int) bool {
		si, sj := severityRank(compiled[i].rule.Severity), severityRank(compiled[j].rule.Severity)
		if si != sj {
			return si < sj
		}
		return compiled[i].rule.ID < compiled[j].rule.ID
	})
	s.mu.Lock()
	s.compiledRules = compiled
	s.mu.Unlock()
	return nil
}

func severityRank(s store.Severity) int {
	switch s {
	case store.SeverityCritical:
		return 0
	case store.SeverityWarn:
		return 1
	case store.SeverityInfo:
		return 2
	}
	return 3
}

// Start launches a goroutine that listens on the supplied tick channel, calls
// Eval(snapshot) on each tick, and exits when ctx is done. Snapshot retrieval
// is the caller's responsibility — pass a function that returns the latest
// snapshot or nil on collector failure.
func (s *Service) Start(ctx context.Context, tickC <-chan time.Time, snapFn func() *metrics.Snapshot) {
	cctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-cctx.Done():
				return
			case <-tickC:
				snap := snapFn()
				s.Eval(snap)
			}
		}
	}()
}

// Stop signals the engine goroutine to exit and waits for it.
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// Eval runs one evaluation tick against the supplied snapshot. If snap is nil
// the tick is treated as snapshot-missing per FR-039 AC-002: no new verdicts
// are emitted, the previous active set is held, and a single log line is
// recorded per outage (re-armed when a snapshot reappears).
func (s *Service) Eval(snap *metrics.Snapshot) []Verdict {
	now := s.cfg.Now()
	if snap == nil {
		s.mu.Lock()
		if !s.lastSnapshotMissing {
			s.cfg.Logger("insights: snapshot-missing")
			s.lastSnapshotMissing = true
		}
		s.mu.Unlock()
		return s.Active()
	}

	s.mu.Lock()
	s.lastSnapshotMissing = false
	rules := s.compiledRules
	s.mu.Unlock()

	ctx := buildEvalCtx(snap, s.makeMissingLogger())
	ctx.NowMS = now.UnixMilli()

	newActive := make(map[string]Verdict, len(s.active))
	for _, cr := range rules {
		if !cr.rule.Enabled {
			// Disabled mid-flight — drop any active verdict immediately and
			// reset hysteresis state so re-enabling starts fresh.
			cr.state.firstEmittedAt = time.Time{}
			cr.state.lastValue = false
			continue
		}
		live := cr.compiled.Eval(ctx)
		fired := s.applyHysteresis(cr, live, now)
		cr.state.lastEvaluatedAt = now
		if fired {
			if cr.state.firstEmittedAt.IsZero() {
				cr.state.firstEmittedAt = now
			}
			newActive[cr.rule.ID] = Verdict{
				RuleID:          cr.rule.ID,
				Title:           cr.rule.Title,
				Severity:        cr.rule.Severity,
				VerdictText:     cr.rule.Verdict,
				Recommendation:  cr.rule.Recommendation,
				Links:           cr.rule.Links,
				FirstEmittedAt:  cr.state.firstEmittedAt,
				LastEvaluatedAt: now,
			}
		} else {
			cr.state.firstEmittedAt = time.Time{}
		}
	}

	s.mu.Lock()
	s.active = newActive
	s.lastEvalAt = now
	s.mu.Unlock()

	return s.Active()
}

// applyHysteresis updates per-rule transition counters and decides whether
// the verdict should fire on this tick (FR-046). Returns the effective
// boolean — possibly held at last stable value during a flap window.
func (s *Service) applyHysteresis(cr *compiledRule, live bool, now time.Time) bool {
	st := &cr.state
	if !st.windowStart.IsZero() && now.Sub(st.windowStart) >= s.cfg.FlapWindow {
		// Window elapsed — reset and resume normal lifecycle.
		st.windowStart = time.Time{}
		st.transitions = 0
		st.flapHeld = false
		st.flapLogged = false
	}
	transitioned := live != st.lastValue
	if transitioned {
		st.transitions++
		if st.windowStart.IsZero() {
			st.windowStart = now
			// Capture the initial stable value at window start.
			st.flapHeldValue = st.lastValue
		}
		if st.transitions >= s.cfg.FlapThreshold && !st.flapHeld {
			st.flapHeld = true
			if !st.flapLogged {
				s.cfg.Logger("insights: rule-flapping rule_id=%s transition_count=%d",
					cr.rule.ID, st.transitions)
				st.flapLogged = true
			}
		}
	}
	st.lastValue = live
	if st.flapHeld {
		return st.flapHeldValue
	}
	return live
}

// Active returns a copy of the current verdict slice sorted critical → warn →
// info, then by FirstEmittedAt descending within a severity.
func (s *Service) Active() []Verdict {
	s.mu.RLock()
	out := make([]Verdict, 0, len(s.active))
	for _, v := range s.active {
		out = append(out, v)
	}
	s.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if si != sj {
			return si < sj
		}
		return out[i].FirstEmittedAt.After(out[j].FirstEmittedAt)
	})
	return out
}

// LastEvalAt returns the timestamp of the most recent successful Eval call.
func (s *Service) LastEvalAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastEvalAt
}

// SetEnabled updates the rule's enabled flag in the store and refreshes the
// in-memory compiled set. Returns the store error if the rule is unknown.
func (s *Service) SetEnabled(id string, enabled bool) error {
	if err := s.cfg.Store.SetEnabled(id, enabled); err != nil {
		return err
	}
	return s.RefreshFromStore()
}

// makeMissingLogger returns a logger that captures the rule context. The
// returned closure is shared across all rules per tick — collisions on the
// (ruleID, varName) key are rate-limited via missingVarSeen.
func (s *Service) makeMissingLogger() func(varName string) {
	// We don't have a per-rule label here because the EvalCtx is shared
	// across rules in a tick (per NFR-016 — single context allocation).
	// The lang package logs at variable granularity; the engine adds the
	// global "skipped-missing-var" tag so structured log parsers can find it.
	return func(varName string) {
		key := varName
		if _, loaded := s.missingVarSeen.LoadOrStore(key, struct{}{}); loaded {
			return
		}
		s.cfg.Logger("insights: skipped-missing-var var=%s", varName)
	}
}

// buildEvalCtx adapts a metrics.Snapshot into the lang.EvalCtx variable map.
// Variables match the names referenced by bundled.json.
func buildEvalCtx(snap *metrics.Snapshot, missing func(string)) *lang.EvalCtx {
	if snap == nil {
		return &lang.EvalCtx{
			Lookup:        func(string) lang.Value { return lang.None() },
			MissingLogger: missing,
		}
	}
	vars := snapshotVars(snap)
	return &lang.EvalCtx{
		Lookup: func(name string) lang.Value {
			if v, ok := vars[name]; ok {
				return v
			}
			return lang.None()
		},
		MissingLogger: missing,
	}
}

// snapshotVars projects a metrics.Snapshot into the lang variable surface.
// Names line up with bundled.json. Variables that the snapshot can't supply
// (e.g. lan_device_offline_count when the lan-devices feature is off) simply
// don't appear in the map; the lang package treats them as missing → false.
func snapshotVars(snap *metrics.Snapshot) map[string]lang.Value {
	out := map[string]lang.Value{
		"cpu_pct": lang.Number(snap.CPU.TotalPercent),
		"ram_pct": lang.Number(snap.RAM.Percent),
	}
	if snap.Temperature != nil {
		out["temp_c"] = lang.Number(*snap.Temperature)
	}
	for _, p := range snap.Disks {
		if p.Path == "/" {
			out["disk_root_pct"] = lang.Number(p.Percent)
		}
	}
	return out
}

// MergeVars overlays additional named variables on top of a base map. Used by
// the SSE adapter to layer wan_*, lan_*, services_failed, etc., into the
// EvalCtx without coupling the engine to those subsystems.
func MergeVars(base map[string]lang.Value, extra map[string]lang.Value) map[string]lang.Value {
	if base == nil {
		base = map[string]lang.Value{}
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// EvalWithVars runs one tick using a custom variable map (rather than a
// metrics.Snapshot). This is the integration seam used by tests and by the
// SSE broker, where callers project their own subsystem state into the
// variable surface.
func (s *Service) EvalWithVars(now time.Time, vars map[string]lang.Value) []Verdict {
	s.mu.Lock()
	rules := s.compiledRules
	s.lastSnapshotMissing = false
	s.mu.Unlock()

	ctx := &lang.EvalCtx{
		Lookup: func(name string) lang.Value {
			if v, ok := vars[name]; ok {
				return v
			}
			return lang.None()
		},
		MissingLogger: s.makeMissingLogger(),
		NowMS:         now.UnixMilli(),
	}

	newActive := make(map[string]Verdict, len(s.active))
	for _, cr := range rules {
		if !cr.rule.Enabled {
			cr.state.firstEmittedAt = time.Time{}
			cr.state.lastValue = false
			continue
		}
		live := cr.compiled.Eval(ctx)
		fired := s.applyHysteresis(cr, live, now)
		cr.state.lastEvaluatedAt = now
		if fired {
			if cr.state.firstEmittedAt.IsZero() {
				cr.state.firstEmittedAt = now
			}
			newActive[cr.rule.ID] = Verdict{
				RuleID:          cr.rule.ID,
				Title:           cr.rule.Title,
				Severity:        cr.rule.Severity,
				VerdictText:     cr.rule.Verdict,
				Recommendation:  cr.rule.Recommendation,
				Links:           cr.rule.Links,
				FirstEmittedAt:  cr.state.firstEmittedAt,
				LastEvaluatedAt: now,
			}
		} else {
			cr.state.firstEmittedAt = time.Time{}
		}
	}

	s.mu.Lock()
	s.active = newActive
	s.lastEvalAt = now
	s.mu.Unlock()

	return s.Active()
}

// SnapshotMissing reports the snapshot-missing condition without supplying a
// snapshot. Used by the SSE broker when the collector returned an error.
func (s *Service) SnapshotMissing() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastSnapshotMissing {
		s.cfg.Logger("insights: snapshot-missing")
		s.lastSnapshotMissing = true
	}
}

// MarshalActive returns the active verdict slice as JSON. Helper for the API.
func (s *Service) MarshalActive() ([]byte, error) {
	v := s.Active()
	if v == nil {
		v = []Verdict{}
	}
	return json.Marshal(v)
}

// LastSnapshotMissing exposes the engine's most recent snapshot-missing flag
// for tests that assert the FR-039 AC-002 single-log semantics.
func (s *Service) LastSnapshotMissing() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSnapshotMissing
}

// RuleLinks returns a snapshot of every loaded rule's id and links field as
// plain data, suitable for the help-page links validator (FR-052). The slice
// and each Links slice are owned by the caller; mutating them does not affect
// the engine.
//
// @aitri-trace FR-052 NFR-026
func (s *Service) RuleLinks() []contract.RuleLink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]contract.RuleLink, 0, len(s.compiledRules))
	for _, cr := range s.compiledRules {
		links := append([]string(nil), cr.rule.Links...)
		out = append(out, contract.RuleLink{
			RuleID: cr.rule.ID,
			Links:  links,
		})
	}
	return out
}
