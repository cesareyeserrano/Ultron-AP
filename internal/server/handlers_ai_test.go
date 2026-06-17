package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/ai"
	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/insights"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
)

// --- test doubles -----------------------------------------------------------

type aiStubDoer struct {
	status   int
	body     string
	err      error
	lastBody string
}

func (d *aiStubDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		d.lastBody = string(b)
	}
	if d.err != nil {
		return nil, d.err
	}
	return &http.Response{StatusCode: d.status, Body: io.NopCloser(strings.NewReader(d.body)), Header: make(http.Header)}, nil
}

type aiFakeInsights struct{ v []insights.Verdict }

func (f aiFakeInsights) Active() []insights.Verdict { return f.v }

type aiFakeMetrics struct{ s *metrics.Snapshot }

func (f aiFakeMetrics) Latest() *metrics.Snapshot { return f.s }

func aiOKBody(cause, rem string, signals []string) string {
	obj := map[string]any{"cause": cause, "remediation": rem, "cited_signals": signals}
	inner, _ := json.Marshal(obj)
	out, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": string(inner)}}},
	})
	return string(out)
}

func setupAIServer(t *testing.T, doer ai.HTTPDoer, sources ai.Sources, settings *database.AISettings) (*Server, *database.Session) {
	t.Helper()
	t.Setenv("ULTRON_SECRET_KEY", "server-ai-test-key")

	cfg := &config.Config{Port: 8080, DBPath: filepath.Join(t.TempDir(), "test.db"), LogLevel: "info", AdminUser: "admin", AdminPass: "secret", SessionTTL: 24 * time.Hour}
	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.CreateUser("admin", "$2a$10$dummy"))

	session := &database.Session{ID: "ai-test-session", UserID: 1, CSRFToken: "test-csrf", ExpiresAt: time.Now().Add(24 * time.Hour)}
	require.NoError(t, db.CreateSession(session))

	if settings != nil {
		require.NoError(t, db.SaveAISettings(*settings))
	}

	srv := New(cfg, db, nil, nil, nil, nil, nil)
	srv.SetAI(ai.NewFromDB(db, sources, doer))
	return srv, session
}

func aiPost(srv *Server, session *database.Session, path, jsonBody string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", session.CSRFToken)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func aiGet(srv *Server, session *database.Session, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func enabledSettings() *database.AISettings {
	return &database.AISettings{Enabled: true, EndpointURL: "https://prov.test/v1", Model: "model-a", APIKey: "sk-aikey9", TimeoutMS: 10000}
}

func insightSources(ruleID string) ai.Sources {
	return ai.Sources{Insights: aiFakeInsights{v: []insights.Verdict{{RuleID: ruleID, Severity: "critical", VerdictText: "cpu_temp high"}}}}
}

// --- TC-AI-016h -------------------------------------------------------------

// @aitri-tc TC-AI-016h
func TestTC_AI_016h_ExplainInsight(t *testing.T) {
	doer := &aiStubDoer{status: 200, body: aiOKBody("temp spiked from docker build", "throttle the build", []string{"cpu_temp", "rule_id=8"})}
	srv, sess := setupAIServer(t, doer, insightSources("8"), enabledSettings())

	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"insight","insight_id":8}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var exp ai.Explanation
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &exp))
	assert.NotEmpty(t, exp.Cause)
	assert.Contains(t, strings.Join(exp.CitedSignals, ","), "cpu_temp")
}

// --- TC-AI-016f -------------------------------------------------------------

// @aitri-tc TC-AI-016f
func TestTC_AI_016f_InsufficientTelemetry(t *testing.T) {
	doer := &aiStubDoer{status: 200, body: aiOKBody("x", "y", nil)}
	srv, sess := setupAIServer(t, doer, ai.Sources{}, enabledSettings()) // empty sources ⇒ insufficient

	before := actionLogCount(t, srv)
	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "insufficient")
	assert.Equal(t, before, actionLogCount(t, srv), "explanation must take no action (audit log unchanged)")
}

func actionLogCount(t *testing.T, srv *Server) int {
	t.Helper()
	var n int
	require.NoError(t, srv.db.QueryRow(`SELECT COUNT(*) FROM ActionLog`).Scan(&n))
	return n
}

// --- TC-AI-017f -------------------------------------------------------------

