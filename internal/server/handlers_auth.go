package server

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/auth"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"golang.org/x/crypto/bcrypt"
)

type loginPageData struct {
	Error     string
	CSRFToken string
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.cleanupExpiredBruteForceAttempts()
	s.cleanupExpiredLoginTokens()

	csrfToken, err := auth.GenerateToken()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Store CSRF token server-side (avoids SameSite cookie issues on local
	// IP addresses in Chrome and other strict browsers). Token expires in 10min.
	s.loginTokens.Store(csrfToken, time.Now().Add(10*time.Minute))

	s.renderLogin(w, loginPageData{CSRFToken: csrfToken})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)

	if s.bruteForce.IsLocked(ip) {
		s.renderLogin(w, loginPageData{
			Error: "Too many login attempts. Try again in 15 minutes.",
		})
		return
	}

	// Validate CSRF token from server-side store (one-time use, avoids cookie SameSite issues)
	submitted := r.FormValue("csrf_token")
	expiry, ok := s.loginTokens.LoadAndDelete(submitted)
	if !ok || submitted == "" || time.Now().After(expiry.(time.Time)) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := s.db.GetUserByUsername(username)
	if err != nil {
		log.Printf("Database error during login: %v", err)
		s.renderLoginWithError(w, "Internal server error")
		return
	}

	// Constant-time: always run bcrypt even if user not found
	var storedHash string
	if user != nil {
		storedHash = user.PasswordHash
	} else {
		// Dummy hash so bcrypt still runs (prevents timing attack)
		storedHash = "$2a$10$0000000000000000000000000000000000000000000000000000"
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)); err != nil || user == nil {
		s.bruteForce.RecordFailure(ip)
		s.renderLoginWithError(w, "Invalid username or password")
		return
	}

	// Success — reset brute force, create session
	s.bruteForce.Reset(ip)

	sessionToken, err := generateSessionToken()
	if err != nil {
		log.Printf("Failed to generate session token: %v", err)
		s.renderLoginWithError(w, "Internal server error")
		return
	}

	csrfToken, err := auth.GenerateToken()
	if err != nil {
		log.Printf("Failed to generate CSRF token: %v", err)
		s.renderLoginWithError(w, "Internal server error")
		return
	}

	session := &database.Session{
		ID:        sessionToken,
		UserID:    user.ID,
		CSRFToken: csrfToken,
		ExpiresAt: time.Now().Add(s.cfg.SessionTTL),
	}

	if err := s.db.CreateSession(session); err != nil {
		log.Printf("Failed to create session: %v", err)
		s.renderLoginWithError(w, "Internal server error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
		Expires:  session.ExpiresAt,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// FR-012 / BG-040: logout is a state-changing POST and must carry a valid
	// CSRF token, like every other mutating endpoint. The logout form already
	// embeds {{.CSRFToken}}.
	if !s.validateCSRF(w, r) {
		return
	}

	cookie, err := r.Cookie("session")
	if err == nil {
		s.db.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPSRequest(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, data loginPageData) {
	tmpl, ok := s.tmplCache["login.html"]
	if !ok {
		log.Printf("render: login.html not in cache")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render: login.html execute failed: %v", err)
	}
}

func (s *Server) renderLoginWithError(w http.ResponseWriter, msg string) {
	s.cleanupExpiredBruteForceAttempts()
	s.cleanupExpiredLoginTokens()

	csrfToken, _ := auth.GenerateToken()
	s.loginTokens.Store(csrfToken, time.Now().Add(10*time.Minute))
	s.renderLogin(w, loginPageData{Error: msg, CSRFToken: csrfToken})
}

func (s *Server) cleanupExpiredLoginTokens() {
	now := time.Now()
	nowUnix := now.Unix()
	last := s.loginTokenSweepNs.Load()
	if nowUnix-last < 60 {
		return
	}
	if !s.loginTokenSweepNs.CompareAndSwap(last, nowUnix) {
		return
	}

	s.loginTokens.Range(func(key, value any) bool {
		expiry, ok := value.(time.Time)
		if !ok || now.After(expiry) {
			s.loginTokens.Delete(key)
		}
		return true
	})
}

func (s *Server) cleanupExpiredBruteForceAttempts() {
	nowUnix := time.Now().Unix()
	last := s.bruteForceSweepNs.Load()
	if nowUnix-last < 60 {
		return
	}
	if !s.bruteForceSweepNs.CompareAndSwap(last, nowUnix) {
		return
	}
	if s.bruteForce != nil {
		s.bruteForce.CleanupExpired()
	}
}

func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
