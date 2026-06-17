// Module: server/handlers_ai
// Purpose: Authenticated HTTP surface for the ai-insights feature — explanation
//          requests, provider test, and the Settings AI config (key masked on read).
// Dependencies: internal/ai, internal/database, net/http, encoding/json.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cesareyeserrano/ultron-ap/internal/ai"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// SetAI injects the AI service (constructed in main with the read-only telemetry
// sources). A nil service means the feature is unavailable and endpoints degrade
// gracefully (FR-019).
func (s *Server) SetAI(svc *ai.Service) { s.aiSvc = svc }

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleAIExplain generates an explanation for the current state or a fired
// insight and returns it as JSON. Provider faults map to typed statuses, never 500.
//
// @aitri-trace FR-ID: FR-016, US-ID: US-016, AC-ID: AC-016-1h, TC-ID: TC-AI-016h
func (s *Server) handleAIExplain(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	if s.aiSvc == nil || !s.aiSvc.Enabled() {
		writeJSONError(w, http.StatusConflict, "AI not configured")
		return
	}
	var body struct {
		Scope     string `json:"scope"`
		InsightID any    `json:"insight_id"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)

	scope := ai.Scope{Kind: ai.ScopeSystem}
	if body.Scope == string(ai.ScopeInsight) {
		scope.Kind = ai.ScopeInsight
		scope.InsightID = anyToString(body.InsightID)
	}

	exp, err := s.aiSvc.Explain(r.Context(), scope)
	if err != nil {
		s.writeAIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, exp)
}

// handleAITest checks reachability of a provider config without persisting it.
//
// @aitri-trace FR-ID: FR-025, US-ID: US-018, AC-ID: AC-018-1h, TC-ID: TC-AI-020f
func (s *Server) handleAITest(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	if s.aiSvc == nil {
		writeJSONError(w, http.StatusConflict, "AI not available")
		return
	}
	var body struct {
		EndpointURL string `json:"endpoint_url"`
		Model       string `json:"model"`
		APIKey      string `json:"api_key"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body)

	stored, _ := s.db.GetAISettings()
	cfg := ai.Config{
		EndpointURL: strings.TrimSpace(body.EndpointURL),
		Model:       strings.TrimSpace(body.Model),
		APIKey:      body.APIKey,
		TimeoutMS:   stored.TimeoutMS,
	}
	if cfg.EndpointURL == "" {
		cfg.EndpointURL = stored.EndpointURL
	}
	if cfg.Model == "" {
		cfg.Model = stored.Model
	}
	if cfg.APIKey == "" {
		cfg.APIKey = stored.APIKey
	}

	model, err := s.aiSvc.Test(r.Context(), cfg)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "resolved_model": model})
}

// aiSettingsView is the GET shape — the raw key is never included (FR-017).
type aiSettingsView struct {
	Enabled       bool   `json:"enabled"`
	EndpointURL   string `json:"endpoint_url"`
	Model         string `json:"model"`
	TelegramPush  bool   `json:"telegram_push"`
	TimeoutMS     int    `json:"timeout_ms"`
	AllowInsecure bool   `json:"allow_insecure"`
	APIKeySet     bool   `json:"api_key_set"`
	APIKey        string `json:"api_key"` // always empty; present so clients see a stable shape
}

// handleAISettingsGet returns the AI config with the key masked (FR-017).
//
// @aitri-trace FR-ID: FR-017, US-ID: US-017, AC-ID: AC-017-1f, TC-ID: TC-AI-017f
func (s *Server) handleAISettingsGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.db.GetAISettings()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "cannot read AI settings")
		return
	}
	keySet, _ := s.db.AIKeyIsSet()
	writeJSON(w, http.StatusOK, aiSettingsView{
		Enabled:       cfg.Enabled,
		EndpointURL:   cfg.EndpointURL,
		Model:         cfg.Model,
		TelegramPush:  cfg.TelegramPush,
		TimeoutMS:     cfg.TimeoutMS,
		AllowInsecure: cfg.AllowInsecure,
		APIKeySet:     keySet,
		APIKey:        "",
	})
}

