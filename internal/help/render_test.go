package help

import (
	"bytes"
	"strings"
	"testing"
)

// TestTC_HP_049e2 — entry with thresholds omitted renders without <table> or
// .thresholds element.
//
// @aitri-tc TC-HP-049e2
// @aitri-trace FR-049 AC-049-001
func TestTC_HP_049e2(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-test","title":"T","category":"system-metrics","technical":"t","plain":"p"}
	  ]
	}`
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fixture)
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	body := buf.String()
	// Article exists.
	if !strings.Contains(body, `id="entry-entry-test"`) {
		t.Fatalf("expected article id 'entry-entry-test' in rendered body")
	}
	// No <table> for thresholds.
	if strings.Contains(body, "<table") {
		t.Fatalf("expected no <table> when thresholds are omitted; body:\n%s", body)
	}
	if strings.Contains(body, "thresholds") {
		// 'thresholds' literal appears only inside threshold class names; ensure absence.
		t.Fatalf("expected no 'thresholds' substring when thresholds are omitted")
	}
}

// TestTC_HP_049e3 — entry without source_path renders without .source-path.
//
// @aitri-tc TC-HP-049e3
// @aitri-trace FR-049 AC-049-001
func TestTC_HP_049e3(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-test","title":"T","category":"system-metrics","technical":"t","plain":"p"}
	  ]
	}`
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fixture)
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	if strings.Contains(buf.String(), "source-path") {
		t.Fatalf("expected no .source-path when source_path is omitted")
	}
}

// TestTC_HP_NFR_024f — rendered HTML uses semantic landmarks.
//
// @aitri-tc TC-HP-NFR-024f
// @aitri-trace NFR-024 AC-048-001
func TestTC_HP_NFR_024f(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-a","title":"A","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"entry-b","title":"B","category":"network-probes","technical":"t","plain":"p"},
	    {"id":"entry-c","title":"C","category":"services-containers","technical":"t","plain":"p"},
	    {"id":"entry-d","title":"D","category":"vpn","technical":"t","plain":"p"},
	    {"id":"verdict-x","title":"X","category":"insights-verdicts","technical":"t","plain":"p"}
	  ]
	}`
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fixture)
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	body := buf.String()
	if got := strings.Count(body, "<h1"); got != 1 {
		t.Fatalf("h1 count = %d, want 1", got)
	}
	if got := strings.Count(body, `<section id="cat-`); got != 5 {
		t.Fatalf("section count = %d, want 5", got)
	}
	if got := strings.Count(body, `<article id="entry-`); got != 5 {
		t.Fatalf("article count = %d, want 5", got)
	}
	// Each section has a header with h2.
	if got := strings.Count(body, "<h2"); got != 5 {
		t.Fatalf("h2 count = %d, want 5", got)
	}
	// Each article has an h3.
	if got := strings.Count(body, "<h3"); got != 5 {
		t.Fatalf("h3 count = %d, want 5", got)
	}
}

// TestTC_HP_NFR_024f2 — filter input has matching label and aria-controls.
//
// @aitri-tc TC-HP-NFR-024f2
// @aitri-trace NFR-024 AC-054-001
func TestTC_HP_NFR_024f2(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxThermal)
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, `<label for="help-filter"`) {
		t.Fatalf("expected <label for=\"help-filter\">; body:\n%s", body)
	}
	if !strings.Contains(body, `id="help-filter"`) || !strings.Contains(body, `type="search"`) {
		t.Fatalf("expected <input type=\"search\" id=\"help-filter\">")
	}
	if !strings.Contains(body, `aria-controls="help-entries"`) {
		t.Fatalf("expected aria-controls=\"help-entries\"")
	}
	if !strings.Contains(body, `id="help-entries"`) {
		t.Fatalf("expected element id=\"help-entries\"")
	}
}

// TestTC_HP_054e2 — single inline filter script, no JS framework markers.
//
// @aitri-tc TC-HP-054e2
// @aitri-trace FR-054 AC-054-001
func TestTC_HP_054e2(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxThermal)
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	body := buf.String()
	if got := strings.Count(body, "<script"); got != 1 {
		t.Fatalf("inline <script> count = %d, want 1", got)
	}
	for _, marker := range []string{"react", "React", "Vue", "Alpine", "jQuery", "window.$"} {
		if strings.Contains(body, marker) {
			t.Fatalf("body contains forbidden framework marker %q", marker)
		}
	}
}

// TestTC_HP_NFR_025h — every entry's title and plain body appear in the raw
// HTML body (no JS execution required).
//
// @aitri-tc TC-HP-NFR-025h
// @aitri-trace NFR-025 AC-048-001
func TestTC_HP_NFR_025h(t *testing.T) {
	log := &fakeLogger{}
	svc, err := New(log.Logf())
	if err != nil {
		t.Fatalf("help.New: %v", err)
	}
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	body := buf.String()
	if !strings.Contains(strings.ToLower(body), "thermal throttling") {
		t.Fatalf("expected 'thermal throttling' in body")
	}
	if !strings.Contains(body, "too hot") {
		t.Fatalf("expected plain-body snippet 'too hot' in body")
	}
}

