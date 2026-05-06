package help

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed glossary/glossary.json
var glossaryJSON []byte

// glossaryFile is the on-wire JSON schema. version pins the format so future
// breaking changes can land without surprising old binaries.
type glossaryFile struct {
	Version int               `json:"version"`
	Entries []json.RawMessage `json:"entries"`
}

// rawEntry mirrors the on-disk per-entry schema. Unknown fields are rejected
// via DisallowUnknownFields below so typos cannot silently disable behaviour
// (FR-049 AC-004).
type rawEntry struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Category   string      `json:"category"`
	Technical  string      `json:"technical"`
	Plain      string      `json:"plain"`
	Thresholds []Threshold `json:"thresholds,omitempty"`
	SourcePath string      `json:"source_path,omitempty"`
}

var (
	slugRE         = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	slugMaxLen     = 64
	validCategories = map[string]Category{
		"system-metrics":       CategorySystemMetrics,
		"network-probes":       CategoryNetworkProbes,
		"services-containers": CategoryServicesContainers,
		"vpn":                  CategoryVPN,
		"insights-verdicts":    CategoryInsightsVerdicts,
	}
)

// loadEmbedded parses the embedded glossary.json. It is the single ingestion
// point; the help.Service has no other content source (NFR-023).
//
// @aitri-trace FR-049 FR-055 NFR-023
func loadEmbedded(log LogFunc) ([]Entry, []byte, error) {
	return loadFromBytes(glossaryJSON, log)
}

// loadFromBytes is the testable seam. It applies strict schema validation,
// dedupes by id (first-wins), and returns entries sorted by category then by
// title — so EntriesByCategory yields alphabetical order without resorting.
func loadFromBytes(raw []byte, log LogFunc) ([]Entry, []byte, error) {
	if log == nil {
		log = func(string, ...interface{}) {}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc glossaryFile
	if err := dec.Decode(&doc); err != nil {
		return nil, raw, fmt.Errorf("help: parse glossary: %w", err)
	}
	if doc.Version != 1 {
		return nil, raw, fmt.Errorf("help: unsupported glossary version %d (want 1)", doc.Version)
	}

	out := make([]Entry, 0, len(doc.Entries))
	seen := make(map[string]struct{}, len(doc.Entries))

	for i, item := range doc.Entries {
		entry, err := decodeEntry(item)
		if err != nil {
			id := peekEntryID(item)
			log("event=glossary-entry-rejected id=%q index=%d reason=%q", id, i, err.Error())
			continue
		}
		if _, dup := seen[entry.ID]; dup {
			log("event=duplicate-entry-id id=%q index=%d", entry.ID, i)
			continue
		}
		seen[entry.ID] = struct{}{}
		out = append(out, entry)
	}

	// Sort entries by category-display-order, then alphabetically by title.
	// The category index slices in help.Service are populated in this order,
	// so EntriesByCategory yields alphabetical order without resorting.
	sort.SliceStable(out, func(i, j int) bool {
		ci, cj := categoryDisplayRank(out[i].Category), categoryDisplayRank(out[j].Category)
		if ci != cj {
			return ci < cj
		}
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})

	return out, raw, nil
}

// decodeEntry validates required fields and slug format. Returns a typed
// Entry on success.
func decodeEntry(raw json.RawMessage) (Entry, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var rr rawEntry
	if err := dec.Decode(&rr); err != nil {
		return Entry{}, normalizeJSONErr(err)
	}
	if strings.TrimSpace(rr.ID) == "" {
		return Entry{}, fmt.Errorf("missing-field=id")
	}
	if len(rr.ID) > slugMaxLen {
		return Entry{ID: rr.ID}, fmt.Errorf("invalid-id=too-long-%d", len(rr.ID))
	}
	if !slugRE.MatchString(rr.ID) {
		return Entry{ID: rr.ID}, fmt.Errorf("invalid-id=bad-chars")
	}
	if strings.TrimSpace(rr.Title) == "" {
		return Entry{ID: rr.ID}, fmt.Errorf("missing-field=title")
	}
	if rr.Category == "" {
		return Entry{ID: rr.ID}, fmt.Errorf("missing-field=category")
	}
	cat, ok := validCategories[rr.Category]
	if !ok {
		return Entry{ID: rr.ID}, fmt.Errorf("invalid-category=%s", rr.Category)
	}
	if strings.TrimSpace(rr.Technical) == "" {
		return Entry{ID: rr.ID}, fmt.Errorf("missing-field=technical")
	}
	if strings.TrimSpace(rr.Plain) == "" {
		return Entry{ID: rr.ID}, fmt.Errorf("missing-field=plain")
	}

	return Entry{
		ID:         rr.ID,
		Title:      rr.Title,
		Category:   cat,
		Technical:  rr.Technical,
		Plain:      rr.Plain,
		Thresholds: rr.Thresholds,
		SourcePath: rr.SourcePath,
	}, nil
}

// peekEntryID extracts the id from a raw entry blob even when full decoding
// failed (so the rejected log line names the offending entry).
func peekEntryID(raw json.RawMessage) string {
	var probe struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &probe)
	return probe.ID
}

// normalizeJSONErr maps the messy default unknown-field error into a stable
// "unknown field <name>" form so log assertions stay readable.
func normalizeJSONErr(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "unknown field") {
		// Strip the surrounding "json: " prefix that strict decode emits.
		return fmt.Errorf("schema=%s", strings.TrimPrefix(msg, "json: "))
	}
	return err
}

// categoryDisplayRank returns the category's position in the rendered order.
// Lower is earlier in the page. Unknown categories (which loader rejects) sort
// last as a defensive measure.
func categoryDisplayRank(c Category) int {
	for i, cc := range categoryOrder {
		if c == cc {
			return i
		}
	}
	return len(categoryOrder)
}
