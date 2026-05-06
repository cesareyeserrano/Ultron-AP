package help

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cesareyeserrano/ultron-ap/internal/help/contract"
)

// fakeLogger captures structured log lines for assertions.
type fakeLogger struct {
	lines []string
}

func (f *fakeLogger) Logf() LogFunc {
	return func(format string, args ...interface{}) {
		// Build the substituted line — same as log.Printf would.
		line := substitute(format, args)
		f.lines = append(f.lines, line)
	}
}

func (f *fakeLogger) Count(substr string) int {
	c := 0
	for _, l := range f.lines {
		if strings.Contains(l, substr) {
			c++
		}
	}
	return c
}

func (f *fakeLogger) FindFirst(substr string) string {
	for _, l := range f.lines {
		if strings.Contains(l, substr) {
			return l
		}
	}
	return ""
}

// substitute renders the captured format/args into the same string the
// journal would record.
func substitute(format string, args []interface{}) string {
	return fmt.Sprintf(format, args...)
}

// loadFromString is a small convenience to feed JSON content directly into
// loadFromBytes for unit tests.
func loadFromString(t *testing.T, log LogFunc, json string) []Entry {
	t.Helper()
	out, _, err := loadFromBytes([]byte(json), log)
	if err != nil {
		t.Fatalf("loadFromBytes returned err = %v", err)
	}
	return out
}

// TestTC_HP_049ha — fully valid entry loads with all fields populated.
//
// @aitri-tc TC-HP-049ha
// @aitri-trace FR-049 AC-049-001
func TestTC_HP_049ha(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {
	      "id": "entry-cpu-percent",
	      "title": "CPU %",
	      "category": "system-metrics",
	      "technical": "Percent of CPU time spent non-idle, sampled every 5 s.",
	      "plain": "How busy your Pi has been recently.",
	      "thresholds": [{"label":"warn","value":">=80%","severity":"warn"}],
	      "source_path": "internal/metrics/system.go"
	    }
	  ]
	}`
	log := &fakeLogger{}
	got := loadFromString(t, log.Logf(), fixture)
	if len(got) != 1 {
		t.Fatalf("entry count = %d, want 1", len(got))
	}
	e := got[0]
	if e.ID != "entry-cpu-percent" || e.Title != "CPU %" || e.Category != CategorySystemMetrics {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if !strings.Contains(e.Technical, "Percent of CPU time spent non-idle") {
		t.Fatalf("technical body missing snippet, got %q", e.Technical)
	}
	if !strings.Contains(e.Plain, "How busy your Pi has been") {
		t.Fatalf("plain body missing snippet, got %q", e.Plain)
	}
	if len(e.Thresholds) != 1 || e.Thresholds[0].Label != "warn" || e.Thresholds[0].Value != ">=80%" {
		t.Fatalf("threshold mismatch: %+v", e.Thresholds)
	}
	if e.SourcePath != "internal/metrics/system.go" {
		t.Fatalf("source_path = %q", e.SourcePath)
	}
	if log.Count("glossary-entry-rejected") != 0 {
		t.Fatalf("unexpected rejection: %v", log.lines)
	}
}

// TestTC_HP_049h — entry missing the 'plain' field is rejected; remaining
// valid entries continue to load.
//
// @aitri-tc TC-HP-049h
// @aitri-trace FR-049 AC-049-001
func TestTC_HP_049h(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-a","title":"A","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"entry-b","title":"B","category":"system-metrics","technical":"t"},
	    {"id":"entry-c","title":"C","category":"system-metrics","technical":"t","plain":"p"}
	  ]
	}`
	log := &fakeLogger{}
	got := loadFromString(t, log.Logf(), fixture)
	if len(got) != 2 {
		t.Fatalf("entry count = %d, want 2; got = %+v", len(got), got)
	}
	if log.Count("glossary-entry-rejected") != 1 {
		t.Fatalf("rejection count = %d, want 1; lines = %v", log.Count("glossary-entry-rejected"), log.lines)
	}
	line := log.FindFirst("glossary-entry-rejected")
	if !strings.Contains(line, "entry-b") {
		t.Fatalf("expected log line to name entry-b; got %q", line)
	}
	if !strings.Contains(line, "missing-field=plain") {
		t.Fatalf("expected reason missing-field=plain; got %q", line)
	}
}

