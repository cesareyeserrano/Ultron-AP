package server

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

func setupAuthHandlerTest(t *testing.T) (*Server, *database.DB) {
	t.Helper()

	cfg := &config.Config{
		Port:       8080,
		DBPath:     filepath.Join(t.TempDir(), "test.db"),
		LogLevel:   "info",
		AdminUser:  "admin",
		AdminPass:  "secret",
		SessionTTL: 24 * time.Hour,
	}

	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Create admin user with bcrypt hash
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	require.NoError(t, err)
	require.NoError(t, db.CreateUser("admin", string(hash)))

	srv := New(cfg, db, nil, nil, nil, nil, nil)
	return srv, db
}

// extractCSRFToken extracts the CSRF token from the login form HTML body.
func extractCSRFToken(body string) string {
	const marker = `name="csrf_token" value="`
	idx := strings.Index(body, marker)
	if idx == -1 {
		return ""
	}
	rest := body[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

func TestLoginPage_Renders(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "username")
	assert.Contains(t, body, "password")
	assert.Contains(t, body, "csrf_token")
}

func TestLogin_Success(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	// GET /login to get CSRF token embedded in HTML
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(getRec, getReq)
	csrfToken := extractCSRFToken(getRec.Body.String())
	require.NotEmpty(t, csrfToken, "should embed csrf_token in login form")

	// POST /login with correct credentials
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("csrf_token", csrfToken)

	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusSeeOther, postRec.Code)
	assert.Equal(t, "/", postRec.Header().Get("Location"))

	// Verify session cookie is set
	var sessionCookie *http.Cookie
	for _, c := range postRec.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
		}
	}
	require.NotNil(t, sessionCookie, "should set session cookie")
	assert.True(t, sessionCookie.HttpOnly, "session cookie must be HttpOnly")
	assert.Equal(t, http.SameSiteLaxMode, sessionCookie.SameSite, "session cookie must be SameSite=Lax")
}

func TestLogin_FailedWrongPassword(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(getRec, getReq)
	csrfToken := extractCSRFToken(getRec.Body.String())
	require.NotEmpty(t, csrfToken)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "wrongpassword")
	form.Set("csrf_token", csrfToken)

	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusOK, postRec.Code)
	body := postRec.Body.String()
	assert.Contains(t, body, "Invalid username or password")

	// No session cookie should be set
	for _, c := range postRec.Result().Cookies() {
		assert.NotEqual(t, "session", c.Name, "session cookie should not be set on failure")
	}
}

func TestLogin_FailedWrongUsername(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(getRec, getReq)
	csrfToken := extractCSRFToken(getRec.Body.String())
	require.NotEmpty(t, csrfToken)

	form := url.Values{}
	form.Set("username", "nonexistent")
	form.Set("password", "secret")
	form.Set("csrf_token", csrfToken)

	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(postRec, postReq)

	// Same error message regardless of whether user exists (no info leak)
	body := postRec.Body.String()
	assert.Contains(t, body, "Invalid username or password")
}

func TestLogin_CSRFMissing(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestLogin_CSRFInvalid(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("csrf_token", "invalid-token-not-in-store")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestLogin_BruteForce_LocksAfter5Failures(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	for i := 0; i < 5; i++ {
		getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
		getRec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(getRec, getReq)
		csrfToken := extractCSRFToken(getRec.Body.String())
		require.NotEmpty(t, csrfToken)

		form := url.Values{}
		form.Set("username", "admin")
		form.Set("password", "wrong")
		form.Set("csrf_token", csrfToken)

		postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		postRec := httptest.NewRecorder()

		srv.httpServer.Handler.ServeHTTP(postRec, postReq)
	}

	// 6th attempt — should be locked even with correct password
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(getRec, getReq)
	csrfToken := extractCSRFToken(getRec.Body.String())
	require.NotEmpty(t, csrfToken)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("csrf_token", csrfToken)

	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(postRec, postReq)

	body := postRec.Body.String()
	assert.Contains(t, body, "Too many login attempts")
}

func TestLogout_ClearsSessionAndRedirects(t *testing.T) {
	srv, db := setupAuthHandlerTest(t)

	// Create a session
	user, _ := db.GetUserByUsername("admin")
	require.NoError(t, db.CreateSession(&database.Session{
		ID:        "logout-test-token",
		UserID:    user.ID,
		CSRFToken: "csrf",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}))

	// FR-012 / BG-040: logout now requires a valid CSRF token.
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader("csrf_token=csrf"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: "logout-test-token"})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))

	// Session cookie should be cleared
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
		}
	}
	require.NotNil(t, sessionCookie)
	assert.Equal(t, -1, sessionCookie.MaxAge, "session cookie should be expired")

	// Session should be deleted from DB
	session, _ := db.GetSession("logout-test-token")
	assert.Nil(t, session)
}

