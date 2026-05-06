package server

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"
)

// handleHelpPage renders /help inside the dashboard chrome. The help.Service
// emits the body fragment; the parent placeholder.html template wraps it in
// the existing sidebar/header layout so the page reuses parent FR-007 auth,
// FR-009 tokens, and FR-056 sidebar (which carries the active "Help" item
// when ActivePage="help").
//
// @aitri-trace FR-048 FR-051 FR-053 FR-056 NFR-022 NFR-023 NFR-024 NFR-025
func (s *Server) handleHelpPage(w http.ResponseWriter, r *http.Request) {
	if s.help == nil {
		http.Error(w, "Help unavailable", http.StatusServiceUnavailable)
		return
	}

	// Cache-Control + ETag (NFR-022). Computed by help.Service.
	if etag, ok := s.helpETag(); ok {
		w.Header().Set("ETag", "\""+etag+"\"")
		if match := r.Header.Get("If-None-Match"); match != "" && containsETag(match, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=300")

	// Render the fragment to a buffer, then template-execute the parent
	// placeholder.html with it as the content. Buffering lets us preserve
	// the existing render() pattern without leaking writes on error.
	var body bytes.Buffer
	if err := s.help.RenderBody(&body); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.render(w, r, "help.html", "Help", "help", template.HTML(body.String()))
}

// helpETag returns the underlying glossary etag for the cache short-circuit.
func (s *Server) helpETag() (string, bool) {
	if s.help == nil {
		return "", false
	}
	return s.help.ETag(), true
}

// containsETag is a relaxed If-None-Match check — a single etag substring
// match is enough since we only emit one weak/strong tag.
func containsETag(header, etag string) bool {
	if header == "" || etag == "" {
		return false
	}
	if header == "*" {
		return true
	}
	return strings.Contains(header, etag)
}
