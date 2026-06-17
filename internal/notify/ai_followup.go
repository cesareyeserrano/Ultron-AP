// Module: notify/ai_followup
// Purpose: Additive, best-effort AI explanation pushed to Telegram after a
//          rule-based alert fires (FR-026). Never blocks, drops, or duplicates the
//          rule-based alert; on any AI failure it sends nothing and logs.
// Dependencies: internal/ai, internal/database, internal/notify/render.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/ai"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/render"
)

// aiExplainer is the slice of the ai.Service the follow-up needs (injectable).
type aiExplainer interface {
	Explain(ctx context.Context, scope ai.Scope) (*ai.Explanation, error)
}

// AIFollowup builds and sends the additive AI Telegram note. All collaborators are
// injected so it is fully testable without a network.
type AIFollowup struct {
	svc      aiExplainer
	pushGate func() bool                              // true ⇒ AI enabled and Telegram push on
	send     func(ctx context.Context, text string) error // delivers the message
	timeout  time.Duration
	logf     func(string, ...any)
}

// NewAIFollowup wires the testable core.
func NewAIFollowup(svc aiExplainer, pushGate func() bool, send func(context.Context, string) error, timeout time.Duration) *AIFollowup {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &AIFollowup{svc: svc, pushGate: pushGate, send: send, timeout: timeout, logf: log.Printf}
}

// Run is the hook invoked by the dispatcher (already in its own goroutine). It is
// best-effort: any miss results in no message, only a log line.
//
// @aitri-trace FR-ID: FR-026, US-ID: US-026, AC-ID: AC-026-1h, TC-ID: TC-AI-026h
func (f *AIFollowup) Run(evt *Event) {
	if f == nil || f.svc == nil || f.send == nil || evt == nil {
		return
	}
	if f.pushGate != nil && !f.pushGate() {
		return // push disabled or AI off ⇒ only the rule-based alert was sent
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	exp, err := f.svc.Explain(ctx, ai.Scope{Kind: ai.ScopeSystem})
	if err != nil || exp == nil {
		f.logf("notify: ai follow-up skipped: %v", err)
		return
	}
	if err := f.send(ctx, f.format(evt, exp)); err != nil {
		f.logf("notify: ai follow-up send failed: %v", err)
	}
}

// format renders the follow-up message body, prefixed with the same category glyph
// used on the alert header.
func (f *AIFollowup) format(evt *Event, exp *ai.Explanation) string {
	cat := ""
	if evt.Rule != nil && evt.Rule.Metric != "" {
		cat = evt.Rule.Metric
	} else if evt.Alert != nil {
		cat = evt.Alert.Source
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🤖 AI analysis · %s\n", render.CategoryGlyph(cat))
	b.WriteString(exp.Cause)
	if strings.TrimSpace(exp.Remediation) != "" {
		b.WriteString("\n\n")
		b.WriteString(exp.Remediation)
	}
	if len(exp.CitedSignals) > 0 {
		fmt.Fprintf(&b, "\n\nsignals: %s", strings.Join(exp.CitedSignals, ", "))
	}
	return b.String()
}

// NewAIFollowupFromDB builds the production follow-up: the push gate reads the AI
// settings, and the sender is constructed from the stored Telegram config. Returns
// nil collaborators gracefully (a missing Telegram config just means no message).
func NewAIFollowupFromDB(db *database.DB, svc aiExplainer) *AIFollowup {
	gate := func() bool {
		cfg, err := db.GetAISettings()
		if err != nil {
			return false
		}
		return cfg.Enabled && cfg.TelegramPush
	}
	send := func(ctx context.Context, text string) error {
		token, chat, ok := telegramCredsFromDB(db)
		if !ok {
			return fmt.Errorf("telegram not configured")
		}
		return NewTelegramSender(token, chat).SendText(ctx, text)
	}
	return NewAIFollowup(svc, gate, send, 0)
}

// telegramCredsFromDB reads the bot token + chat id from the telegram notification
// config row.
func telegramCredsFromDB(db *database.DB) (token, chat string, ok bool) {
	nc, err := db.GetNotificationConfig("telegram")
	if err != nil || nc == nil || !nc.Enabled {
		return "", "", false
	}
	var fields struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	if err := json.Unmarshal([]byte(nc.Config), &fields); err != nil {
		return "", "", false
	}
	if fields.BotToken == "" || fields.ChatID == "" {
		return "", "", false
	}
	return fields.BotToken, fields.ChatID, true
}
