package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

const telegramAPIBase = "https://api.telegram.org/bot"

// TelegramSender sends notifications via Telegram Bot API.
type TelegramSender struct {
	botToken string
	chatID   string
	client   *http.Client
}

// NewTelegramSender creates a Telegram notifier.
func NewTelegramSender(botToken, chatID string) *TelegramSender {
	return &TelegramSender{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 30 * time.Second}, // Increased timeout for file uploads
	}
}

func (t *TelegramSender) Name() string { return "telegram" }

// Send sends an alert message to Telegram.
func (t *TelegramSender) Send(alert *database.Alert) error {
	if t.botToken == "" || t.chatID == "" {
		return fmt.Errorf("telegram not configured")
	}

	msg := FormatAlertMessage(alert)
	return t.sendMessage(msg)
}

// SendFile sends a file as a document to Telegram.
func (t *TelegramSender) SendFile(filePath string, caption string) error {
	if t.botToken == "" || t.chatID == "" {
		return fmt.Errorf("telegram not configured")
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy file to part: %w", err)
	}

	if err := writer.WriteField("chat_id", t.chatID); err != nil {
		return fmt.Errorf("failed to write chat_id: %w", err)
	}
	if err := writer.WriteField("caption", caption); err != nil {
		return fmt.Errorf("failed to write caption: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	url := telegramAPIBase + t.botToken + "/sendDocument"
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (t *TelegramSender) sendMessage(text string) error {
	url := telegramAPIBase + t.botToken + "/sendMessage"
	return t.sendMessageTo(url, text)
}

func (t *TelegramSender) sendMessageTo(url string, text string) error {
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}

	payload := map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal error: %w", err)
	}

	resp, err := t.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: API returned %d", resp.StatusCode)
	}

	return nil
}

// FormatAlertMessage formats an alert into a Telegram message.
func FormatAlertMessage(alert *database.Alert) string {
	emoji := severityEmoji(alert.Severity)
	severity := strings.ToUpper(alert.Severity)

	msg := fmt.Sprintf("%s *%s ALERT*\n\n", emoji, severity)
	msg += fmt.Sprintf("*Message:* %s\n", alert.Message)
	msg += fmt.Sprintf("*Source:* `%s`\n", alert.Source)

	if alert.Value != nil {
		msg += fmt.Sprintf("*Current Value:* `%.1f`\n", *alert.Value)
	}

	msg += fmt.Sprintf("\n*Time:* %s", alert.CreatedAt.Format("2006-01-02 15:04:05"))

	return msg
}

func severityEmoji(severity string) string {
	switch severity {
	case "critical":
		return "\xf0\x9f\x94\xb4" // red circle
	case "warning":
		return "\xf0\x9f\x9f\xa1" // yellow circle
	case "info":
		return "\xf0\x9f\x94\xb5" // blue circle
	default:
		return "\xe2\x9a\xaa" // white circle
	}
}
