package server

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) validateCSRF(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		s.auditLog(r, "security", "csrf_reject", r.URL.Path, "missing session cookie", false)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	session, err := s.db.GetSession(cookie.Value)
	if err != nil || session == nil {
		s.auditLog(r, "security", "csrf_reject", r.URL.Path, "invalid session", false)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	csrfToken := r.FormValue("csrf_token")
	if csrfToken == "" {
		csrfToken = r.Header.Get("X-CSRF-Token")
	}

	if csrfToken != session.CSRFToken {
		s.auditLog(r, "security", "csrf_reject", r.URL.Path, "csrf token mismatch", false)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}

	// Defense-in-depth: enforce same-origin when Origin/Referer are provided.
	if !isSameOriginRequest(r) {
		s.auditLog(r, "security", "csrf_reject", r.URL.Path, "origin/referer mismatch", false)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func isSameOriginRequest(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if origin == "" && referer == "" {
		return true
	}

	expectedScheme := "http"
	if isHTTPSRequest(r) {
		expectedScheme = "https"
	}
	expectedHost := r.Host

	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		if !strings.EqualFold(u.Scheme, expectedScheme) || !strings.EqualFold(u.Host, expectedHost) {
			return false
		}
	}

	if referer != "" {
		u, err := url.Parse(referer)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		if !strings.EqualFold(u.Scheme, expectedScheme) || !strings.EqualFold(u.Host, expectedHost) {
			return false
		}
	}

	return true
}