// handleAISettingsSave validates and persists the AI config. Accepts either a
// form POST (htmx) or a JSON body. An empty api_key keeps the stored key (FR-018).
//
// @aitri-trace FR-ID: FR-018, US-ID: US-018, AC-ID: AC-018-1f, TC-ID: TC-AI-018f
func (s *Server) handleAISettingsSave(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	in := s.parseAISettingsInput(r)
	asJSON := strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")

	fail := func(status int, msg string) {
		if asJSON {
			writeJSONError(w, status, msg)
			return
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, msg))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		fmt.Fprintf(w, `<div class="text-sm text-[color:var(--color-error-text)] py-2">%s</div>`, html.EscapeString(msg))
	}

	if in.Enabled && strings.TrimSpace(in.EndpointURL) == "" {
		fail(http.StatusBadRequest, "Endpoint URL is required when AI is enabled")
		return
	}
	keySet, _ := s.db.AIKeyIsSet()
	if in.Enabled && in.APIKey == "" && !keySet {
		fail(http.StatusBadRequest, "API key is required when AI is enabled")
		return
	}
	if in.EndpointURL != "" {
		u, perr := url.Parse(in.EndpointURL)
		if perr != nil || u.Scheme == "" || u.Host == "" {
			fail(http.StatusBadRequest, "Endpoint URL is not a valid URL")
			return
		}
		if u.Scheme != "https" && !in.AllowInsecure {
			fail(http.StatusBadRequest, "Endpoint must be https (or enable insecure override)")
			return
		}
	}
	if in.TimeoutMS < 1000 || in.TimeoutMS > 60000 {
		in.TimeoutMS = database.DefaultAITimeoutMS
	}

	if err := s.db.SaveAISettings(in); err != nil {
		fail(http.StatusInternalServerError, "cannot save AI settings")
		return
	}
	keySet, _ = s.db.AIKeyIsSet()
	if asJSON {
		writeJSON(w, http.StatusOK, aiSettingsView{
			Enabled: in.Enabled, EndpointURL: in.EndpointURL, Model: in.Model,
			TelegramPush: in.TelegramPush, TimeoutMS: in.TimeoutMS, AllowInsecure: in.AllowInsecure,
			APIKeySet: keySet,
		})
		return
	}
	w.Header().Set("HX-Trigger", `{"showToast":{"message":"AI settings saved","type":"success"}}`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<div class="text-sm text-green-400 py-2">Saved</div>`))
}

func (s *Server) parseAISettingsInput(r *http.Request) database.AISettings {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		var b struct {
			Enabled       bool   `json:"enabled"`
			EndpointURL   string `json:"endpoint_url"`
			Model         string `json:"model"`
			APIKey        string `json:"api_key"`
			TelegramPush  bool   `json:"telegram_push"`
			TimeoutMS     int    `json:"timeout_ms"`
			AllowInsecure bool   `json:"allow_insecure"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&b)
		return database.AISettings{
			Enabled: b.Enabled, EndpointURL: strings.TrimSpace(b.EndpointURL), Model: strings.TrimSpace(b.Model),
			APIKey: b.APIKey, TelegramPush: b.TelegramPush, TimeoutMS: b.TimeoutMS, AllowInsecure: b.AllowInsecure,
		}
	}
	timeout, _ := strconv.Atoi(r.FormValue("timeout_ms"))
	if timeout == 0 {
		timeout = database.DefaultAITimeoutMS
	}
	return database.AISettings{
		Enabled:       r.FormValue("enabled") == "on" || r.FormValue("enabled") == "true",
		EndpointURL:   strings.TrimSpace(r.FormValue("endpoint_url")),
		Model:         strings.TrimSpace(r.FormValue("model")),
		APIKey:        r.FormValue("api_key"),
		TelegramPush:  r.FormValue("telegram_push") == "on" || r.FormValue("telegram_push") == "true",
		TimeoutMS:     timeout,
		AllowInsecure: r.FormValue("allow_insecure") == "on" || r.FormValue("allow_insecure") == "true",
	}
}

// writeAIError maps ai sentinel/provider errors to HTTP statuses (FR-019/021).
func (s *Server) writeAIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ai.ErrNotConfigured):
		writeJSONError(w, http.StatusConflict, "AI not configured")
	case errors.Is(err, ai.ErrInsufficient):
		writeJSONError(w, http.StatusUnprocessableEntity, "insufficient telemetry to explain")
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			writeJSONError(w, http.StatusGatewayTimeout, "provider timed out")
			return
		}
		writeJSONError(w, http.StatusBadGateway, err.Error())
	}
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}