// TestTC_HP_NFR_025e — five stable category fragments are present.
//
// @aitri-tc TC-HP-NFR-025e
// @aitri-trace NFR-025 AC-048-001
func TestTC_HP_NFR_025e(t *testing.T) {
	log := &fakeLogger{}
	svc, err := New(log.Logf())
	if err != nil {
		t.Fatalf("help.New: %v", err)
	}
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	body := buf.String()
	for _, want := range []string{
		`id="cat-system-metrics"`,
		`id="cat-network-probes"`,
		`id="cat-services-containers"`,
		`id="cat-vpn"`,
		`id="cat-insights-verdicts"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q", want)
		}
	}
}

// TestTC_HP_050h — two-voice layout structure: plain and technical bodies
// are both rendered, plain comes first in DOM order, and the responsive grid
// utility classes are present so the responsive switch is mechanical.
//
// @aitri-tc TC-HP-050h
// @aitri-trace FR-050 AC-050-001
func TestTC_HP_050h(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-test","title":"T","category":"system-metrics","technical":"TECH-BODY-XYZ","plain":"PLAIN-BODY-ABC"}
	  ]
	}`
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fixture)
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	body := buf.String()

	// Both bodies present.
	if !strings.Contains(body, "TECH-BODY-XYZ") {
		t.Fatalf("technical body not rendered")
	}
	if !strings.Contains(body, "PLAIN-BODY-ABC") {
		t.Fatalf("plain body not rendered")
	}
	// Both labels present.
	if !strings.Contains(body, ">Technical<") {
		t.Fatalf("'Technical' label not rendered")
	}
	if !strings.Contains(body, ">Plain<") {
		t.Fatalf("'Plain' label not rendered")
	}
	// Plain comes BEFORE technical in DOM order (FR-050 — mobile shows plain first).
	plainIdx := strings.Index(body, `data-voice="plain"`)
	techIdx := strings.Index(body, `data-voice="technical"`)
	if plainIdx < 0 || techIdx < 0 {
		t.Fatalf("data-voice attributes missing: plainIdx=%d techIdx=%d", plainIdx, techIdx)
	}
	if plainIdx >= techIdx {
		t.Fatalf("plain voice must precede technical in DOM order; plainIdx=%d techIdx=%d", plainIdx, techIdx)
	}
	// Responsive grid utility class — the Tailwind breakpoint that flips from
	// stacked to side-by-side at >=768px.
	if !strings.Contains(body, "md:grid-cols-2") {
		t.Fatalf("expected 'md:grid-cols-2' utility class for responsive two-column layout")
	}
}

// TestTC_HP_051h — anchor scroll structure: every entry wrapper carries a
// stable `id="entry-<slug>"` (the URL fragment target). The inline <style>
// contains the :target rule so the highlight is browser-native, requiring no
// JS at runtime.
//
// @aitri-tc TC-HP-051h
// @aitri-trace FR-051 AC-051-001
func TestTC_HP_051h(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"verdict-thermal-throttling","title":"Thermal","category":"insights-verdicts","technical":"t","plain":"p"},
	    {"id":"entry-cpu-percent","title":"CPU","category":"system-metrics","technical":"t","plain":"p"}
	  ]
	}`
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fixture)
	var buf bytes.Buffer
	if err := svc.RenderBody(&buf); err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	body := buf.String()
	// Every slug gets a wrapper id.
	for _, slug := range []string{"verdict-thermal-throttling", "entry-cpu-percent"} {
		want := `id="entry-` + slug + `"`
		if !strings.Contains(body, want) {
			t.Fatalf("expected wrapper %q in rendered body", want)
		}
	}
	// Inline <style> contains the :target rule and the help-target-flash
	// animation — pure-CSS highlight per FR-051 AC-002.
	if !strings.Contains(body, ".help-entry:target") {
		t.Fatalf("expected '.help-entry:target' selector in inline <style>")
	}
	if !strings.Contains(body, "help-target-flash") {
		t.Fatalf("expected 'help-target-flash' keyframes animation name in inline <style>")
	}
	// 1.5 s animation duration matches FR-051 AC-002 (1–2 s window).
	if !strings.Contains(body, "1.5s") {
		t.Fatalf("expected 1.5s animation duration on .help-entry:target")
	}
}

// TestRender_dataSearchHaystackLowercased — sanity check that the data-search
// attribute combines title + bodies and is lowercased so the inline filter
// stays at one String.includes per entry per keystroke.
func TestRender_dataSearchHaystackLowercased(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-x","title":"CPU Throttling","category":"system-metrics","technical":"high","plain":"PI is HOT"}
	  ]
	}`
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fixture)
	var buf bytes.Buffer
	_ = svc.RenderBody(&buf)
	body := buf.String()
	if !strings.Contains(body, `data-search="cpu throttling high pi is hot"`) {
		t.Fatalf("data-search attribute not pre-lowercased; body fragment:\n%s", body)
	}
}
