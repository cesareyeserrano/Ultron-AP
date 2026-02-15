package notify

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// EmailSender sends notifications via SMTP.
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

func (e *EmailSender) Name() string { return "email" }

// Send sends an alert email.
func (e *EmailSender) Send(alert *database.Alert) error {
	if e.host == "" || e.from == "" || e.to == "" {
		return fmt.Errorf("email not configured")
	}

	subject := formatEmailSubject(alert)
	body := formatEmailBody(alert)

	msg := buildMIMEMessage(e.from, e.to, subject, body)

	addr := net.JoinHostPort(e.host, e.port)

	var auth smtp.Auth
	if e.user != "" && e.password != "" {
		auth = smtp.PlainAuth("", e.user, e.password, e.host)
	}

	return e.sendMail(addr, auth, e.from, []string{e.to}, msg)
}

// sendMail wraps smtp.SendMail for testability.
var sendMailFunc = smtp.SendMail

func (e *EmailSender) sendMail(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- sendMailFunc(addr, auth, from, to, msg)
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("email: send timeout after 10s")
	}
}

func formatEmailSubject(alert *database.Alert) string {
	severity := strings.ToUpper(alert.Severity)
	return fmt.Sprintf("[Ultron-AP] %s: %s", severity, alert.Source)
}

func formatEmailBody(alert *database.Alert) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Severity: %s\n", alert.Severity))
	b.WriteString(fmt.Sprintf("Message: %s\n", alert.Message))
	b.WriteString(fmt.Sprintf("Source: %s\n", alert.Source))
	if alert.Value != nil {
		b.WriteString(fmt.Sprintf("Value: %.1f\n", *alert.Value))
	}
	b.WriteString(fmt.Sprintf("Time: %s\n", alert.CreatedAt.Format("2006-01-02 15:04:05")))
	return b.String()
}

func buildMIMEMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s\r\n", from))
	b.WriteString(fmt.Sprintf("To: %s\r\n", to))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
