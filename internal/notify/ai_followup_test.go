package notify

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cesareyeserrano/ultron-ap/internal/ai"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

type stubExplainer struct {
	exp *ai.Explanation
	err error
}

func (s stubExplainer) Explain(ctx context.Context, scope ai.Scope) (*ai.Explanation, error) {
	return s.exp, s.err
}

type recSender struct {
	mu   sync.Mutex
	msgs []string
}

func (r *recSender) send(ctx context.Context, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, text)
	return nil
}

func (r *recSender) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.msgs)
}

func fireEvent() *Event {
	return &Event{
		Kind:  EventFire,
		Alert: &database.Alert{Source: "cpu", Severity: "critical", Message: "High CPU"},
		Rule:  &database.AlertConfig{Metric: "cpu"},
	}
}

// @aitri-tc TC-AI-026h
//
// @aitri-trace FR-ID: FR-026, US-ID: US-026, AC-ID: AC-026-1h, TC-ID: TC-AI-026h
func TestTC_AI_026h_AlertThenAIFollowup(t *testing.T) {
	rec := &recSender{}
	// The rule-based alert is delivered first on its own path.
	_ = rec.send(context.Background(), "🧮🔴 CPU usage critical on host")

	exp := &ai.Explanation{Cause: "build container spiked load", Remediation: "throttle it", CitedSignals: []string{"cpu_temp"}}
	f := NewAIFollowup(stubExplainer{exp: exp}, func() bool { return true }, rec.send, 0)
	f.Run(fireEvent())

	if rec.count() != 2 {
		t.Fatalf("message count = %d; want 2 (alert + AI follow-up)", rec.count())
	}
	if !strings.HasPrefix(rec.msgs[1], "🤖 AI analysis") {
		t.Fatalf("second message is not the AI follow-up: %q", rec.msgs[1])
	}
	if !strings.Contains(rec.msgs[1], "build container spiked load") {
		t.Fatalf("AI message missing cause: %q", rec.msgs[1])
	}
}

// @aitri-tc TC-AI-026e
//
// @aitri-trace FR-ID: FR-026, US-ID: US-026, AC-ID: AC-026-1h, TC-ID: TC-AI-026e
func TestTC_AI_026e_PushOffSendsOnlyAlert(t *testing.T) {
	rec := &recSender{}
	_ = rec.send(context.Background(), "🧮🔴 CPU usage critical on host")

	exp := &ai.Explanation{Cause: "x"}
	f := NewAIFollowup(stubExplainer{exp: exp}, func() bool { return false }, rec.send, 0)
	f.Run(fireEvent())

	if rec.count() != 1 {
		t.Fatalf("message count = %d; want 1 (rule-based only, push off)", rec.count())
	}
	if strings.HasPrefix(rec.msgs[0], "🤖") {
		t.Fatalf("unexpected AI message sent with push off")
	}
}

// @aitri-tc TC-AI-026f
//
// @aitri-trace FR-ID: FR-026, US-ID: US-026, AC-ID: AC-026-1f, TC-ID: TC-AI-026f
func TestTC_AI_026f_AIFailureKeepsAlert(t *testing.T) {
	rec := &recSender{}
	_ = rec.send(context.Background(), "🧮🔴 CPU usage critical on host")

	f := NewAIFollowup(stubExplainer{err: errors.New("provider timeout")}, func() bool { return true }, rec.send, 0)
	f.logf = func(string, ...any) {} // silence
	f.Run(fireEvent())

	if rec.count() != 1 {
		t.Fatalf("message count = %d; want exactly 1 (rule-based delivered, AI failed, no duplicate)", rec.count())
	}
}
