// @aitri-tc TC-SR-068h, TC-SR-068e, TC-SR-068f, TC-SR-068sec1, TC-SR-068sec2, TC-SR-068sec3
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: build authed GET request to the probe endpoint
func probeRequest(t *testing.T, sessionID, scheme, value string) *http.Request {
	t.Helper()
	q := url.Values{}
	q.Set("scheme", scheme)
	q.Set("value", value)
	req := httptest.NewRequest(http.MethodGet, "/api/settings/encryption-key/probe?"+q.Encode(), nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
	return req
}

// TC-SR-068h — env var SET → ok:true with locked reason shape.
func TestProbe_EnvVarSet_ReturnsOKWithLockedReason(t *testing.T) {
	t.Setenv("ULTRON_BACKUP_KEY_TEST_FOUND", "any-value")
	srv, session := setupSSETestServer(t)

	req := probeRequest(t, session.ID, "env", "ULTRON_BACKUP_KEY_TEST_FOUND")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))

	keys := make([]string, 0)
	for k := range result {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"ok", "reason"}, keys, "probe response must have exactly {ok, reason} keys")
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "env var ULTRON_BACKUP_KEY_TEST_FOUND found", result["reason"])
}

// TC-SR-068e — env var UNSET → ok:false locked reason.
func TestProbe_EnvVarUnset_ReturnsLockedNotSet(t *testing.T) {
	os.Unsetenv("DOES_NOT_EXIST_PROBE_TEST")
	srv, session := setupSSETestServer(t)

	req := probeRequest(t, session.ID, "env", "DOES_NOT_EXIST_PROBE_TEST")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var result probeResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.False(t, result.OK)
	assert.Equal(t, "env var not set", result.Reason)
}

// TC-SR-068f — unauthenticated probe returns 401.
func TestProbe_Unauthenticated_Returns401(t *testing.T) {
	srv, _ := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/settings/encryption-key/probe?scheme=env&value=FOO", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TC-SR-068sec1 — golden-shape: every (scheme, value) combo returns exactly
// {ok, reason} with reason in the locked enum (or the env-var-found dynamic
// pattern for the env-found case).
func TestProbe_ResponseShape_LockedEnum(t *testing.T) {
	srv, session := setupSSETestServer(t)

	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "key.bin")
	require.NoError(t, os.WriteFile(existingFile, []byte("dummy"), 0o600))

	t.Setenv("ENV_PROBE_GOLDEN", "x")

	cases := []struct {
		name    string
		scheme  string
		value   string
		wantOK  bool
		want    string // "" means: assert env-found pattern
		envName string // for the env-found case
	}{
		{"env-found", "env", "ENV_PROBE_GOLDEN", true, "", "ENV_PROBE_GOLDEN"},
		{"env-missing", "env", "ENV_PROBE_GOLDEN_MISSING_XYZ", false, "env var not set", ""},
		{"file-readable", "file", existingFile, true, "file readable", ""},
		{"file-missing", "file", filepath.Join(tmpDir, "no-such.bin"), false, "file not found", ""},
		{"kms-not-supported", "kms", "anything", false, "kms scheme not supported in v1", ""},
		{"empty-scheme", "", "", false, "scheme required", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := probeRequest(t, session.ID, c.scheme, c.value)
			rec := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(rec, req)

			require.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusBadRequest,
				"probe must return 200 or 400 (got %d)", rec.Code)

			var raw map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

			// Key set MUST be exactly {ok, reason} — guards against new fields
			// leaking key data.
			require.Len(t, raw, 2)
			require.Contains(t, raw, "ok")
			require.Contains(t, raw, "reason")

			ok, _ := raw["ok"].(bool)
			reason, _ := raw["reason"].(string)
			assert.Equal(t, c.wantOK, ok)
			assert.LessOrEqual(t, len(reason), probeReasonMaxLen, "reason must stay under length cap")

			if c.envName != "" {
				assert.Equal(t, "env var "+c.envName+" found", reason)
			} else {
				assert.True(t, probeReasonEnum[reason] || reason == c.want,
					"reason %q is not in the locked enum (or expected %q)", reason, c.want)
			}
		})
	}
}

// TC-SR-068sec2 — path traversal rejected with locked reason.
func TestProbe_FileScheme_PathTraversalRejected(t *testing.T) {
	srv, session := setupSSETestServer(t)

	cases := []string{
		"/etc/ultron-ap/../../etc/passwd",
		"./etc/passwd", // not absolute
		"../../../etc/passwd",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			req := probeRequest(t, session.ID, "file", p)
			rec := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code, "traversal must be rejected with 400")

			var result probeResult
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			assert.False(t, result.OK)
			assert.Contains(t, []string{"file not found", "file not readable"}, result.Reason)
		})
	}
}

// TC-SR-068sec3 — null byte rejected.
func TestProbe_FileScheme_NullByteRejected(t *testing.T) {
	srv, session := setupSSETestServer(t)
	// Pass a literal NUL — url.Values{}.Encode() will percent-encode to %00,
	// and r.URL.Query().Get("value") decodes it back to a real null byte.
	req := probeRequest(t, session.ID, "file", "/etc/ultron-ap\x00.env")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var result probeResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.False(t, result.OK)
	assert.Equal(t, "file not found", result.Reason)
}

// Defensive: env-name with weird chars rejected (no echo of attacker input).
func TestProbe_EnvScheme_RejectsUnsafeName(t *testing.T) {
	srv, session := setupSSETestServer(t)
	req := probeRequest(t, session.ID, "env", "weird;NAME$")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var result probeResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.False(t, result.OK)
	// MUST NOT echo the unsafe name back.
	assert.False(t, strings.Contains(result.Reason, "weird"))
	assert.Equal(t, "env var not set", result.Reason)
}
