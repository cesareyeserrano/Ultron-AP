// Package help implements the /help glossary page (FR-048..FR-056). Glossary
// content is bundled into the binary via go:embed and parsed once at startup.
// The package is a read-only consumer of public types — it MUST NOT import
// internal/insights, internal/alerts, or internal/notify (NFR-026).
//
// Public surface (what server wires):
//   - help.New(LogFunc) (*Service, error)             — boot
//   - svc.Handler() http.Handler                       — GET /help
//   - svc.ValidateLinks([]contract.RuleLink)           — FR-052
//   - svc.FirstValidAnchor([]string) (string, bool)    — FR-053
//   - svc.EntryCount() int                             — boot log
//
// @aitri-trace FR-048 FR-049 FR-050 FR-051 FR-052 FR-053 FR-054 FR-055 NFR-022 NFR-023 NFR-024 NFR-025 NFR-026
package help

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"sync"
)

// LogFunc matches the signature used elsewhere in the project (log.Printf).
type LogFunc func(format string, args ...interface{})

// Category enumerates the five fixed categories.
type Category string

const (
	CategorySystemMetrics      Category = "system-metrics"
	CategoryNetworkProbes      Category = "network-probes"
	CategoryServicesContainers Category = "services-containers"
	CategoryVPN                Category = "vpn"
	CategoryInsightsVerdicts   Category = "insights-verdicts"
)

// categoryOrder is the rendered, fixed display order (FR-048 AC-001).
var categoryOrder = []Category{
	CategorySystemMetrics,
	CategoryNetworkProbes,
	CategoryServicesContainers,
	CategoryVPN,
	CategoryInsightsVerdicts,
}

// categoryLabel maps a category id to its human-readable header.
var categoryLabel = map[Category]string{
	CategorySystemMetrics:      "System metrics",
	CategoryNetworkProbes:      "Network probes",
	CategoryServicesContainers: "Services & containers",
	CategoryVPN:                "VPN",
	CategoryInsightsVerdicts:   "Insights verdicts",
}

// Threshold is one row of an entry's optional thresholds table.
type Threshold struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	Severity string `json:"severity"`
}

// Entry is a single glossary item. All fields are immutable post-load.
type Entry struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Category   Category    `json:"category"`
	Technical  string      `json:"technical"`
	Plain      string      `json:"plain"`
	Thresholds []Threshold `json:"thresholds,omitempty"`
	SourcePath string      `json:"source_path,omitempty"`
}

// Service is the help-page runtime. Construct with New, then call Handler /
// ValidateLinks / FirstValidAnchor. The service is immutable after New.
type Service struct {
	log       LogFunc
	entries   []Entry          // ordered as parsed
	byID      map[string]int   // id → index into entries
	byCat     map[Category][]int // ordered indexes per category
	etag      string           // sha256(glossary.json), 16-hex prefix
	tmpl      *template.Template
	once      sync.Once
}

// New parses the embedded glossary.json and returns a ready Service. Schema
// errors on individual entries are logged and the offending entry is skipped;
// only a catastrophic embed/parse failure returns an error.
//
// @aitri-trace FR-049 FR-055 NFR-023
func New(log LogFunc) (*Service, error) {
	if log == nil {
		log = func(string, ...interface{}) {}
	}
	entries, raw, err := loadEmbedded(log)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	log("event=glossary-loaded entries=%d", len(entries))
	return svc, nil
}

// EntryCount returns the number of loaded glossary entries. Used by the boot
// log line and tests.
//
// @aitri-trace FR-055
func (s *Service) EntryCount() int { return len(s.entries) }

// AllEntryIDs returns a fresh slice of every loaded entry id, in load order.
// Used by tests asserting slug stability (NFR-025).
func (s *Service) AllEntryIDs() []string {
	out := make([]string, len(s.entries))
	for i, e := range s.entries {
		out[i] = e.ID
	}
	return out
}

// EntryByID returns the entry with the given id, or nil if absent.
func (s *Service) EntryByID(id string) *Entry {
	i, ok := s.byID[id]
	if !ok {
		return nil
	}
	return &s.entries[i]
}

// EntriesByCategory returns the entries in a category, sorted alphabetically
// by title (FR-048 — "within a category, entries are sorted alphabetically").
// The slice is a fresh copy.
func (s *Service) EntriesByCategory(c Category) []Entry {
	idxs := s.byCat[c]
	out := make([]Entry, len(idxs))
	for i, idx := range idxs {
		out[i] = s.entries[idx]
	}
	return out
}

