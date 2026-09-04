package notify

import (
	"context"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// TestFormatEmailSubject_Critical asserts the post-refactor email subject
// equals the Telegram subject minus the severity emoji (FR-027 AC-027-001).
//
// @aitri-trace FR-017 FR-027 TC-TMU-Legacy-Email-Subject-Critical
func TestFormatEmailSubject_Critical(t *testing.T) {
	alert := &database.Alert{Severity: "critical", Source: "cpu"}
	subject := formatEmailSubject(alert)
	assert.Contains(t, subject, "CPU usage critical")
	assert.NotContains(t, subject, "🔴")
}

// @aitri-trace FR-017 FR-027 TC-TMU-Legacy-Email-Subject-Warning
func TestFormatEmailSubject_Warning(t *testing.T) {
	alert := &database.Alert{Severity: "warning", Source: "docker:nginx"}
	subject := formatEmailSubject(alert)
	assert.Contains(t, subject, "warning")
	assert.NotContains(t, subject, "🟡")
}

// TestFormatEmailBody_WithValue asserts the new email plain-text body
// contains the friendly metric block and the deep-link footer.
//
// @aitri-trace FR-016 FR-023 FR-027 TC-TMU-Legacy-Email-Body-Value
func TestFormatEmailBody_WithValue(t *testing.T) {
	value := 95.0
	alert := &database.Alert{
		Severity:  "critical",
		Message:   "High CPU: 95.0 > 90.0",
		Source:    "cpu",
		Value:     &value,
		CreatedAt: time.Date(2026, 2, 9, 14, 30, 0, 0, time.UTC),
	}

	body := formatEmailBody(alert)
	assert.Contains(t, body, "ALERT FIRED")
	assert.Contains(t, body, "CPU 95%")
	assert.Contains(t, body, "Open dashboard:")
}

// @aitri-trace FR-027 TC-TMU-Legacy-Email-Body-NoValue
func TestFormatEmailBody_WithoutValue(t *testing.T) {
	alert := &database.Alert{
		Severity:  "warning",
		Message:   "Container nginx stopped",
		Source:    "docker:nginx",
		CreatedAt: time.Now(),
	}

	body := formatEmailBody(alert)
	assert.Contains(t, body, "ALERT FIRED")
	assert.Contains(t, body, "Open dashboard:")
}

func TestBuildMIMEMessage(t *testing.T) {
	// Plain-only path — html arg empty preserves the legacy text/plain message.
	msg := buildMIMEMessage("from@test.com", "to@test.com", "Test Subject", "Test body", "")
	s := string(msg)
	assert.Contains(t, s, "From: from@test.com")
	assert.Contains(t, s, "To: to@test.com")
	assert.Contains(t, s, "Subject: Test Subject")
	assert.Contains(t, s, "MIME-Version: 1.0")
	assert.Contains(t, s, "Content-Type: text/plain")
	assert.Contains(t, s, "Test body")
}

// TestTC_TMU_027x covers FR-027 — buildMIMEMessage with both plain and html
// produces a multipart/alternative message containing both body parts.
//
// @aitri-trace FR-027 AC-027-002 TC-TMU-027x
func TestTC_TMU_027x_BuildMIMEMessage_Multipart(t *testing.T) {
	msg := buildMIMEMessage("a@b.com", "c@d.com", "Hi", "plain body", "<p>html body</p>")
	s := string(msg)
	assert.Contains(t, s, "Content-Type: multipart/alternative")
	assert.Contains(t, s, "Content-Type: text/plain")
	assert.Contains(t, s, "Content-Type: text/html")
	assert.Contains(t, s, "plain body")
	assert.Contains(t, s, "<p>html body</p>")
}

func TestEmailSender_Name(t *testing.T) {
	sender := NewEmailSender("", "", "", "", "", "")
	assert.Equal(t, "email", sender.Name())
}

func TestEmailSender_NotConfigured(t *testing.T) {
	sender := NewEmailSender("", "", "", "", "", "")
	alert := &database.Alert{Severity: "info", Message: "test", Source: "test"}
	err := sender.Send(alert)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestEmailSender_Send_MockSMTP(t *testing.T) {
	// Override sendMailFunc for testing
	var capturedFrom string
	var capturedTo []string
	var capturedMsg []byte

	original := sendMailFunc
	sendMailFunc = func(_ context.Context, addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		capturedFrom = from
		capturedTo = to
		capturedMsg = msg
		return nil
	}
	defer func() { sendMailFunc = original }()

	sender := NewEmailSender("smtp.test.com", "587", "user", "pass", "from@test.com", "to@test.com")
	value := 95.0
	alert := &database.Alert{
		Severity:  "critical",
		Message:   "CPU high",
		Source:    "cpu",
		Value:     &value,
		CreatedAt: time.Now(),
	}

	err := sender.Send(alert)
	require.NoError(t, err)
	assert.Equal(t, "from@test.com", capturedFrom)
	assert.Equal(t, []string{"to@test.com"}, capturedTo)
	// New format: friendly metric label in subject + ALERT FIRED + threshold clause.
	assert.Contains(t, string(capturedMsg), "CPU usage critical")
	assert.Contains(t, string(capturedMsg), "ALERT FIRED")
	assert.Contains(t, string(capturedMsg), "Content-Type: multipart/alternative")
}

func TestEmailSender_Send_SMTPError(t *testing.T) {
	original := sendMailFunc
	sendMailFunc = func(_ context.Context, addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return assert.AnError
	}
	defer func() { sendMailFunc = original }()

	sender := NewEmailSender("smtp.test.com", "587", "user", "pass", "from@test.com", "to@test.com")
	alert := &database.Alert{Severity: "info", Message: "test", Source: "test", CreatedAt: time.Now()}

	err := sender.Send(alert)
	assert.Error(t, err)
}

func TestDispatcher_BuildNotifiers_EmailEnabled(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "test-secret-key") // BG-044: secrets require a key
	db := setupTestDB(t)
	db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "email",
		Enabled: true,
		Config:  `{"smtp_host":"smtp.test.com","smtp_port":"587","smtp_user":"user","smtp_password":"pass","from":"a@b.com","to":"c@d.com"}`,
	})

	d := NewDispatcher(db)
	notifiers := d.buildNotifiers()
	assert.Len(t, notifiers, 1)
	assert.Equal(t, "email", notifiers[0].Name())
}

