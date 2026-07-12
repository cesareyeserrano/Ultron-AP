package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/render"
)

// smtpTimeout bounds the whole SMTP conversation (dial + handshake + send).
const smtpTimeout = 10 * time.Second

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

// sendMailFunc is the indirection point used by tests. The real implementation
// is context-aware so an abandoned send doesn't leak its goroutine/connection.
var sendMailFunc = sendMailContext

// sendMail runs the SMTP send under an overall timeout. Unlike the old
// time.After race against a non-cancelable smtp.SendMail, the send now honors
// ctx: sendMailContext closes the connection when ctx is done, so the worker
// goroutine unblocks promptly instead of lingering on a dead socket (BL-027).
func (e *EmailSender) sendMail(ctx context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, smtpTimeout)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- sendMailFunc(ctx, addr, auth, from, to, msg)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sendMailContext is a context-aware replacement for smtp.SendMail. It mirrors
// the stdlib flow (dial → STARTTLS if offered → AUTH → MAIL/RCPT/DATA) but
// dials with the context and closes the connection when the context is done so
// a blocked read/write returns instead of hanging until the OS socket times out.
func sendMailContext(ctx context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("email: dial: %w", err)
	}
	// Watcher closes the conn on ctx done so an in-flight SMTP call unblocks.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	host, _, _ := net.SplitHostPort(addr)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("email: smtp client: %w", err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("email: starttls: %w", err)
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("email: auth: %w", err)
			}
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("email: MAIL: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("email: RCPT %s: %w", rcpt, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close body: %w", err)
	}
	return c.Quit()
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
// sanitizeHeader strips CR/LF and other control characters (except tab) from an
// email header value to prevent header injection (B4). From/To/Subject come
// from operator settings and alert content; a value containing "\r\n" could
// otherwise smuggle extra headers such as Bcc: into the message.
func sanitizeHeader(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		if r < 0x20 && r != '\t' {
			return -1
		}
		return r
	}, v)
}

func buildMIMEMessage(from, to, subject, plain, html string) []byte {
	from = sanitizeHeader(from)
	to = sanitizeHeader(to)
	subject = sanitizeHeader(subject)
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
