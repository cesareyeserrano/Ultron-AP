package server

import (
	"strings"
	"testing"

	"github.com/cesareyeserrano/ultron-ap/internal/notify"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/render"
)

// TestTC_TMU_026h covers FR-026 / AC-026-001 — the synthetic Test
// notification event built by the settings-page handler is a CPU-fire whose
// rendered Telegram body begins with the "TEST — " prefix and exercises
// every resource-surface renderer branch (severity emoji, friendly label,
// threshold clause, deep-link footer).
//
// @aitri-trace FR-026 US-026 AC-026-001 TC-TMU-026h
func TestTC_TMU_026h_TestNotificationEventRendersAsTestPrefixedFire(t *testing.T) {
	evt := buildTestNotificationEvent()

	if evt.Kind != notify.EventFire {
		t.Fatalf("Kind = %q; want EventFire", evt.Kind)
	}
	if evt.Surface != notify.SurfaceResource {
		t.Fatalf("Surface = %q; want SurfaceResource", evt.Surface)
	}
	if evt.Alert == nil || evt.Alert.Severity != "critical" {
		t.Fatalf("Alert.Severity = %q; want 'critical'", evt.Alert.Severity)
	}
	if !strings.HasPrefix(evt.Alert.Message, "TEST —") {
		t.Fatalf("Alert.Message = %q; want a 'TEST —' prefix", evt.Alert.Message)
	}
	if evt.Rule == nil || evt.Rule.Operator != ">" || evt.Rule.Threshold != 80.0 {
		t.Fatalf("Rule = %+v; want operator '>' and threshold 80.0", evt.Rule)
	}
	if evt.FirstFiredAt.IsZero() {
		t.Fatalf("FirstFiredAt is zero; want a non-zero timestamp so the renderer takes the elapsed branch")
	}

	in := render.Input{
		Alert:        evt.Alert,
		Rule:         evt.Rule,
		Kind:         render.KindFire,
		Surface:      render.SurfaceResource,
		FirstFiredAt: evt.FirstFiredAt,
		Hostname:     "test-host",
		PublicURL:    "https://example.com",
	}
	out := render.Render(in)

	body := out.TelegramMD
	for _, want := range []string{
		"🔴",
		"CPU usage critical",
		"ALERT FIRED",
		"CPU 92%",
		"threshold > 80",
		"[Open dashboard]",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body missing %q; got:\n%s", want, body)
		}
	}
}

// TestTC_TMU_026e covers FR-026 / AC-026-002 — the rendered Test message
// for resource alerts contains the elapsed-since-breach phrase, proving the
// renderer takes the FR-019 elapsed branch (FirstFiredAt was set in the
// past by buildTestNotificationEvent).
//
// @aitri-trace FR-026 US-026 AC-026-002 TC-TMU-026e
func TestTC_TMU_026e_TestNotificationContainsElapsedAndFooter(t *testing.T) {
	evt := buildTestNotificationEvent()
	in := render.Input{
		Alert:        evt.Alert,
		Rule:         evt.Rule,
		Kind:         render.KindFire,
		Surface:      render.SurfaceResource,
		FirstFiredAt: evt.FirstFiredAt,
		Hostname:     "ultron",
		PublicURL:    "https://example.com",
	}
	out := render.Render(in)

	if !strings.Contains(out.TelegramMD, "for ") {
		t.Errorf("Telegram body missing 'for <duration>'; got:\n%s", out.TelegramMD)
	}
	if !strings.Contains(out.TelegramMD, "[Open dashboard](https://example.com/alerts)") {
		t.Errorf("Telegram body missing footer; got:\n%s", out.TelegramMD)
	}
	if !strings.Contains(out.EmailHTML, "<a href=\"https://example.com/alerts\">Open dashboard</a>") {
		t.Errorf("Email HTML missing footer link; got:\n%s", out.EmailHTML)
	}
}

// TestTC_TMU_026f covers FR-026 — the test message exercises a resource
// surface, NOT systemd or docker; the renderer must therefore not include
// surface-specific markers like 'journal' or 'exit code' in the body.
//
// @aitri-trace FR-026 US-026 AC-026-001 TC-TMU-026f
func TestTC_TMU_026f_TestNotificationOmitsSystemdAndDockerBlocks(t *testing.T) {
	evt := buildTestNotificationEvent()
	in := render.Input{
		Alert:        evt.Alert,
		Rule:         evt.Rule,
		Kind:         render.KindFire,
		Surface:      render.SurfaceResource,
		FirstFiredAt: evt.FirstFiredAt,
		Hostname:     "ultron",
		PublicURL:    "https://example.com",
	}
	out := render.Render(in)

	for _, forbidden := range []string{
		"journal unavailable",
		"docker logs unavailable",
		"exit code",
		"active since",
	} {
		if strings.Contains(out.TelegramMD, forbidden) {
			t.Errorf("test message unexpectedly contains %q (resource-only event):\n%s", forbidden, out.TelegramMD)
		}
	}
}
