// Insights-engine HTTP surface.
//
// @aitri-trace FR-043 FR-044 NFR-019 US-043 US-044 TC-IE-005h TC-IE-005f TC-IE-005e TC-IE-006h TC-IE-006f TC-IE-006e TC-IE-012h TC-IE-012f
package server

import (
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/insights"
)

// verdictJSON is the wire format for GET /api/insights/verdicts. Timestamps
// are emitted as ISO 8601 UTC strings (FR-042 AC-003).
type verdictJSON struct {
	RuleID          string   `json:"rule_id"`
	Title           string   `json:"title"`
	Severity        string   `json:"severity"`
	VerdictText     string   `json:"verdict_text"`
	Recommendation  string   `json:"recommendation"`
	Links           []string `json:"links"`
	FirstEmittedAt  string   `json:"first_emitted_at"`
	LastEvaluatedAt string   `json:"last_evaluated_at"`
}

// handleInsightsVerdicts serves the active verdict list (FR-043 / TC-IE-012h).
// Auth is enforced by the parent middleware, so the handler itself is small.
func (s *Server) handleInsightsVerdicts(w http.ResponseWriter, r *http.Request) {
	if s.insights == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "insights not initialized"})
		return
	}
	verdicts := s.insights.Active()
	out := make([]verdictJSON, 0, len(verdicts))
	for _, v := range verdicts {
		links := v.Links
		if links == nil {
			links = []string{}
		}
		out = append(out, verdictJSON{
			RuleID:          v.RuleID,
			Title:           v.Title,
			Severity:        string(v.Severity),
			VerdictText:     v.VerdictText,
			Recommendation:  v.Recommendation,
			Links:           links,
			FirstEmittedAt:  v.FirstEmittedAt.UTC().Format(time.RFC3339),
			LastEvaluatedAt: v.LastEvaluatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// VerdictsFragmentRow is one card passed to the verdicts partial template.
type VerdictsFragmentRow struct {
	RuleID         string
	Title          string
	Severity       string
	SeverityLabel  string // "CRITICAL", "WARN", "INFO" — explicit label (NFR-020)
	BadgeClass     string // Tailwind utility classes for the badge background.
	VerdictText    string
	Recommendation string
	RelativeTime   string // e.g. "first seen 2 min ago"

	// LearnMoreHref is "/help#<slug>" when the rule's links contain at least
	// one fragment matching a real glossary entry; empty otherwise. The
	// template renders the "Learn more →" anchor only when this is non-empty
	// (FR-053 — never render a dead link).
	LearnMoreHref string
}

// VerdictsFragmentData is the payload for the operational-indicators partial.
type VerdictsFragmentData struct {
	HasVerdicts bool
	Verdicts    []VerdictsFragmentRow
}

// renderVerdictsFragment renders the operational-indicators HTML fragment
// from the engine's current Active set. Used by the SSE broker (per tick) and
// by the synchronous /api/insights/verdicts/fragment endpoint (initial load).
func (s *Server) renderVerdictsFragment(now time.Time) string {
	data := s.gatherVerdictsFragment(now)
	tmpl, ok := s.tmplCache["partials/sse-verdicts.html"]
	if !ok {
		log.Printf("insights fragment: template not in cache")
		return ""
	}
	var buf stringWriter
	if err := tmpl.ExecuteTemplate(&buf, "partials/sse-verdicts.html", data); err != nil {
		log.Printf("insights fragment: render: %v", err)
		return ""
	}
	return buf.String()
}

// gatherVerdictsFragment projects engine verdicts into template-friendly rows.
func (s *Server) gatherVerdictsFragment(now time.Time) VerdictsFragmentData {
	out := VerdictsFragmentData{}
	if s.insights == nil {
		return out
	}
	verdicts := s.insights.Active()
	if len(verdicts) == 0 {
		return out
	}
	// Sort one more time defensively — Active already sorts but we want
	// the template to be agnostic to caller ordering.
	sort.SliceStable(verdicts, func(i, j int) bool {
		si, sj := severitySortKey(string(verdicts[i].Severity)), severitySortKey(string(verdicts[j].Severity))
		if si != sj {
			return si < sj
		}
		return verdicts[i].FirstEmittedAt.After(verdicts[j].FirstEmittedAt)
	})
	out.HasVerdicts = true
	out.Verdicts = make([]VerdictsFragmentRow, 0, len(verdicts))
	for _, v := range verdicts {
		row := VerdictsFragmentRow{
			RuleID:         v.RuleID,
			Title:          v.Title,
			Severity:       string(v.Severity),
			SeverityLabel:  severityLabel(string(v.Severity)),
			BadgeClass:     severityBadgeClass(string(v.Severity)),
			VerdictText:    v.VerdictText,
			Recommendation: v.Recommendation,
			RelativeTime:   "first seen " + relativeTime(now, v.FirstEmittedAt),
		}
		if s.help != nil {
			if href, ok := s.help.FirstValidAnchor(v.Links); ok {
				row.LearnMoreHref = href
			}
		}
		out.Verdicts = append(out.Verdicts, row)
	}
	return out
}

// handleInsightsFragment serves the same partial used over SSE for synchronous
// loads and HTTP-level fragment tests.
func (s *Server) handleInsightsFragment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(s.renderVerdictsFragment(time.Now())))
}

func severitySortKey(s string) int {
	switch s {
	case "critical":
		return 0
	case "warn":
		return 1
	case "info":
		return 2
	}
	return 3
}

func severityLabel(s string) string {
	switch s {
	case "critical":
		return "CRITICAL"
	case "warn":
		return "WARN"
	case "info":
		return "INFO"
	}
	return "UNKNOWN"
}

func severityBadgeClass(s string) string {
	switch s {
	case "critical":
		return "bg-danger/20 text-danger border-danger/40"
	case "warn":
		return "bg-yellow-500/20 text-yellow-400 border-yellow-500/40"
	case "info":
		return "bg-blue-500/20 text-blue-400 border-blue-500/40"
	}
	return "bg-card text-text-muted border-border/40"
}

func relativeTime(now, then time.Time) string {
	if then.IsZero() {
		return "just now"
	}
	d := now.Sub(then)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return formatPlural(int(d/time.Minute), "min")
	case d < 24*time.Hour:
		return formatPlural(int(d/time.Hour), "h")
	default:
		return then.UTC().Format("2006-01-02 15:04 UTC")
	}
}

// stringWriter is a minimal io.Writer that builds a string. We use this
// instead of bytes.Buffer to keep the fragment renderer self-contained.
type stringWriter struct{ b []byte }

func (s *stringWriter) Write(p []byte) (int, error) {
	s.b = append(s.b, p...)
	return len(p), nil
}

func (s *stringWriter) String() string { return string(s.b) }

// SetInsights wires the engine into the server. The engine may be nil for
// tests that exercise only the handler shell.
func (s *Server) SetInsights(svc *insights.Service) {
	s.insights = svc
}