// TestTC_HP_049f — entry with category='something-else' is rejected.
//
// @aitri-tc TC-HP-049f
// @aitri-trace FR-049 AC-049-001
func TestTC_HP_049f(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-a","title":"A","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"entry-b","title":"B","category":"something-else","technical":"t","plain":"p"}
	  ]
	}`
	log := &fakeLogger{}
	got := loadFromString(t, log.Logf(), fixture)
	if len(got) != 1 {
		t.Fatalf("entry count = %d, want 1", len(got))
	}
	if log.Count("glossary-entry-rejected") != 1 {
		t.Fatalf("rejection count = %d, want 1", log.Count("glossary-entry-rejected"))
	}
	if !strings.Contains(log.FindFirst("glossary-entry-rejected"), "invalid-category=something-else") {
		t.Fatalf("expected reason 'invalid-category=something-else'; got %q", log.FindFirst("glossary-entry-rejected"))
	}
}

// TestTC_HP_049e — duplicate entry id: only first kept; second logged.
//
// @aitri-tc TC-HP-049e
// @aitri-trace FR-049 AC-049-001
func TestTC_HP_049e(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-cpu-percent","title":"First","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"entry-cpu-percent","title":"Second","category":"system-metrics","technical":"t","plain":"p"}
	  ]
	}`
	log := &fakeLogger{}
	got := loadFromString(t, log.Logf(), fixture)
	if len(got) != 1 {
		t.Fatalf("entry count = %d, want 1", len(got))
	}
	if got[0].Title != "First" {
		t.Fatalf("first kept title = %q, want 'First'", got[0].Title)
	}
	if log.Count("duplicate-entry-id") != 1 {
		t.Fatalf("duplicate log count = %d, want 1", log.Count("duplicate-entry-id"))
	}
}

