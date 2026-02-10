package server

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const userContextKey contextKey = "user_id"

func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userContextKey).(int64)
	return id, ok
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			s.redirectOrUnauthorized(w, r)
			return
		}

		session, err := s.db.GetSession(cookie.Value)
		if err != nil || session == nil {
			s.redirectOrUnauthorized(w, r)
			return
		}

		if time.Now().After(session.ExpiresAt) {
			s.db.DeleteSession(session.ID)
			s.redirectOrUnauthorized(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, session.UserID)
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
