package help

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/help/contract"
)

// minimalSvc constructs a help.Service from a tiny inline glossary so tests
// don't depend on the embedded production fixture.
func minimalSvc(t *testing.T, log LogFunc, json string) *Service {
	t.Helper()
	entries, raw, err := loadFromBytes([]byte(json), log)
	if err != nil {
		t.Fatalf("loadFromBytes: %v", err)
	}
	svc := &Service{
		log:     log,
		entries: entries,
		byID:    make(map[string]int, len(entries)),
		byCat:   make(map[Category][]int),
		etag:    etagFromBytes(raw),
	}
	for i, e := range entries {
		svc.byID[e.ID] = i
		svc.byCat[e.Category] = append(svc.byCat[e.Category], i)
	}
	if err := svc.compileTemplate(); err != nil {
		t.Fatalf("compileTemplate: %v", err)
	}
	return svc
}

const fxTwoEntries = `{
  "version": 1,
  "entries": [
    {"id":"entry-a","title":"A","category":"system-metrics","technical":"t","plain":"p"},
    {"id":"entry-b","title":"B","category":"system-metrics","technical":"t","plain":"p"}
  ]
}`

// TestTC_HP_052h — all rule links resolve; zero WARN lines.
//
// @aitri-tc TC-HP-052h
// @aitri-trace FR-052 AC-052-002
func TestTC_HP_052h(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxTwoEntries)
	rules := []contract.RuleLink{
		{RuleID: "r1", Links: []string{"#entry-a"}},
		{RuleID: "r2", Links: []string{"#entry-b", "#entry-a"}},
	}
	svc.ValidateLinks(rules)
	if c := log.Count("insights-link-missing"); c != 0 {
		t.Fatalf("missing log count = %d, want 0; lines = %v", c, log.lines)
	}
	if c := log.Count("links-validator-deferred"); c != 0 {
		t.Fatalf("deferred count = %d, want 0", c)
	}
}

// TestTC_HP_052f — one missing fragment yields exactly one WARN with rule id
// and missing anchor; rule slice is not mutated.
//
// @aitri-tc TC-HP-052f
// @aitri-trace FR-052 AC-052-002
func TestTC_HP_052f(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxTwoEntries)
	rules := []contract.RuleLink{
		{RuleID: "thermal", Links: []string{"#verdict-foo"}},
	}
	pre := deepCopyRules(rules)
	svc.ValidateLinks(rules)
	if c := log.Count("insights-link-missing"); c != 1 {
		t.Fatalf("missing log count = %d, want 1", c)
	}
	line := log.FindFirst("insights-link-missing")
	if !strings.Contains(line, `rule_id="thermal"`) {
		t.Fatalf("expected rule_id=\"thermal\" in %q", line)
	}
	if !strings.Contains(line, `missing_anchor="verdict-foo"`) {
		t.Fatalf("expected missing_anchor=\"verdict-foo\" in %q", line)
	}
	if !reflect.DeepEqual(rules, pre) {
		t.Fatalf("validator mutated input: pre=%v post=%v", pre, rules)
	}
}

// TestTC_HP_052e — external http/https links are skipped; only fragments are
// validated.
//
// @aitri-tc TC-HP-052e
// @aitri-trace FR-052 AC-052-002
func TestTC_HP_052e(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxTwoEntries)
	rules := []contract.RuleLink{
		{RuleID: "r1", Links: []string{"https://example.com/foo", "http://docs/x", "#entry-a"}},
	}
	svc.ValidateLinks(rules)
	if c := log.Count("insights-link-missing"); c != 0 {
		t.Fatalf("external links should be skipped; count = %d, want 0", c)
	}
}

