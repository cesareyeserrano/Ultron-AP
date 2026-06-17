package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/render"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/storm"
)

const telegramAPIBase = "https://api.telegram.org/bot"

// TelegramSender sends notifications via the Telegram Bot API.
//
// It uses the new render package to compose MarkdownV2 message bodies and the
// storm.Cache to coalesce rapid same-rule fires into editMessageText updates
// (FR-024). Send / Notify are the two public entry points; both end up
// calling sendOrEdit.
type TelegramSender struct {
	botToken string
	chatID   string
	client   *http.Client
	storm    *storm.Cache // optional; nil ⇒ no storm protection (legacy Send)
	now      func() time.Time
}

// NewTelegramSender creates a Telegram notifier with the default 30-second
// HTTP timeout and a fresh storm cache.
func NewTelegramSender(botToken, chatID string) *TelegramSender {
	return NewTelegramSenderWithTimeout(botToken, chatID, 30*time.Second)
}

// NewTelegramSenderWithTimeout is exposed for tests that need to bypass the
// default 30s budget.
func NewTelegramSenderWithTimeout(botToken, chatID string, timeout time.Duration) *TelegramSender {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &TelegramSender{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: timeout},
		storm:    storm.New(time.Now),
		now:      time.Now,
	}
}

// Name returns the channel identifier used by the dispatcher.
func (t *TelegramSender) Name() string { return "telegram" }

// StartJanitor runs the storm cache's periodic sweep in a goroutine until stop
// is closed. No-op when this sender has no storm cache. The dispatcher starts
// exactly one janitor per long-lived sender so storm coalescing state is
// actually reaped over time (BL-030).
func (t *TelegramSender) StartJanitor(stop <-chan struct{}) {
	if t.storm == nil {
		return
	}
	go t.storm.RunJanitor(stop)
}

// Send is the legacy entry point. It synthesises a minimal Event from the
// bare alert and delegates to Notify so legacy callers (and tests) still
// reach the new render path.
//
// @aitri-trace ADR-05
func (t *TelegramSender) Send(alert *database.Alert) error {
	if t.botToken == "" || t.chatID == "" {
		return fmt.Errorf("telegram not configured")
	}
	evt := eventFromAlert(alert)
	return t.Notify(context.Background(), evt)
}

// Notify sends the rich-context fire / resolve event to Telegram. On a fire
// it consults storm.Cache to decide between sendMessage and editMessageText.
//
// @aitri-trace FR-024 FR-016 FR-017 FR-018 FR-019 FR-023 FR-025 FR-028
func (t *TelegramSender) Notify(ctx context.Context, evt *Event) error {
	if t == nil {
		return fmt.Errorf("telegram: nil sender")
	}
	if t.botToken == "" || t.chatID == "" {
		return fmt.Errorf("telegram not configured")
	}
	if evt == nil || evt.Alert == nil {
		return fmt.Errorf("telegram: nil event/alert")
	}

	in := buildRenderInputFromEvent(evt)
	out := render.Render(in)

	// Storm decision (fires only). Resolves always create a fresh chat row.
	ruleID := ruleIDForEvent(evt)
	if evt.Kind == EventFire && t.storm != nil && ruleID != 0 {
		d := t.storm.Decide(ruleID)
		if !d.Send {
			// Edit existing chat row in place.
			body := injectFireCount(out.TelegramMD, d.FireCount)
			err := t.editMessage(ctx, d.EditTarget, body)
			if err == nil {
				t.storm.RecordEdit(ruleID)
				return nil
			}
			// Spurious "not modified": treat as success per FR-024 AC-024-005.
			if isMessageNotModifiedErr(err) {
				return nil
			}
			// "Message to edit not found": clear cache and fall through to a
			// fresh sendMessage so the operator still gets the alert.
			if isMessageNotFoundErr(err) {
				t.storm.Clear(ruleID)
			} else {
				return err
			}
		}
	}

	// Fresh sendMessage path.
	msgID, err := t.sendMessageReturningID(ctx, out.TelegramMD)
	if err != nil {
		return err
	}
	if evt.Kind == EventFire && t.storm != nil && ruleID != 0 {
		t.storm.RecordSend(ruleID, msgID)
	}
	if evt.Kind == EventResolve && t.storm != nil && ruleID != 0 {
		t.storm.Clear(ruleID)
	}
	return nil
}

// SendFile is unchanged from the legacy implementation — it is used by the
// backup-download flow (FR-015). Kept verbatim to avoid coupling that
// feature with this refactor.
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

	endpoint := telegramAPIBase + t.botToken + "/sendDocument"
	req, err := http.NewRequest("POST", endpoint, body)
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

// sendMessage is preserved for backwards compat with TestTelegramSender_*
// tests. New code should call sendMessageReturningID.
func (t *TelegramSender) sendMessage(text string) error {
	endpoint := telegramAPIBase + t.botToken + "/sendMessage"
	return t.sendMessageTo(endpoint, text)
}

// sendMessageTo posts a sendMessage payload to an arbitrary endpoint. Used
// by tests that inject a stubbed http.Client.
func (t *TelegramSender) sendMessageTo(endpoint string, text string) error {
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}
	payload := map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "MarkdownV2",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal error: %w", err)
	}
	resp, err := t.client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: request error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: API returned %d", resp.StatusCode)
	}
	return nil
}

// sendMessageReturningID is the production sendMessage path — it returns the
// message_id so the storm cache can record it.
// SendText posts a plain-text message (no MarkdownV2 parse mode, so arbitrary AI
// output needs no escaping). Used by the additive AI follow-up (FR-026).
func (t *TelegramSender) SendText(ctx context.Context, text string) error {
	if t.botToken == "" || t.chatID == "" {
		return fmt.Errorf("telegram not configured")
	}
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}
	payload := map[string]string{"chat_id": t.chatID, "text": text}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal error: %w", err)
	}
	endpoint := telegramAPIBase + t.botToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: request build error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: send error: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram: status %d", resp.StatusCode)
	}
	return nil
}

