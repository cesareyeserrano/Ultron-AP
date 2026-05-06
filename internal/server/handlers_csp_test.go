package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// @aitri-trace BG-032 BL-012

func TestCSPReport_AcceptsLegacyContentType(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	body := []byte(`{"csp-report":{"document-uri":"https://x/","violated-directive":"script-src","blocked-uri":"inline"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/csp-report")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCSPReport_AcceptsReportingAPIv1ContentType(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	body := []byte(`[{"type":"csp-violation","body":{"documentURL":"https://x/","effectiveDirective":"script-src","blockedURL":"inline"}}]`)
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/reports+json")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCSPReport_RejectsUnsupportedContentType(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	body := []byte(`<csp blocked />`)
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/xml")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestCSPReport_AcceptsContentTypeWithCharset(t *testing.T) {
	// Some browsers send "application/csp-report; charset=UTF-8"; the
	// handler must compare base type only.
	srv, _ := setupSSETestServer(t)
	body := []byte(`{"csp-report":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/csp-report; charset=UTF-8")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCSPReport_NoAuthRequired(t *testing.T) {
	// Browsers do not attach session cookies or CSRF tokens to CSP
	// reports. The endpoint must remain reachable without either.
	srv, _ := setupSSETestServer(t)
	body := []byte(`{"csp-report":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/csp-report")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCSPReport_TruncatesOversizedBody(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	huge := bytes.Repeat([]byte("A"), cspReportMaxBodyBytes*2)
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", bytes.NewReader(huge))
	req.Header.Set("Content-Type", "application/csp-report")
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	// Even truncated, the endpoint accepts the report so the browser
	// doesn't retry indefinitely.
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// --- Header configuration ---

func TestSecurityHeaders_CSPReportOnlyByDefault(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Security-Policy-Report-Only"); got == "" {
		t.Fatal("expected Content-Security-Policy-Report-Only header by default")
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("expected enforced CSP header to be absent by default, got %q", got)
	}
}

func TestSecurityHeaders_CSPEnforcedWhenConfigured(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	srv.cfg.CSPEnforce = true
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("expected enforced Content-Security-Policy header when CSPEnforce=true")
	}
	if got := rec.Header().Get("Content-Security-Policy-Report-Only"); got != "" {
		t.Fatalf("expected Report-Only header to be absent when enforced, got %q", got)
	}
}

func TestSecurityHeaders_CSPIncludesReportURI(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	csp := rec.Header().Get("Content-Security-Policy-Report-Only")
	if !strings.Contains(csp, "report-uri /api/csp-report") {
		t.Fatalf("CSP must include report-uri so reports flow even in Report-Only mode, got %q", csp)
	}
}