// TestTC_HP_049f2 — unknown field on entry is rejected.
//
// @aitri-tc TC-HP-049f2
// @aitri-trace FR-049 AC-049-001
func TestTC_HP_049f2(t *testing.T) {
	const fixture = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-a","title":"A","category":"system-metrics","technical":"t","plain":"p","mute_until":"2026-12-31"}
	  ]
	}`
	log := &fakeLogger{}
	got := loadFromString(t, log.Logf(), fixture)
	if len(got) != 0 {
		t.Fatalf("entry count = %d, want 0 (unknown field rejection)", len(got))
	}
	if log.Count("glossary-entry-rejected") != 1 {
		t.Fatalf("rejection count = %d, want 1", log.Count("glossary-entry-rejected"))
	}
	line := log.FindFirst("glossary-entry-rejected")
	if !strings.Contains(line, "unknown field") || !strings.Contains(line, "mute_until") {
		t.Fatalf("expected reason to contain 'unknown field' and 'mute_until'; got %q", line)
	}
}

// TestTC_HP_055f — production glossary loads with zero rejections.
//
// @aitri-tc TC-HP-055f
// @aitri-trace FR-055 AC-055-001
func TestTC_HP_055f(t *testing.T) {
	log := &fakeLogger{}
	svc, err := New(log.Logf())
	if err != nil {
		t.Fatalf("help.New: %v", err)
	}
	if c := log.Count("glossary-entry-rejected"); c != 0 {
		t.Fatalf("production glossary has %d rejections; want 0; lines = %v", c, log.lines)
	}
	if c := log.Count("duplicate-entry-id"); c != 0 {
		t.Fatalf("production glossary has %d duplicates; want 0", c)
	}
	if svc.EntryCount() < 30 {
		t.Fatalf("production glossary has %d entries; want ≥ 30", svc.EntryCount())
	}
}

// TestTC_HP_055h — production glossary boots and emits glossary-loaded with
// entries=N matching EntryCount().
//
// @aitri-tc TC-HP-055h
// @aitri-trace FR-055 AC-055-001
func TestTC_HP_055h(t *testing.T) {
	log := &fakeLogger{}
	svc, err := New(log.Logf())
	if err != nil {
		t.Fatalf("help.New: %v", err)
	}
	if svc.EntryCount() < 30 {
		t.Fatalf("EntryCount = %d, want ≥ 30", svc.EntryCount())
	}
	if log.Count("glossary-loaded") != 1 {
		t.Fatalf("glossary-loaded log count = %d, want 1", log.Count("glossary-loaded"))
	}
	expectedFragment := "entries=" + itoa(svc.EntryCount())
	if !strings.Contains(log.FindFirst("glossary-loaded"), expectedFragment) {
		t.Fatalf("expected log to contain %q; got %q", expectedFragment, log.FindFirst("glossary-loaded"))
	}
}

// TestTC_HP_055h2 — Insights verdicts category contains exactly 10 entries
// (one per bundled rule per insights-engine FR-047).
//
// @aitri-tc TC-HP-055h2
// @aitri-trace FR-055 AC-055-002
func TestTC_HP_055h2(t *testing.T) {
	log := &fakeLogger{}
	svc, err := New(log.Logf())
	if err != nil {
		t.Fatalf("help.New: %v", err)
	}
	verdicts := svc.EntriesByCategory(CategoryInsightsVerdicts)
	if len(verdicts) != 10 {
		t.Fatalf("insights-verdicts count = %d, want 10", len(verdicts))
	}
	// Every verdict id must start with "verdict-" so rule.Links fragments map cleanly.
	for _, e := range verdicts {
		if !strings.HasPrefix(e.ID, "verdict-") {
			t.Fatalf("verdict entry id %q must start with 'verdict-'", e.ID)
		}
	}
}

// TestTC_HP_055e — System-metrics covers CPU, RAM, temperature, disk-percent.
//
// @aitri-tc TC-HP-055e
// @aitri-trace FR-055 AC-055-001
func TestTC_HP_055e(t *testing.T) {
	log := &fakeLogger{}
	svc, err := New(log.Logf())
	if err != nil {
		t.Fatalf("help.New: %v", err)
	}
	sys := svc.EntriesByCategory(CategorySystemMetrics)
	concat := func() string {
		var sb strings.Builder
		for _, e := range sys {
			sb.WriteString(strings.ToLower(e.Title))
			sb.WriteString(" ")
			sb.WriteString(strings.ToLower(e.Technical))
			sb.WriteString(" ")
		}
		return sb.String()
	}
	all := concat()
	for _, want := range []string{"cpu", "ram", "temp", "disk"} {
		if !strings.Contains(all, want) {
			t.Fatalf("system-metrics missing coverage for %q; entries: %d", want, len(sys))
		}
	}
	if !strings.Contains(all, "%") {
		t.Fatalf("system-metrics missing percent coverage; entries: %d", len(sys))
	}
}

// TestTC_HP_NFR_025f — adding a new entry does not change existing entry ids.
//
// @aitri-tc TC-HP-NFR-025f
// @aitri-trace NFR-025 AC-049-001
func TestTC_HP_NFR_025f(t *testing.T) {
	const baseline = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-cpu-percent","title":"CPU %","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"entry-ram-percent","title":"RAM %","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"verdict-thermal-throttling","title":"Thermal","category":"insights-verdicts","technical":"t","plain":"p"}
	  ]
	}`
	const extended = `{
	  "version": 1,
	  "entries": [
	    {"id":"entry-cpu-percent","title":"CPU %","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"entry-ram-percent","title":"RAM %","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"entry-disk-percent","title":"Disk %","category":"system-metrics","technical":"t","plain":"p"},
	    {"id":"verdict-thermal-throttling","title":"Thermal","category":"insights-verdicts","technical":"t","plain":"p"}
	  ]
	}`
	log := &fakeLogger{}
	a := loadFromString(t, log.Logf(), baseline)
	b := loadFromString(t, log.Logf(), extended)
	idsA := idSet(a)
	idsB := idSet(b)
	for k := range idsA {
		if _, ok := idsB[k]; !ok {
			t.Fatalf("baseline id %q is missing from extended set", k)
		}
	}
	diff := []string{}
	for k := range idsB {
		if _, ok := idsA[k]; !ok {
			diff = append(diff, k)
		}
	}
	want := []string{"entry-disk-percent"}
	if !reflect.DeepEqual(diff, want) {
		t.Fatalf("extended-baseline diff = %v, want %v", diff, want)
	}
}

func idSet(es []Entry) map[string]struct{} {
	out := make(map[string]struct{}, len(es))
	for _, e := range es {
		out[e.ID] = struct{}{}
	}
	return out
}

// itoa avoids strconv import in this dense test file.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Compile-time guard — ensure we always have a contract.RuleLink import in
// test reachability so the loader_test file does not lose it accidentally.
var _ = contract.RuleLink{}
