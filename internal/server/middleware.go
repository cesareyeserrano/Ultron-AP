package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

type contextKey string

const sessionContextKey contextKey = "session"

// SessionFromContext returns the authenticated session that requireAuth stored
// on the request context. Consumers behind requireAuth reuse it instead of
// re-querying the DB for the same row (D2).
func SessionFromContext(ctx context.Context) (*database.Session, bool) {
	s, ok := ctx.Value(sessionContextKey).(*database.Session)
	return s, ok && s != nil
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	if s, ok := SessionFromContext(ctx); ok {
		return s.UserID, true
	}
	return 0, false
}

// sessionForRequest returns the session for this request, preferring the one
// requireAuth already loaded onto the context (D2) and falling back to a cookie
// lookup for any caller not behind requireAuth. Returns nil when there is none.
func (s *Server) sessionForRequest(r *http.Request) *database.Session {
	if sess, ok := SessionFromContext(r.Context()); ok {
		return sess
	}
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	sess, err := s.db.GetSession(cookie.Value)
	if err != nil {
		return nil
	}
	return sess
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			s.auditLog(r, "security", "auth_reject", r.URL.Path, "missing session cookie", false)
			s.redirectOrUnauthorized(w, r)
			return
		}

		session, err := s.db.GetSession(cookie.Value)
		if err != nil || session == nil {
			s.auditLog(r, "security", "auth_reject", r.URL.Path, "invalid session", false)
			s.redirectOrUnauthorized(w, r)
			return
		}

		if time.Now().After(session.ExpiresAt) {
			s.db.DeleteSession(session.ID)
			s.auditLog(r, "security", "auth_reject", r.URL.Path, "expired session", false)
			s.redirectOrUnauthorized(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) redirectOrUnauthorized(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// CSP. Report-Only by default while /api/csp-report ingests
		// browser violation reports; ULTRON_CSP_ENFORCE=1 flips to
		// enforced once the policy is verified to break nothing.
		// report-uri runs in both modes. (BL-012 / BG-032.)
		const cspPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; report-uri /api/csp-report"
		cspHeader := "Content-Security-Policy-Report-Only"
		if s.cfg != nil && s.cfg.CSPEnforce {
			cspHeader = "Content-Security-Policy"
		}
		h.Set(cspHeader, cspPolicy)
		if s.isHTTPSRequest(r) {
			h.Set("Strict-Transport-Security", "max-age=15552000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