// @aitri-tc TC-AI-017f
func TestTC_AI_017f_KeyNeverReturnedOrLogged(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	doer := &aiStubDoer{status: 200, body: aiOKBody("c", "r", []string{"cpu"})}
	srv, sess := setupAIServer(t, doer, insightSources("1"), &database.AISettings{Enabled: true, EndpointURL: "https://prov.test/v1", Model: "m", APIKey: "sk-secret123", TimeoutMS: 10000})

	get := aiGet(srv, sess, "/api/settings/ai")
	require.Equal(t, http.StatusOK, get.Code)
	var view map[string]any
	require.NoError(t, json.Unmarshal(get.Body.Bytes(), &view))
	assert.Equal(t, "", view["api_key"])
	assert.Equal(t, true, view["api_key_set"])
	assert.NotContains(t, get.Body.String(), "sk-secret123")

	_ = aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.NotContains(t, logBuf.String(), "sk-secret123", "secret must never appear in logs")
}

// --- TC-AI-018h -------------------------------------------------------------

// @aitri-tc TC-AI-018h
func TestTC_AI_018h_ConfigPersists(t *testing.T) {
	srv, sess := setupAIServer(t, &aiStubDoer{status: 200, body: aiOKBody("c", "r", nil)}, ai.Sources{}, nil)

	save := aiPost(srv, sess, "/api/settings/ai", `{"enabled":true,"endpoint_url":"https://api.example/v1","model":"qwen2.5:14b","api_key":"sk-x","timeout_ms":10000}`)
	require.Equal(t, http.StatusOK, save.Code, save.Body.String())

	get := aiGet(srv, sess, "/api/settings/ai")
	var view map[string]any
	require.NoError(t, json.Unmarshal(get.Body.Bytes(), &view))
	assert.Equal(t, "https://api.example/v1", view["endpoint_url"])
	assert.Equal(t, "qwen2.5:14b", view["model"])
	assert.Equal(t, true, view["api_key_set"])
	assert.Equal(t, "", view["api_key"])
}

// --- TC-AI-018e -------------------------------------------------------------

// @aitri-tc TC-AI-018e
func TestTC_AI_018e_SettingsSectionRenders(t *testing.T) {
	srv, sess := setupAIServer(t, &aiStubDoer{status: 200, body: aiOKBody("c", "r", nil)}, ai.Sources{}, enabledSettings())
	rec := aiGet(srv, sess, "/settings")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `id="settings-ai"`)
	assert.Contains(t, body, `name="endpoint_url"`)
	assert.Contains(t, body, `name="model"`)
	assert.Contains(t, body, "min-h-[44px]", "mobile tap target sizing present")
}

// --- TC-AI-018f -------------------------------------------------------------