// TestTC_HP_052e2 — RuleLink type lives in internal/help/contract with
// exactly two plain fields and zero methods.
//
// @aitri-tc TC-HP-052e2
// @aitri-trace FR-052 NFR-026
func TestTC_HP_052e2(t *testing.T) {
	rt := reflect.TypeOf(contract.RuleLink{})
	if pkg := rt.PkgPath(); pkg != "github.com/cesareyeserrano/ultron-ap/internal/help/contract" {
		t.Fatalf("RuleLink PkgPath = %q, want internal/help/contract", pkg)
	}
	if rt.NumField() != 2 {
		t.Fatalf("RuleLink NumField = %d, want 2", rt.NumField())
	}
	f0 := rt.Field(0)
	if f0.Name != "RuleID" || f0.Type.Kind() != reflect.String {
		t.Fatalf("Field 0 = %s %s, want RuleID string", f0.Name, f0.Type)
	}
	f1 := rt.Field(1)
	if f1.Name != "Links" || f1.Type.Kind() != reflect.Slice || f1.Type.Elem().Kind() != reflect.String {
		t.Fatalf("Field 1 = %s %s, want Links []string", f1.Name, f1.Type)
	}
	if rt.NumMethod() != 0 {
		t.Fatalf("RuleLink methods = %d, want 0", rt.NumMethod())
	}
	if reflect.PointerTo(rt).NumMethod() != 0 {
		t.Fatalf("*RuleLink methods = %d, want 0", reflect.PointerTo(rt).NumMethod())
	}
}

// TestTC_HP_052e3 — validator scales to 100 rules × 10 missing fragments.
//
// @aitri-tc TC-HP-052e3
// @aitri-trace FR-052 AC-052-002
func TestTC_HP_052e3(t *testing.T) {
	log := &fakeLogger{}
	svc := minimalSvc(t, log.Logf(), fxTwoEntries)
	rules := make([]contract.RuleLink, 0, 100)
	for i := 0; i < 100; i++ {
		links := make([]string, 0, 10)
		for j := 0; j < 10; j++ {
			links = append(links, "#missing-"+itoa(i)+"-"+itoa(j))
		}
		rules = append(rules, contract.RuleLink{RuleID: "rule-" + itoa(i), Links: links})
	}
	t0 := time.Now()
	svc.ValidateLinks(rules)
	d := time.Since(t0)
	if d > 50*time.Millisecond {
		t.Fatalf("validator took %v, want < 50ms", d)
	}
	if c := log.Count("insights-link-missing"); c != 1000 {
		t.Fatalf("missing log count = %d, want 1000", c)
	}
}

// TestTC_HP_NFR_026h — go list -deps of internal/help/... contains no
// internal/insights, internal/alerts, or internal/notify imports.
//
// @aitri-tc TC-HP-NFR-026h
// @aitri-trace NFR-026 AC-052-002
func TestTC_HP_NFR_026h(t *testing.T) {
	out, err := goListDeps(t, "./internal/help/...")
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if strings.Contains(line, "github.com/cesareyeserrano/ultron-ap/internal/insights") ||
			strings.Contains(line, "github.com/cesareyeserrano/ultron-ap/internal/alerts") ||
			strings.Contains(line, "github.com/cesareyeserrano/ultron-ap/internal/notify") {
			// internal/help/contract is a peer of internal/help and is allowed —
			// but the help-page package itself must not pull in insights/alerts/notify.
			t.Fatalf("forbidden dependency: %s", line)
		}
	}
}

// TestTC_HP_NFR_026e — production .go files in internal/help do not import
// alerts/notify/telegram/smtp packages.
//
// @aitri-tc TC-HP-NFR-026e
// @aitri-trace NFR-026 AC-052-002
func TestTC_HP_NFR_026e(t *testing.T) {
	files, err := globGoFiles(t, "../../internal/help")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, f := range files {
		imports, err := readImports(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, p := range imports {
			lower := strings.ToLower(p)
			if strings.Contains(lower, "/internal/alerts") ||
				strings.Contains(lower, "/internal/notify") ||
				strings.Contains(lower, "telegram") ||
				strings.Contains(lower, "smtp") {
				t.Fatalf("file %s imports forbidden package %s", f, p)
			}
		}
	}
}

func deepCopyRules(in []contract.RuleLink) []contract.RuleLink {
	out := make([]contract.RuleLink, len(in))
	for i, r := range in {
		out[i] = contract.RuleLink{
			RuleID: r.RuleID,
			Links:  append([]string(nil), r.Links...),
		}
	}
	return out
}
