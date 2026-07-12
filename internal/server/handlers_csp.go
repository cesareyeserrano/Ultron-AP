package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// cspReportMaxBodyBytes caps the request body the browser can POST to
// /api/csp-report. Real CSP violation payloads are well under 4 KiB;
// the cap protects against an attacker burning panel disk / log volume
// by spraying large bodies at this unauthenticated endpoint. Reports
// larger than the cap are truncated and noted.
const cspReportMaxBodyBytes = 8 * 1024

// handleCSPReport ingests browser-emitted CSP violation reports so we
// can monitor the existing Content-Security-Policy-Report-Only header
// for breakage before flipping it to enforced (BL-012 / BG-032).
//
// The endpoint MUST stay unauthenticated and CSRF-free — browsers do
// not attach session cookies or CSRF tokens when generating reports
// for cross-origin or block events. To bound abuse, we cap body size
// and only accept the two well-known content types. Each report is
// logged in a single line so a future log scraper can grep for them.
func (s *Server) handleCSPReport(w http.ResponseWriter, r *http.Request) {
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	// Strip charset / boundary parameters to compare base type only.
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "application/csp-report", "application/reports+json", "application/json":
	default:
		http.Error(w, "unsupported content type", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, cspReportMaxBodyBytes+1))
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}
	truncated := len(body) > cspReportMaxBodyBytes
	if truncated {
		body = body[:cspReportMaxBodyBytes]
	}

	logCSPReport(body, truncated, r.RemoteAddr, ct)
	w.WriteHeader(http.StatusNoContent)
}

// logCSPReport writes one log line per report and tries to surface the
// most useful fields (blocked-uri, violated-directive, document-uri).
// Older reports use Content-Type: application/csp-report with shape
// {"csp-report": {...}}. Reporting API v1 uses application/reports+json
// with a JSON array of {"type":"csp-violation","body":{...}} entries.
// Anything we can't parse falls back to the raw body, capped above.
func logCSPReport(body []byte, truncated bool, remote, contentType string) {
	prefix := "csp-report:"
	if truncated {
		prefix = "csp-report (truncated):"
	}

	// Try legacy {"csp-report": {...}} first.
	var legacy struct {
		Report map[string]any `json:"csp-report"`
	}
	if err := json.Unmarshal(body, &legacy); err == nil && legacy.Report != nil {
		log.Printf("%s remote=%s blocked=%q directive=%q document=%q",
			prefix, remote,
			cspLogField(legacy.Report["blocked-uri"]),
			cspLogField(legacy.Report["violated-directive"]),
			cspLogField(legacy.Report["document-uri"]))
		return
	}

	// Reporting API v1: array of {"type":"csp-violation","body":{...}}.
	var modern []struct {
		Type string         `json:"type"`
		Body map[string]any `json:"body"`
	}
	if err := json.Unmarshal(body, &modern); err == nil && len(modern) > 0 {
		for _, e := range modern {
			if e.Type != "csp-violation" || e.Body == nil {
				continue
			}
			log.Printf("%s remote=%s blocked=%q directive=%q document=%q",
				prefix, remote,
				cspLogField(e.Body["blockedURL"]),
				cspLogField(e.Body["effectiveDirective"]),
				cspLogField(e.Body["documentURL"]))
		}
		return
	}

	// Last resort: log the raw body for forensics.
	log.Printf("%s remote=%s ct=%s raw=%q", prefix, remote, contentType, body)
}

// cspLogField renders an attacker-controlled report field for a single log
// line. The %q verb the callers use already escapes CR/LF, but capping the
// length keeps one sprayed report from dominating the journal (B8).
func cspLogField(v any) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > 256 {
		s = s[:256] + "…"
	}
	return s
}
