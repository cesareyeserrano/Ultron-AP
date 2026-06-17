// Module: ai/client
// Purpose: OpenAI-compatible Chat Completions HTTP client over HTTPS. One contract
//          covers Ollama Cloud / OpenAI / GLM / vLLM, so the provider is swappable
//          via config alone (FR-020, ADR-01).
// Dependencies: net/http, encoding/json.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeoutMS is the fallback per-request bound when config omits one.
const DefaultTimeoutMS = 10000

// maxResponseBytes caps how much of a provider response we read, defending
// against a hostile/huge body.
const maxResponseBytes = 1 << 20 // 1 MiB

// HTTPDoer is the minimal HTTP contract (satisfied by *http.Client). Injectable
// so tests can supply a stub provider with no real network.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client speaks the OpenAI-compatible /chat/completions API.
type Client struct {
	http HTTPDoer
}

// NewClient builds a Client. A nil doer falls back to a default *http.Client; the
// per-request deadline is carried by the request context, not the client.
func NewClient(hc HTTPDoer) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{http: hc}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// buildRequest constructs the POST <endpoint>/chat/completions request with the
// model, system+user messages, and Bearer auth (exposed for unit testing — FR-020).
//
// @aitri-trace FR-ID: FR-020, US-ID: US-020, AC-ID: AC-020-1h, TC-ID: TC-AI-020e
func buildRequest(ctx context.Context, cfg Config, system, user string) (*http.Request, error) {
	body, err := json.Marshal(chatRequest{
		Model:    cfg.Model,
		Messages: []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Stream:   false,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	url := strings.TrimRight(cfg.EndpointURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	return req, nil
}

// Complete sends the prompt and returns the assistant message content. Provider
// faults (non-2xx, malformed body, timeout) are returned as typed, redacted
// errors — never a panic (FR-021).
func (c *Client) Complete(ctx context.Context, cfg Config, system, user string) (string, error) {
	req, err := buildRequest(ctx, cfg, system, user)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("provider error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("provider error: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("provider error: %d %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("provider error: malformed response body")
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("provider error: empty response")
	}
	return parsed.Choices[0].Message.Content, nil
}
