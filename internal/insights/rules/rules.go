// Package rules ships the bundled v1 rule set as an embedded JSON blob and
// exposes a strict-schema decoder. Each rule in the bundled set is a self-
// contained JSON object validated against the FR-040 schema (id, title,
// condition, severity, verdict, recommendation, links). The decoder rejects
// unknown fields, duplicate ids, and invalid severity values, logging each
// rejection so a typo in a JSON file fails loudly rather than silently
// disabling diagnostics in production.
//
// @aitri-trace FR-040 FR-047 US-040 US-047 TC-IE-002h TC-IE-002f TC-IE-002e
package rules

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/cesareyeserrano/ultron-ap/internal/insights/lang"
)

//go:embed bundled.json
var bundledJSON []byte

// Severity is the rule severity. Values outside this set are rejected at load.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityCritical Severity = "critical"
)

// Rule is the in-memory representation of one bundled rule. ConditionRaw is
// kept around for persistence (the canonical condition_json column) while
// Compiled is the hot-path closure created at load time.
type Rule struct {
	ID             string
	Title          string
	ConditionRaw   json.RawMessage
	Compiled       *lang.Compiled
	Severity       Severity
	Verdict        string
	Recommendation string
	Links          []string
}

// LogFunc is the signature used by the loader to surface load-time errors.
// The engine wires this to its parent logger; tests inject capture-buffers.
type LogFunc func(format string, args ...interface{})

// LoadBundled parses the embedded bundled.json blob.
func LoadBundled(logf LogFunc) ([]Rule, error) {
	return LoadFromBytes(bundledJSON, logf)
}

// rawRule mirrors the on-disk schema. Unknown fields are rejected via
// DisallowUnknownFields; required fields produce per-field error messages.
type rawRule struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Condition      json.RawMessage `json:"condition"`
	Severity       string          `json:"severity"`
	Verdict        string          `json:"verdict"`
	Recommendation string          `json:"recommendation"`
	Links          []string        `json:"links,omitempty"`
}

// LoadFromBytes parses a JSON array of rules from raw bytes. A single
// malformed rule is dropped with a structured log line; remaining rules
// continue to load (FR-040 AC-001).
func LoadFromBytes(raw []byte, logf LogFunc) ([]Rule, error) {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var docs []json.RawMessage
	if err := dec.Decode(&docs); err != nil {
		return nil, fmt.Errorf("rules: parse top-level array: %w", err)
	}

	out := make([]Rule, 0, len(docs))
	seen := make(map[string]struct{}, len(docs))

	for i, raw := range docs {
		r, err := decodeRule(raw)
		if err != nil {
			id := r.ID
			if id == "" {
				// Strict decode may fail before populating ID; recover the
				// id with a permissive secondary decode so the structured
				// log line names the offending rule.
				if peeked := peekID(raw); peeked != "" {
					id = peeked
				}
			}
			if id == "" {
				id = fmt.Sprintf("(index=%d)", i)
			}
			logf("rules: rejected rule_id=%s reason=%s", id, err.Error())
			continue
		}
		if _, dup := seen[r.ID]; dup {
			logf("rules: rejected rule_id=%s reason=duplicate-rule-id", r.ID)
			continue
		}
		compiled, err := lang.Compile(r.ConditionRaw)
		if err != nil {
			logf("rules: rejected rule_id=%s reason=invalid-condition: %v", r.ID, err)
			continue
		}
		r.Compiled = compiled
		seen[r.ID] = struct{}{}
		out = append(out, r)
	}
	return out, nil
}

// decodeRule strict-decodes one rule blob. Returns a Rule with zeroed
// Compiled; the caller compiles the condition.
func decodeRule(raw json.RawMessage) (Rule, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var rr rawRule
	if err := dec.Decode(&rr); err != nil {
		// json.Decoder error messages already mention "unknown field …"
		// when DisallowUnknownFields rejects something, so re-shape the
		// error into the structured format expected by tests.
		return Rule{}, normalizeDecodeErr(err)
	}

	if rr.ID == "" {
		return Rule{}, fmt.Errorf("missing-field=id")
	}
	out := Rule{ID: rr.ID}
	if rr.Title == "" {
		return out, fmt.Errorf("missing-field=title")
	}
	if len(rr.Condition) == 0 {
		return out, fmt.Errorf("missing-field=condition")
	}
	if rr.Verdict == "" {
		return out, fmt.Errorf("missing-field=verdict")
	}
	if rr.Recommendation == "" {
		return out, fmt.Errorf("missing-field=recommendation")
	}
	sev, err := normalizeSeverity(rr.Severity)
	if err != nil {
		return out, err
	}
	links := rr.Links
	if links == nil {
		links = []string{}
	}
	out.Title = rr.Title
	out.ConditionRaw = append(out.ConditionRaw[:0], rr.Condition...)
	out.Severity = sev
	out.Verdict = rr.Verdict
	out.Recommendation = rr.Recommendation
	out.Links = links
	return out, nil
}

func normalizeSeverity(s string) (Severity, error) {
	switch Severity(s) {
	case SeverityInfo, SeverityWarn, SeverityCritical:
		return Severity(s), nil
	case "":
		return "", fmt.Errorf("missing-field=severity")
	default:
		return "", fmt.Errorf("invalid-severity=%s", s)
	}
}

// normalizeDecodeErr converts encoding/json error messages into the
// structured-log format expected by the loader's call sites.
func normalizeDecodeErr(err error) error {
	msg := err.Error()
	// Examples: 'json: unknown field "mute_until"' → unknown-field=mute_until
	if i := indexOf(msg, "unknown field "); i >= 0 {
		field := msg[i+len("unknown field "):]
		field = trimQuotes(field)
		return fmt.Errorf("unknown-field=%s", field)
	}
	return fmt.Errorf("invalid-json: %s", msg)
}

func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// peekID does a permissive decode of just the {"id":...} field so the
// structured-log line can name the rule even when the strict decoder
// rejected the rule for some other reason.
func peekID(raw json.RawMessage) string {
	var probe struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &probe)
	return probe.ID
}

func trimQuotes(s string) string {
	for len(s) > 0 && (s[0] == '"' || s[0] == '\'') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == '"' || s[len(s)-1] == '\'') {
		s = s[:len(s)-1]
	}
	return s
}
