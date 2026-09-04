package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/auth"
)

func (s *Server) validateCSRF(w http.ResponseWriter, r *http.Request) bool {
	// Reuse the session requireAuth already loaded (D2); falls back to a cookie
	// lookup for any caller not behind requireAuth.
	session := s.sessionForRequest(r)
	if session == nil {
		s.auditLog(r, "security", "csrf_reject", r.URL.Path, "missing or invalid session", false)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	csrfToken := r.FormValue("csrf_token")
	if csrfToken == "" {
		csrfToken = r.Header.Get("X-CSRF-Token")
	}

	// BG-002: use constant-time compare to match auth.ValidateToken and stay
	// timing-safe if the token format ever shrinks or gains structure.
	if !auth.ValidateToken(session.CSRFToken, csrfToken) {
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

// isSameOriginRequest enforces same-origin on Origin/Referer when present. The
// scheme is derived from the raw request (TLS or X-Forwarded-Proto) rather than
// the trusted-proxy-gated isHTTPSRequest: a spoofed X-Forwarded-Proto cannot
// help a cross-origin attacker here (they would still have to match the host),
// and gating it would reject legitimate POSTs from a TLS-terminating reverse
// proxy that has not been added to ULTRON_TRUSTED_PROXIES.
func isSameOriginRequest(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if origin == "" && referer == "" {
		return true
	}

	expectedScheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
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
