package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func makeJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFormatAlertMessage_Critical(t *testing.T) {
	value := 95.0
	alert := &database.Alert{
		Severity:  "critical",
		Message:   "High CPU: 95.0 > 90.0",
		Source:    "cpu",
		Value:     &value,
		CreatedAt: time.Date(2026, 2, 9, 14, 30, 0, 0, time.UTC),
	}

	msg := FormatAlertMessage(alert)
	assert.Contains(t, msg, "CRITICAL")
	assert.Contains(t, msg, "High CPU")
	assert.Contains(t, msg, "95.0")
	assert.Contains(t, msg, "cpu")
	assert.Contains(t, msg, "2026-02-09 14:30:00")
}

func TestFormatAlertMessage_Warning(t *testing.T) {
	alert := &database.Alert{
		Severity:  "warning",
		Message:   "Container nginx stopped",
		Source:    "docker:nginx",
		CreatedAt: time.Now(),
	}

	msg := FormatAlertMessage(alert)
	assert.Contains(t, msg, "WARNING")
	assert.Contains(t, msg, "nginx")
	assert.NotContains(t, msg, "Value:") // No value for docker alerts
}

func TestFormatAlertMessage_Info(t *testing.T) {
	alert := &database.Alert{
		Severity:  "info",
		Message:   "Test alert",
		Source:    "test",
		CreatedAt: time.Now(),
	}

	msg := FormatAlertMessage(alert)
	assert.Contains(t, msg, "INFO")
}

func TestSeverityEmoji(t *testing.T) {
	assert.NotEmpty(t, severityEmoji("critical"))
	assert.NotEmpty(t, severityEmoji("warning"))
	assert.NotEmpty(t, severityEmoji("info"))
	assert.NotEmpty(t, severityEmoji("unknown"))
	// Each should be different
	assert.NotEqual(t, severityEmoji("critical"), severityEmoji("warning"))
	assert.NotEqual(t, severityEmoji("warning"), severityEmoji("info"))
}

func TestTelegramSender_Send_Success(t *testing.T) {
	var receivedBody map[string]string

	sender := &TelegramSender{
		botToken: "test-token",
		chatID:   "12345",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				require.NoError(t, json.NewDecoder(req.Body).Decode(&receivedBody))
				return makeJSONResponse(http.StatusOK, `{"ok":true}`), nil
			}),
		},
	}

	err := sender.sendMessageTo("https://example.test/sendMessage", "Test message")
	require.NoError(t, err)
	assert.Equal(t, "12345", receivedBody["chat_id"])
	assert.Equal(t, "Test message", receivedBody["text"])
}

func TestTelegramSender_Send_APIError(t *testing.T) {
	sender := &TelegramSender{
		botToken: "bad-token",
		chatID:   "12345",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return makeJSONResponse(http.StatusUnauthorized, ""), nil
			}),
		},
	}
	err := sender.sendMessageTo("https://example.test/sendMessage", "Test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestTelegramSender_Send_NotConfigured(t *testing.T) {
	sender := NewTelegramSender("", "")
	alert := &database.Alert{Severity: "info", Message: "test", Source: "test"}
	err := sender.Send(alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestTelegramSender_MessageTruncation(t *testing.T) {
	sender := &TelegramSender{
		botToken: "token",
		chatID:   "123",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				var body map[string]string
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				assert.LessOrEqual(t, len(body["text"]), 4096)
				return makeJSONResponse(http.StatusOK, ""), nil
			}),
		},
	}

	// Create a message longer than 4096 chars
	longMsg := make([]byte, 5000)
	for i := range longMsg {
		longMsg[i] = 'A'
	}
	err := sender.sendMessageTo("https://example.test/sendMessage", string(longMsg))
	require.NoError(t, err)
}

func TestTelegramSender_Name(t *testing.T) {
	sender := NewTelegramSender("", "")
	assert.Equal(t, "telegram", sender.Name())
}
