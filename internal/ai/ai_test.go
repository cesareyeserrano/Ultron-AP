package ai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/insights"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
)

// --- test doubles -----------------------------------------------------------

type stubDoer struct {
	status    int
	body      string
	sleep     time.Duration
	lastModel string
	lastBody  string
}

func (s *stubDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		s.lastBody = string(b)
		if strings.Contains(s.lastBody, `"model":"model-b"`) {
			s.lastModel = "model-b"
		} else if strings.Contains(s.lastBody, `"model":"model-a"`) {
			s.lastModel = "model-a"
		}
	}
	if s.sleep > 0 {
		select {
		case <-time.After(s.sleep):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}, nil
}

type fakeInsights struct{ verdicts []insights.Verdict }

func (f fakeInsights) Active() []insights.Verdict { return f.verdicts }

type fakeMetrics struct{ snap *metrics.Snapshot }

func (f fakeMetrics) Latest() *metrics.Snapshot { return f.snap }

func okBody(content string) string {
	return `{"choices":[{"message":{"role":"assistant","content":` + jsonQuote(content) + `}}]}`
}

func jsonQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

func cfgFunc(c Config) ConfigFunc { return func() (Config, error) { return c, nil } }

// --- TC-AI-016e -------------------------------------------------------------

// @aitri-tc TC-AI-016e
func TestTC_AI_016e_PromptCaps(t *testing.T) {
	verdicts := make([]insights.Verdict, 10000)
	for i := range verdicts {
		verdicts[i] = insights.Verdict{RuleID: "r", Severity: "warn", VerdictText: strings.Repeat("x", 50)}
	}
	snap := TelemetrySnapshot{Verdicts: verdicts}
	out := buildPrompt(snap, Scope{Kind: ScopeSystem}, nil)

	verdictLines := strings.Count(out, "rule_id=")
	if verdictLines > maxVerdicts {
		t.Fatalf("verdict lines = %d; want <= %d", verdictLines, maxVerdicts)
	}
	if len(out) > maxPromptBytes {
		t.Fatalf("prompt bytes = %d; want <= %d", len(out), maxPromptBytes)
	}
}

// --- TC-AI-019e -------------------------------------------------------------

// @aitri-tc TC-AI-019e
func TestTC_AI_019e_DisabledServiceNotConfigured(t *testing.T) {
	svc := New(cfgFunc(Config{Enabled: false}), Sources{}, &stubDoer{status: 200, body: okBody("x")}, nil)
	if svc.Enabled() {
		t.Fatal("Enabled() = true; want false when disabled")
	}
	_, err := svc.Explain(context.Background(), Scope{Kind: ScopeSystem})
	if err != ErrNotConfigured {
		t.Fatalf("Explain err = %v; want ErrNotConfigured", err)
	}
}

// --- TC-AI-020e -------------------------------------------------------------

// @aitri-tc TC-AI-020e
func TestTC_AI_020e_BuildRequestForAnyBaseURL(t *testing.T) {
	for _, base := range []string{"https://a.test/v1", "https://b.test/v1"} {
		cfg := Config{EndpointURL: base, Model: "m", APIKey: "k"}
		req, err := buildRequest(context.Background(), cfg, "sys", "user")
		if err != nil {
			t.Fatalf("buildRequest(%s) err: %v", base, err)
		}
		if got, want := req.URL.String(), base+"/chat/completions"; got != want {
			t.Fatalf("url = %q; want %q", got, want)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer k" {
			t.Fatalf("auth header = %q; want Bearer k", got)
		}
		b, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(b), `"model":"m"`) || !strings.Contains(string(b), `"messages"`) {
			t.Fatalf("body missing model/messages: %s", b)
		}
	}
}

// --- TC-AI-021e -------------------------------------------------------------

// @aitri-tc TC-AI-021e
func TestTC_AI_021e_TimeoutBoundary(t *testing.T) {
	cfg := Config{Enabled: true, EndpointURL: "https://x/v1", Model: "m", APIKey: "k", TimeoutMS: 1000}
	src := Sources{Insights: fakeInsights{verdicts: []insights.Verdict{{RuleID: "1", VerdictText: "busy"}}}}
	svc := New(cfgFunc(cfg), src, &stubDoer{status: 200, body: okBody("late"), sleep: 3 * time.Second}, nil)

	start := time.Now()
	_, err := svc.Explain(context.Background(), Scope{Kind: ScopeSystem})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed > 1300*time.Millisecond {
		t.Fatalf("elapsed = %v; want abort near 1s (<=1.3s), not 3s", elapsed)
	}
}

// --- TC-AI-023h -------------------------------------------------------------

// @aitri-tc TC-AI-023h
func TestTC_AI_023h_RedactKnownSecret(t *testing.T) {
	secret := "bot12345:AAblahblah"
	in := "sent via token " + secret
	out := Scrub(in, []string{secret})
	if strings.Contains(out, secret) {
		t.Fatalf("secret leaked: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in %q", out)
	}
}

// --- TC-AI-023e -------------------------------------------------------------

// @aitri-tc TC-AI-023e
func TestTC_AI_023e_RedactSecretShapedToken(t *testing.T) {
	in := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc.def"
	out := Scrub(in, nil)
	if strings.Contains(out, "eyJhbGciOi") {
		t.Fatalf("jwt not redacted: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in %q", out)
	}
}
