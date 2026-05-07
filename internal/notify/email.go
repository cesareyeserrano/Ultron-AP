package notify

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/render"
)

// EmailSender sends notifications via SMTP. Body content is produced by the
// shared render package so Telegram and Email surfaces stay in lock-step.
//
// @aitri-trace FR-027
type EmailSender struct {
	host     string
	port     string
	user     string
	password string
	from     string
	to       string
}

// NewEmailSender creates an email notifier.
func NewEmailSender(host, port, user, password, from, to string) *EmailSender {
	return &EmailSender{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		from:     from,
		to:       to,
	}
}

// Name returns the channel identifier.
func (e *EmailSender) Name() string { return "email" }

// Send is the legacy entry point; delegates to Notify with a synthesised
// minimal Event so output stays consistent across surfaces.
func (e *EmailSender) Send(alert *database.Alert) error {
	if e.host == "" || e.from == "" || e.to == "" {
		return fmt.Errorf("email not configured")
	}
	return e.Notify(context.Background(), eventFromAlert(alert))
}

// Notify sends the rich-context fire / resolve event as a multipart/
// alternative email (HTML + plain-text fallback per RFC 2046).
//
// @aitri-trace FR-027 AC-027-001 AC-027-002
func (e *EmailSender) Notify(ctx context.Context, evt *Event) error {
	if e.host == "" || e.from == "" || e.to == "" {
		return fmt.Errorf("email not configured")
	}
	if evt == nil || evt.Alert == nil {
		return fmt.Errorf("email: nil event/alert")
	}

	in := buildRenderInputFromEvent(evt)
	out := render.Render(in)

	subject := out.EmailSubject
	if subject == "" {
		subject = fmt.Sprintf("[Ultron-AP] %s: %s", strings.ToUpper(evt.Alert.Severity), evt.Alert.Source)
	}
	msg := buildMIMEMessage(e.from, e.to, subject, out.EmailPlain, out.EmailHTML)

	addr := net.JoinHostPort(e.host, e.port)
	var auth smtp.Auth
	if e.user != "" && e.password != "" {
		auth = smtp.PlainAuth("", e.user, e.password, e.host)
	}
	return e.sendMail(ctx, addr, auth, e.from, []string{e.to}, msg)
}

// sendMailFunc is the indirection point used by tests.
var sendMailFunc = smtp.SendMail

// sendMail wraps smtp.SendMail with a 10-second timeout. The ctx parameter
// is honored for cancellation; on ctx done we drop the in-flight goroutine
// (the smtp call itself isn't context-aware).
func (e *EmailSender) sendMail(ctx context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- sendMailFunc(addr, auth, from, to, msg)
	}()
	select {
	case err := <-errCh:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("email: send timeout after 10s")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// formatEmailSubject is preserved for callers that need just the header.
// The Notify path uses the render package's EmailSubject directly; this
// helper now wraps it for backwards compat with code that referenced the
// old function.
func formatEmailSubject(alert *database.Alert) string {
	if alert == nil {
		return ""
	}
	in := buildRenderInputFromEvent(eventFromAlert(alert))
	subj := render.Render(in).EmailSubject
	if subj == "" {
		subj = fmt.Sprintf("[Ultron-AP] %s: %s", strings.ToUpper(alert.Severity), alert.Source)
	}
	return subj
}

// formatEmailBody is preserved for backwards compat. Returns the plain-text
// body produced by the renderer.
func formatEmailBody(alert *database.Alert) string {
	if alert == nil {
		return ""
	}
	in := buildRenderInputFromEvent(eventFromAlert(alert))
	return render.Render(in).EmailPlain
}

// buildMIMEMessage produces a multipart/alternative MIME body so legacy mail
// clients see the plain-text fallback and modern clients see HTML.
//
// The signature accepts plain and html separately; legacy callers passing a
// single body should set html="" — the resulting message is a plain-only
// text/plain message identical to the old format.
//
// @aitri-trace FR-027 AC-027-002
func buildMIMEMessage(from, to, subject, plain, html string) []byte {
	if html == "" {
		// Backwards-compat: plain-only message.
		var b strings.Builder
		b.WriteString(fmt.Sprintf("From: %s\r\n", from))
		b.WriteString(fmt.Sprintf("To: %s\r\n", to))
		b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
		b.WriteString("MIME-Version: 1.0\r\n")
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("\r\n")
		b.WriteString(plain)
		return []byte(b.String())
	}

	const boundary = "ultron-ap-mixed-boundary-==9f8e7d6c5b4a"
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s\r\n", from))
	b.WriteString(fmt.Sprintf("To: %s\r\n", to))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n", boundary))
	b.WriteString("\r\n")
	// plain part
	b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(plain)
	b.WriteString("\r\n")
	// html part
	b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(html)
	b.WriteString("\r\n")
	b.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	return []byte(b.String())
}