func (t *TelegramSender) sendMessageReturningID(ctx context.Context, text string) (int64, error) {
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}
	payload := map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "MarkdownV2",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("telegram: marshal error: %w", err)
	}
	endpoint := telegramAPIBase + t.botToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("telegram: request build error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("telegram: request error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("telegram: API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
	}
	var parsed struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		// Non-fatal: success status but unparseable body. Log it so a
		// silently-degraded storm cache (no message_id ⇒ no edit-in-place
		// coalescing) is visible, then return 0 so the cache skips the entry.
		log.Printf("telegram: sent OK but failed to decode response body: %v", err)
		return 0, nil
	}
	return parsed.Result.MessageID, nil
}

// editMessage calls editMessageText against an existing chat row.
func (t *TelegramSender) editMessage(ctx context.Context, messageID int64, text string) error {
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}
	payload := map[string]any{
		"chat_id":    t.chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "MarkdownV2",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal error: %w", err)
	}
	endpoint := telegramAPIBase + t.botToken + "/editMessageText"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: request build error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: request error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("telegram: editMessageText %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
}

// FormatAlertMessage is preserved for legacy callers. It now delegates to
// render.Render via a synthesised Event so output is consistent with the new
// surface contract.
//
// @aitri-trace ADR-05
func FormatAlertMessage(alert *database.Alert) string {
	if alert == nil {
		return ""
	}
	evt := eventFromAlert(alert)
	in := buildRenderInputFromEvent(evt)
	return render.Render(in).TelegramMD
}

// severityEmoji is preserved for the existing TestSeverityEmoji test. The
// new render package has its own copy; this stays here for backwards compat.
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

// buildRenderInputFromEvent maps an Event to render.Input. Surface-specific
// data (systemd, docker, trend) and the cause line are NOT populated here —
// those are filled in by the dispatcher once it has fetched journals / proc
// snapshots / etc. The legacy Send path uses this with all those fields nil,
// producing a clean "minimal" message.
func buildRenderInputFromEvent(evt *Event) render.Input {
	if evt == nil {
		return render.Input{}
	}
	in := render.Input{
		Alert:        evt.Alert,
		Rule:         evt.Rule,
		Kind:         evt.renderKind(),
		Surface:      evt.renderSurface(),
		FirstFiredAt: evt.FirstFiredAt,
		ResolvedAt:   evt.ResolvedAt,
		Hostname:     evt.Hostname,
		PublicURL:    evt.PublicURL,
		Trend:        evt.Trend,
		Cause:        evt.Cause,
		Systemd:      evt.Systemd,
		Docker:       evt.Docker,
	}
	return in
}

// eventFromAlert synthesises a minimal Event for the legacy Send path. The
// dispatcher's real Notify path provides a fully-populated Event with a
// resolved Rule and process-cached Hostname / PublicURL.
func eventFromAlert(alert *database.Alert) *Event {
	host, _ := os.Hostname()
	publicURL := strings.TrimRight(os.Getenv("ULTRON_PUBLIC_URL"), "/")
	if publicURL == "" {
		publicURL = "http://localhost"
	}
	return &Event{
		Alert:     alert,
		Kind:      EventFire,
		Surface:   SurfaceFromSource(safeAlertSource(alert)),
		Hostname:  host,
		PublicURL: publicURL,
	}
}

func safeAlertSource(a *database.Alert) string {
	if a == nil {
		return ""
	}
	return a.Source
}

// ruleIDForEvent returns the rule ID for storm-cache keying. Falls back to
// the alert ID when the rule ID is unavailable so transient state-change
// alerts (docker / systemd, which today have no ConfigID) still benefit
// from per-source coalescing.
func ruleIDForEvent(evt *Event) int64 {
	if evt == nil || evt.Alert == nil {
		return 0
	}
	if evt.Alert.ConfigID != nil && *evt.Alert.ConfigID > 0 {
		return *evt.Alert.ConfigID
	}
	// Fallback: hash the source to a stable int64 so docker/systemd
	// transitions still collapse storms within a 60-second window.
	return int64(hashSource(evt.Alert.Source))
}

// hashSource is a tiny FNV-1a 32-bit hash. We avoid hash/fnv to keep the
// notify package self-contained; the implementation is a one-liner.
func hashSource(s string) uint32 {
	const prime = 16777619
	h := uint32(2166136261)
	for _, b := range []byte(s) {
		h ^= uint32(b)
		h *= prime
	}
	return h
}

// injectFireCount appends "(N fires)" to the subject line of body. The
// renderer doesn't know about storm state, so the dispatcher mutates the
// already-rendered MarkdownV2 in place. We splice between the first line and
// the rest.
func injectFireCount(body string, n int) string {
	if n < 2 {
		return body
	}
	idx := strings.Index(body, "\n")
	if idx < 0 {
		return body + fmt.Sprintf(" \\(%d fires\\)", n)
	}
	subject := body[:idx]
	rest := body[idx:]
	suffix := fmt.Sprintf(" \\(%d fires\\)", n)
	return subject + suffix + rest
}

// isMessageNotModifiedErr returns true when err is the spurious "message is
// not modified" 400 from editMessageText (FR-024 AC-024-005).
func isMessageNotModifiedErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "message is not modified")
}

// isMessageNotFoundErr returns true when Telegram reports the cached
// message_id has been deleted by the user (FR-024 risk-flag mitigation).
func isMessageNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "message to edit not found")
}

// _ keeps url imported for future use.
var _ = url.Parse