func TestDispatcher_BuildNotifiers_BothEnabled(t *testing.T) {
	t.Setenv("ULTRON_SECRET_KEY", "test-secret-key") // BG-044: secrets require a key
	db := setupTestDB(t)
	db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "telegram",
		Enabled: true,
		Config:  `{"bot_token":"123","chat_id":"456"}`,
	})
	db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "email",
		Enabled: true,
		Config:  `{"smtp_host":"smtp.test.com","smtp_port":"587","from":"a@b.com","to":"c@d.com"}`,
	})

	d := NewDispatcher(db)
	notifiers := d.buildNotifiers()
	assert.Len(t, notifiers, 2)
}

// B4 — header injection via CR/LF in From/To/Subject must be stripped so an
// attacker-controlled value cannot smuggle extra headers.
func TestBuildMIMEMessage_StripsHeaderInjection(t *testing.T) {
	msg := string(buildMIMEMessage(
		"alerts@ultron",
		"admin@host\r\nBcc: victim@evil.com",
		"Alert\r\nX-Injected: yes",
		"body",
		"",
	))
	// The dangerous property is a CR/LF that starts a NEW header line. The
	// injected text may survive inline in the field value (a malformed
	// recipient), but must never appear on its own header line.
	if strings.Contains(msg, "\nBcc:") || strings.Contains(msg, "\nX-Injected:") {
		t.Fatalf("header injection not stripped:\n%s", msg)
	}
	// And the header block itself must contain no bare LF beyond the CRLF pairs.
	headerBlock := msg
	if i := strings.Index(msg, "\r\n\r\n"); i >= 0 {
		headerBlock = msg[:i]
	}
	if strings.Contains(strings.ReplaceAll(headerBlock, "\r\n", ""), "\n") {
		t.Fatalf("stray LF left in header block:\n%q", headerBlock)
	}
}
