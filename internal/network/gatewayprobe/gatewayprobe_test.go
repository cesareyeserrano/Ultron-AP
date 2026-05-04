package gatewayprobe

import (
	"math"
	"testing"
	"time"
)

func TestDecodeProcGatewayHex(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"0100007F", "127.0.0.1"},
		{"0101A8C0", "192.168.1.1"},
		{"FE01A8C0", "192.168.1.254"},
		{"00000000", "0.0.0.0"},
	}
	for _, tt := range tests {
		got, err := decodeProcGatewayHex(tt.in)
		if err != nil {
			t.Errorf("decodeProcGatewayHex(%q) returned error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("decodeProcGatewayHex(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDecodeProcGatewayHex_BadInput(t *testing.T) {
	cases := []string{"", "ABC", "GGGGGGGG", "0102A8C0FF"}
	for _, c := range cases {
		if _, err := decodeProcGatewayHex(c); err == nil {
			t.Errorf("decodeProcGatewayHex(%q) expected error, got nil", c)
		}
	}
}

func TestStrconvParseUint(t *testing.T) {
	tests := []struct {
		in   string
		want uint64
	}{
		{"0", 0},
		{"3", 3},
		{"A", 10},
		{"FF", 255},
		{"0003", 3},
	}
	for _, tt := range tests {
		got, err := strconvParseUint(tt.in)
		if err != nil {
			t.Errorf("strconvParseUint(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("strconvParseUint(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestNew_DefaultsToGatewayWhenNoTargets(t *testing.T) {
	p := New(0, nil, nil)
	snaps := p.Snapshots()
	if len(snaps) != 1 {
		t.Fatalf("Snapshots() len = %d, want 1", len(snaps))
	}
	if snaps[0].Label != "gateway" {
		t.Errorf("default label = %q, want gateway", snaps[0].Label)
	}
	if snaps[0].Status != StatusInit {
		t.Errorf("initial status = %q, want %q", snaps[0].Status, StatusInit)
	}
}

func TestNew_PreservesTargetOrder(t *testing.T) {
	targets := []Target{
		{Label: "gateway"},
		{Label: "cloudflare", Host: "1.1.1.1"},
		{Label: "google", Host: "8.8.8.8"},
	}
	p := New(time.Second, nil, targets)
	snaps := p.Snapshots()
	if len(snaps) != 3 {
		t.Fatalf("Snapshots() len = %d, want 3", len(snaps))
	}
	for i, want := range []string{"gateway", "cloudflare", "google"} {
		if snaps[i].Label != want {
			t.Errorf("snaps[%d].Label = %q, want %q", i, snaps[i].Label, want)
		}
	}
}

func TestNew_NilSinkAccepted(t *testing.T) {
	// Nil sink must not panic — used in tests and in setups without persistence.
	_ = New(time.Second, nil, []Target{{Label: "gw"}})
}

// --- jitter / loss math ---

func TestRecordSuccess_FirstSampleHasZeroJitter(t *testing.T) {
	st := &targetState{cfg: Target{Label: "gw"}}
	snap := &Snapshot{}
	st.recordSuccess(snap, 10.0)
	if snap.JitterMs != 0 {
		t.Errorf("first sample jitter = %v, want 0", snap.JitterMs)
	}
	if snap.LossPct != 0 {
		t.Errorf("first sample loss = %v, want 0", snap.LossPct)
	}
}

func TestRecordSuccess_JitterEWMAFormula(t *testing.T) {
	st := &targetState{cfg: Target{Label: "gw"}}
	// Seed prev=10ms, then a 14ms sample: |Δ|=4. EWMA = α*4 + (1-α)*0 = 0.2*4 = 0.8.
	st.recordSuccess(&Snapshot{}, 10.0)
	snap := &Snapshot{}
	st.recordSuccess(snap, 14.0)
	want := 0.2 * 4.0
	if math.Abs(snap.JitterMs-want) > 1e-9 {
		t.Errorf("jitter = %v, want %v", snap.JitterMs, want)
	}
	// Third 14ms sample: |Δ|=0. EWMA = 0.2*0 + 0.8*0.8 = 0.64.
	snap2 := &Snapshot{}
	st.recordSuccess(snap2, 14.0)
	want2 := 0.8 * want
	if math.Abs(snap2.JitterMs-want2) > 1e-9 {
		t.Errorf("jitter after no-change = %v, want %v", snap2.JitterMs, want2)
	}
}

func TestRecordFailure_FlowsIntoLossPct(t *testing.T) {
	st := &targetState{cfg: Target{Label: "gw"}}
	// 3 success then 2 failure → 2/5 = 40%.
	for i := 0; i < 3; i++ {
		st.recordSuccess(&Snapshot{}, 10.0)
	}
	snap := &Snapshot{}
	st.recordFailure(snap)
	st.recordFailure(snap)
	if math.Abs(snap.LossPct-40.0) > 1e-9 {
		t.Errorf("loss = %v, want 40", snap.LossPct)
	}
}

func TestHistoryWindow_RingBufferSlides(t *testing.T) {
	st := &targetState{cfg: Target{Label: "gw"}}
	// historyWindow successes then 1 failure → loss = 1/historyWindow * 100.
	for i := 0; i < historyWindow; i++ {
		st.recordSuccess(&Snapshot{}, 10.0)
	}
	snap := &Snapshot{}
	st.recordFailure(snap)
	want := 1.0 / float64(historyWindow) * 100.0
	if math.Abs(snap.LossPct-want) > 1e-9 {
		t.Errorf("loss after one failure = %v, want %v", snap.LossPct, want)
	}
	// Now add historyWindow more failures — the original successes should
	// have been evicted, loss should approach 100%.
	for i := 0; i < historyWindow-1; i++ {
		st.recordFailure(snap)
	}
	if math.Abs(snap.LossPct-100.0) > 1e-9 {
		t.Errorf("loss after window of failures = %v, want 100", snap.LossPct)
	}
}