// BG-040 regression: a POST /logout without a valid CSRF token must be rejected
// and must NOT destroy the session (FR-012).
func TestLogout_RejectsMissingCSRFToken(t *testing.T) {
	srv, db := setupAuthHandlerTest(t)

	user, _ := db.GetUserByUsername("admin")
	require.NoError(t, db.CreateSession(&database.Session{
		ID:        "logout-csrf-token",
		UserID:    user.ID,
		CSRFToken: "csrf",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}))

	// No csrf_token in the body — a forged cross-site request.
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "logout-csrf-token"})
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code, "missing CSRF token must be rejected with 403")

	// Session must still exist — the forged request did not log the user out.
	session, _ := db.GetSession("logout-csrf-token")
	assert.NotNil(t, session, "session must survive a CSRF-less logout attempt")
}

// loginAndGetSessionCookie performs the login flow and returns the session cookie.
func loginAndGetSessionCookie(t *testing.T, srv *Server, mutate func(*http.Request)) *http.Cookie {
	t.Helper()
	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(getRec, getReq)
	csrfToken := extractCSRFToken(getRec.Body.String())
	require.NotEmpty(t, csrfToken)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("csrf_token", csrfToken)

	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if mutate != nil {
		mutate(postReq)
	}
	postRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(postRec, postReq)
	assert.Equal(t, http.StatusSeeOther, postRec.Code)

	for _, c := range postRec.Result().Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	return nil
}

// BG-042: X-Forwarded-Proto sets the Secure flag only when the TCP peer is a
// configured trusted proxy.
func TestLogin_SetsSecureCookie_WhenForwardedProtoHTTPS_FromTrustedProxy(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)
	// httptest's default RemoteAddr is 192.0.2.1:1234 — trust that peer.
	_, ipnet, err := net.ParseCIDR("192.0.2.1/32")
	require.NoError(t, err)
	srv.cfg.TrustedProxies = []*net.IPNet{ipnet}

	cookie := loginAndGetSessionCookie(t, srv, func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https")
	})
	require.NotNil(t, cookie)
	assert.True(t, cookie.Secure, "Secure flag set when X-Forwarded-Proto:https comes from a trusted proxy")
}

// BG-042 regression: X-Forwarded-Proto from an untrusted peer (no trusted proxy
// configured) must NOT influence the Secure flag — it is spoofable.
func TestLogin_IgnoresForwardedProtoHTTPS_WhenNotTrustedProxy(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)
	// No TrustedProxies configured (default).
	cookie := loginAndGetSessionCookie(t, srv, func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https")
	})
	require.NotNil(t, cookie)
	assert.False(t, cookie.Secure, "spoofable X-Forwarded-Proto must be ignored without a trusted proxy")
}

func TestLogin_SetsSecureCookie_WhenTLS(t *testing.T) {
	srv, _ := setupAuthHandlerTest(t)

	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(getRec, getReq)
	csrfToken := extractCSRFToken(getRec.Body.String())
	require.NotEmpty(t, csrfToken)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("csrf_token", csrfToken)

	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.TLS = &tls.ConnectionState{}
	postRec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(postRec, postReq)
	assert.Equal(t, http.StatusSeeOther, postRec.Code)

	var sessionCookie *http.Cookie
	for _, c := range postRec.Result().Cookies() {
		if c.Name == "session" {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie)
	assert.True(t, sessionCookie.Secure)
}
