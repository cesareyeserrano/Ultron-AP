package server

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"log"
	"net"
	"net/http"
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
	csrfToken, err := auth.GenerateToken()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Store CSRF token in a temporary cookie for the login form
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_login",
		Value:    csrfToken,
		Path:     "/login",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

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

	// Validate CSRF token from cookie
	csrfCookie, err := r.Cookie("csrf_login")
	if err != nil || !auth.ValidateToken(csrfCookie.Value, r.FormValue("csrf_token")) {
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
		SameSite: http.SameSiteStrictMode,
	})

	// Clear the login CSRF cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "csrf_login",
		Value:  "",
		Path:   "/login",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
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
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, data loginPageData) {
	tmpl, err := template.ParseFS(s.templates, "templates/login.html")
	if err != nil {
		log.Printf("Failed to parse login template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

func (s *Server) renderLoginWithError(w http.ResponseWriter, msg string) {
	csrfToken, _ := auth.GenerateToken()
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_login",
		Value:    csrfToken,
		Path:     "/login",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	s.renderLogin(w, loginPageData{Error: msg, CSRFToken: csrfToken})
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
