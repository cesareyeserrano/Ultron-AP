package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @aitri-trace BG-033 BL-021

func TestVersionEndpoint_ReturnsBuildMetadata(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, Version, got["version"])
	assert.Equal(t, BuildCommit, got["commit"])
	assert.Equal(t, runtime.Version(), got["go"])
}

func TestVersionEndpoint_NoAuthRequired(t *testing.T) {
	// deploy-verify and external uptime tooling poll /version without
	// holding a session cookie. Confirm the route is in the public
	// allowlist.
	srv, _ := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusFound, rec.Code, "must not redirect to /login")
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestVersionEndpoint_NoCacheHeader(t *testing.T) {
	// Stale caches would defeat the point — deploy-verify needs the
	// live answer from the running process.
	srv, _ := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestVersionEndpoint_LdflagsInjectionPath(t *testing.T) {
	// Confirm BuildCommit is a settable var (not a const), so the
	// Makefile -ldflags -X path can populate it. If someone refactors
	// it back to a const, this test will fail to compile.
	prev := BuildCommit
	BuildCommit = "deadbeef"
	t.Cleanup(func() { BuildCommit = prev })

	srv, _ := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	srv.httpServer.Handler.ServeHTTP(rec, req)
	var got map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "deadbeef", got["commit"])
}