// @aitri-tc TC-AI-018f
func TestTC_AI_018f_EnableWithEmptyEndpointBlocked(t *testing.T) {
	srv, sess := setupAIServer(t, &aiStubDoer{status: 200, body: aiOKBody("c", "r", nil)}, ai.Sources{}, nil)

	rec := aiPost(srv, sess, "/api/settings/ai", `{"enabled":true,"endpoint_url":"","model":"m","api_key":"k"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Endpoint URL is required")

	get := aiGet(srv, sess, "/api/settings/ai")
	var view map[string]any
	require.NoError(t, json.Unmarshal(get.Body.Bytes(), &view))
	assert.Equal(t, false, view["enabled"], "partial config must not be saved")
}

// --- TC-AI-019h -------------------------------------------------------------

// @aitri-tc TC-AI-019h
func TestTC_AI_019h_UnconfiguredPanelClean(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	srv, sess := setupAIServer(t, &aiStubDoer{status: 200, body: aiOKBody("c", "r", nil)}, ai.Sources{}, nil) // no settings ⇒ disabled

	// Settings page renders normally with AI unconfigured.
	settings := aiGet(srv, sess, "/settings")
	require.Equal(t, http.StatusOK, settings.Code)
	// AI is reported not enabled, so the explain endpoint degrades cleanly (409, not 500).
	explain := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.Equal(t, http.StatusConflict, explain.Code)
	assert.NotContains(t, strings.ToLower(logBuf.String()), "ai: error", "no AI errors logged when unconfigured")
}

// --- TC-AI-019f -------------------------------------------------------------

// @aitri-tc TC-AI-019f
func TestTC_AI_019f_ExplainUnconfiguredReturns409(t *testing.T) {
	srv, sess := setupAIServer(t, &aiStubDoer{status: 200, body: aiOKBody("c", "r", nil)}, ai.Sources{}, nil)
	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code)

	health := aiGet(srv, sess, "/health")
	assert.Equal(t, http.StatusOK, health.Code)
}

// --- TC-AI-020h -------------------------------------------------------------

// @aitri-tc TC-AI-020h
func TestTC_AI_020h_ModelSwapNoRestart(t *testing.T) {
	doer := &aiStubDoer{status: 200, body: aiOKBody("c", "r", []string{"cpu"})}
	srv, sess := setupAIServer(t, doer, insightSources("1"), enabledSettings()) // model-a

	save := aiPost(srv, sess, "/api/settings/ai", `{"enabled":true,"endpoint_url":"https://prov.test/v1","model":"model-b","timeout_ms":10000}`)
	require.Equal(t, http.StatusOK, save.Code, save.Body.String())

	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, doer.lastBody, `"model":"model-b"`, "request must use the newly saved model with no restart")
}

// --- TC-AI-020f -------------------------------------------------------------

// @aitri-tc TC-AI-020f
func TestTC_AI_020f_UnreachableEndpointTypedError(t *testing.T) {
	doer := &aiStubDoer{err: io.ErrUnexpectedEOF}
	srv, sess := setupAIServer(t, doer, insightSources("1"), enabledSettings())

	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "provider error")

	health := aiGet(srv, sess, "/health")
	assert.Equal(t, http.StatusOK, health.Code, "process must keep serving after provider failure")
}

// --- TC-AI-021h -------------------------------------------------------------

// @aitri-tc TC-AI-021h
func TestTC_AI_021h_FastProviderResponsive(t *testing.T) {
	doer := &aiStubDoer{status: 200, body: aiOKBody("ok", "do x", []string{"cpu"})}
	srv, sess := setupAIServer(t, doer, insightSources("1"), enabledSettings())

	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.Equal(t, http.StatusOK, rec.Code)
	health := aiGet(srv, sess, "/health")
	assert.Equal(t, http.StatusOK, health.Code)
}

// --- TC-AI-021f -------------------------------------------------------------

// @aitri-tc TC-AI-021f
func TestTC_AI_021f_Provider401NoCrash(t *testing.T) {
	doer := &aiStubDoer{status: 401, body: "unauthorized"}
	srv, sess := setupAIServer(t, doer, insightSources("1"), enabledSettings())

	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "401")

	health := aiGet(srv, sess, "/health")
	assert.Equal(t, http.StatusOK, health.Code)
}

// --- TC-AI-022h -------------------------------------------------------------

// @aitri-tc TC-AI-022h
func TestTC_AI_022h_SuccessStateContract(t *testing.T) {
	doer := &aiStubDoer{status: 200, body: aiOKBody("docker build spiked load", "throttle the container", []string{"cpu_temp"})}
	srv, sess := setupAIServer(t, doer, insightSources("8"), enabledSettings())

	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var exp ai.Explanation
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &exp))
	assert.NotEmpty(t, exp.Cause)
	assert.NotEmpty(t, exp.Remediation)
}

// --- TC-AI-022e -------------------------------------------------------------

// @aitri-tc TC-AI-022e
func TestTC_AI_022e_EmptyStateContract(t *testing.T) {
	doer := &aiStubDoer{status: 200, body: aiOKBody("x", "y", nil)}
	srv, sess := setupAIServer(t, doer, ai.Sources{}, enabledSettings()) // insufficient ⇒ 422 drives empty state
	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

// --- TC-AI-022f -------------------------------------------------------------

// @aitri-tc TC-AI-022f
func TestTC_AI_022f_ErrorStateContract(t *testing.T) {
	doer := &aiStubDoer{status: 502, body: "bad gateway"}
	srv, sess := setupAIServer(t, doer, insightSources("1"), enabledSettings())
	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "error")
}

// --- TC-AI-023f -------------------------------------------------------------

// @aitri-tc TC-AI-023f
func TestTC_AI_023f_NoSecretsInPromptOrLogs(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	doer := &aiStubDoer{status: 200, body: aiOKBody("c", "r", []string{"cpu"})}
	// telemetry that embeds all three secret values
	secrets := []string{"bot999:AAtok", "smtpP@ss1", "sk-aikey9"}
	src := ai.Sources{Insights: aiFakeInsights{v: []insights.Verdict{{RuleID: "1", Severity: "warn", VerdictText: "leaked " + strings.Join(secrets, " ")}}}}
	srv, sess := setupAIServer(t, doer, src, enabledSettings())

	// seed telegram + email notification secrets so the redactor knows them
	tgJSON, _ := json.Marshal(map[string]string{"bot_token": "bot999:AAtok", "chat_id": "1"})
	require.NoError(t, srv.db.UpsertNotificationConfig(&database.NotificationConfig{Channel: "telegram", Enabled: true, Config: string(tgJSON)}))
	emJSON, _ := json.Marshal(map[string]string{"smtp_password": "smtpP@ss1"})
	require.NoError(t, srv.db.UpsertNotificationConfig(&database.NotificationConfig{Channel: "email", Enabled: true, Config: string(emJSON)}))

	rec := aiPost(srv, sess, "/api/ai/explain", `{"scope":"system"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	for _, s := range secrets {
		assert.NotContains(t, doer.lastBody, s, "secret leaked to provider prompt: "+s)
		assert.NotContains(t, logBuf.String(), s, "secret leaked to logs: "+s)
	}
}
