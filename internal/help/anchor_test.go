package help

import (
	"strings"
	"testing"
)

const fxThermal = `{
  "version": 1,
  "entries": [
    {"id":"verdict-thermal-throttling","title":"Thermal","category":"insights-verdicts","technical":"t","plain":"p"}
  ]
}`

// TestTC_HP_053h — single valid fragment returns "/help#<id>", true.
//
// @aitri-tc TC-HP-053h
// @aitri-trace FR-053 AC-053-001
func TestTC_HP_053h(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxThermal)
	href, ok := svc.FirstValidAnchor([]string{"#verdict-thermal-throttling"})
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if href != "/help#verdict-thermal-throttling" {
		t.Fatalf("href = %q, want /help#verdict-thermal-throttling", href)
	}
}

// TestTC_HP_053e — first fragment missing, second valid → returns the second.
//
// @aitri-tc TC-HP-053e
// @aitri-trace FR-053 AC-053-001
func TestTC_HP_053e(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxThermal)
	href, ok := svc.FirstValidAnchor([]string{"#verdict-foo", "#verdict-thermal-throttling"})
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if href != "/help#verdict-thermal-throttling" {
		t.Fatalf("href = %q, want /help#verdict-thermal-throttling", href)
	}
}

// TestTC_HP_053f — empty links and all-missing both yield ok=false; rendered
// fragment from sse-verdicts must omit the Learn-more anchor (asserted at the
// server layer in handlers_help_test.go; the function-level test ensures the
// AnchorResolver returns false in both cases).
//
// @aitri-tc TC-HP-053f
// @aitri-trace FR-053 AC-053-001
func TestTC_HP_053f(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxThermal)
	if href, ok := svc.FirstValidAnchor([]string{}); ok || href != "" {
		t.Fatalf("empty links: got (%q, %v), want (\"\", false)", href, ok)
	}
	if href, ok := svc.FirstValidAnchor([]string{"#verdict-foo"}); ok || href != "" {
		t.Fatalf("all-missing: got (%q, %v), want (\"\", false)", href, ok)
	}
	// External-only links also yield false (no fragments to consider).
	if href, ok := svc.FirstValidAnchor([]string{"https://example.com/foo"}); ok || href != "" {
		t.Fatalf("external-only: got (%q, %v), want (\"\", false)", href, ok)
	}
}

// TestFirstValidAnchor_ignoresMalformed — defence-in-depth: garbage links
// (no leading #, empty strings) are skipped without error.
func TestFirstValidAnchor_ignoresMalformed(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxThermal)
	href, ok := svc.FirstValidAnchor([]string{"", "no-hash", "://broken", "#verdict-thermal-throttling"})
	if !ok || !strings.HasSuffix(href, "verdict-thermal-throttling") {
		t.Fatalf("got (%q, %v), want suffix verdict-thermal-throttling, ok=true", href, ok)
	}
}
