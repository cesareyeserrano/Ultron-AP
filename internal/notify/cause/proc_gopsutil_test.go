package cause

import (
	"context"
	"testing"
	"time"
)

// TestTC_TMU_029_SamplerStartReturnsNonEmpty covers FR-029 wiring:
// after Start() the sampler has primed itself with the host's process
// list and TopProcesses returns at least one process (this Go test
// runner itself is a process).
//
// @aitri-trace FR-029 NFR-005 TC-TMU-029-sampler
func TestTC_TMU_029_SamplerStartReturnsNonEmpty(t *testing.T) {
	s := NewProcessSampler()
	s.Start()
	defer s.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	got, err := s.TopProcesses(ctx, AxisRSS, 5)
	if err != nil {
		t.Fatalf("TopProcesses err: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("TopProcesses returned 0 processes; want ≥1 after Start()")
	}
	for _, p := range got {
		if p.Comm == "" {
			t.Errorf("ProcSample.Comm is empty: %+v", p)
		}
	}
}

// TestTC_TMU_029_SamplerCtxCancelled confirms ctx cancellation is
// honored — TopProcesses must return ctx.Err() rather than blocking.
//
// @aitri-trace FR-029 NFR-005 TC-TMU-029-sampler-ctx
func TestTC_TMU_029_SamplerCtxCancelled(t *testing.T) {
	s := NewProcessSampler()
	s.Start()
	defer s.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.TopProcesses(ctx, AxisCPU, 1); err == nil {
		t.Fatalf("err = nil; want context.Canceled")
	}
}