// FirstValidAnchor scans a rule's links and returns the first fragment whose
// id matches a loaded glossary entry. Returns ("/help#<id>", true) on hit;
// ("", false) if none match. Empty links and links without a leading "#" yield
// (false, ""). External links (http/https) are skipped.
//
// @aitri-trace FR-053
func (s *Service) FirstValidAnchor(links []string) (string, bool) {
	for _, l := range links {
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
			continue
		}
		if !strings.HasPrefix(l, "#") {
			continue
		}
		id := strings.TrimPrefix(l, "#")
		if _, ok := s.byID[id]; ok {
			return "/help#" + id, true
		}
	}
	return "", false
}

// Handler returns the GET /help http.Handler. Caller is responsible for
// wrapping it in auth middleware. Used when the help page is served as a
// standalone document; the ultron-ap server wraps RenderBody in the parent
// dashboard chrome instead.
//
// @aitri-trace FR-048
func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(s.serveHelp)
}

// RenderBody writes the /help <main> body fragment (without the parent
// sidebar/header chrome) to w. Used by internal/server.handleHelpPage so the
// help page reuses the dashboard layout, sidebar nav, and theme tokens.
//
// @aitri-trace FR-048 FR-050
func (s *Service) RenderBody(w io.Writer) error {
	return s.renderHTML(w)
}

// ETag returns the stable content-hash used for HTTP cache validation. The
// value is computed once at New() over the embedded glossary bytes.
//
// @aitri-trace NFR-022
func (s *Service) ETag() string { return s.etag }

// serveHelp is the GET /help endpoint. Cache-Control + ETag short-circuit
// repeat fetches; the body is server-rendered html/template.
func (s *Service) serveHelp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("ETag", "\""+s.etag+"\"")

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, s.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if err := s.renderHTML(w); err != nil {
		s.log("help: render failed: %v", err)
	}
}

// renderHTML writes the full /help body to w.
func (s *Service) renderHTML(w io.Writer) error {
	view := s.buildView()
	return s.tmpl.ExecuteTemplate(w, "help-page", view)
}

// view is the template input. Every field is plain data.
type view struct {
	Title      string
	Categories []categoryView
}

type categoryView struct {
	ID      string // e.g. "cat-system-metrics"
	Label   string // e.g. "System metrics"
	Entries []entryView
}

type entryView struct {
	ID         string // "entry-<slug>"
	AnchorID   string // raw slug
	Title      string
	Technical  string
	Plain      string
	Thresholds []Threshold
	SourcePath string
	Search     string // pre-lowercased haystack for client filter
}

func (s *Service) buildView() view {
	cats := make([]categoryView, 0, len(categoryOrder))
	for _, c := range categoryOrder {
		idxs := s.byCat[c]
		if len(idxs) == 0 {
			// Empty category is omitted entirely (FR-048 AC-005).
			continue
		}
		// Sorted alphabetically by title within a category — entries in
		// idxs were already sorted at load time (loader.go).
		es := make([]entryView, 0, len(idxs))
		for _, i := range idxs {
			e := s.entries[i]
			es = append(es, entryView{
				ID:         "entry-" + e.ID,
				AnchorID:   e.ID,
				Title:      e.Title,
				Technical:  e.Technical,
				Plain:      e.Plain,
				Thresholds: e.Thresholds,
				SourcePath: e.SourcePath,
				Search:     buildSearchHaystack(e),
			})
		}
		cats = append(cats, categoryView{
			ID:      "cat-" + string(c),
			Label:   categoryLabel[c],
			Entries: es,
		})
	}
	return view{
		Title:      "Help & glossary",
		Categories: cats,
	}
}

// buildSearchHaystack returns a lowercased concatenation of every searchable
// field, used as the data-search attribute by the inline filter. Pre-
// lowercasing on the server keeps the client-side filter at one
// String.includes per entry per keystroke (FR-054 AC-001).
func buildSearchHaystack(e Entry) string {
	var sb strings.Builder
	sb.Grow(len(e.Title) + len(e.Technical) + len(e.Plain) + 8)
	sb.WriteString(strings.ToLower(e.Title))
	sb.WriteByte(' ')
	sb.WriteString(strings.ToLower(e.Technical))
	sb.WriteByte(' ')
	sb.WriteString(strings.ToLower(e.Plain))
	return sb.String()
}

func etagFromBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

// Format helpers tracing.
var _ = fmt.Sprintf
