package notify

import (
	"net/smtp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

func TestFormatEmailSubject_Critical(t *testing.T) {
	alert := &database.Alert{Severity: "critical", Source: "cpu"}
	subject := formatEmailSubject(alert)
	assert.Equal(t, "[Ultron-AP] CRITICAL: cpu", subject)
}

func TestFormatEmailSubject_Warning(t *testing.T) {
	alert := &database.Alert{Severity: "warning", Source: "docker:nginx"}
	subject := formatEmailSubject(alert)
	assert.Equal(t, "[Ultron-AP] WARNING: docker:nginx", subject)
}

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
	assert.Contains(t, body, "Severity: critical")
	assert.Contains(t, body, "High CPU")
	assert.Contains(t, body, "Value: 95.0")
	assert.Contains(t, body, "2026-02-09 14:30:00")
}

func TestFormatEmailBody_WithoutValue(t *testing.T) {
	alert := &database.Alert{
		Severity:  "warning",
		Message:   "Container nginx stopped",
		Source:    "docker:nginx",
		CreatedAt: time.Now(),
	}

	body := formatEmailBody(alert)
	assert.Contains(t, body, "Severity: warning")
	assert.NotContains(t, body, "Value:")
}

func TestBuildMIMEMessage(t *testing.T) {
	msg := buildMIMEMessage("from@test.com", "to@test.com", "Test Subject", "Test body")
	s := string(msg)
	assert.Contains(t, s, "From: from@test.com")
	assert.Contains(t, s, "To: to@test.com")
	assert.Contains(t, s, "Subject: Test Subject")
	assert.Contains(t, s, "MIME-Version: 1.0")
	assert.Contains(t, s, "Content-Type: text/plain")
	assert.Contains(t, s, "Test body")
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
	sendMailFunc = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
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
	assert.Contains(t, string(capturedMsg), "[Ultron-AP] CRITICAL: cpu")
	assert.Contains(t, string(capturedMsg), "CPU high")
}

func TestEmailSender_Send_SMTPError(t *testing.T) {
	original := sendMailFunc
	sendMailFunc = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return assert.AnError
	}
	defer func() { sendMailFunc = original }()

	sender := NewEmailSender("smtp.test.com", "587", "user", "pass", "from@test.com", "to@test.com")
	alert := &database.Alert{Severity: "info", Message: "test", Source: "test", CreatedAt: time.Now()}

	err := sender.Send(alert)
	assert.Error(t, err)
}

func TestDispatcher_BuildNotifiers_EmailEnabled(t *testing.T) {
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
