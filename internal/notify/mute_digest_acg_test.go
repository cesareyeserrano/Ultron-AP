// Feature ac-coverage-gaps — FR-079 (Telegram mute window) and FR-080 (daily
// email digest). Test ids match spec/03_TEST_CASES.json; the runner links them
// by test name (TestTC_ACG_079Ah ↔ TC-ACG-079Ah).
package notify

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// --- fakes -------------------------------------------------------------

// countingNotifier stands in for a real channel so a test can assert exactly
// how many sends happened, with no network.
type countingNotifier struct {
	name  string
	sends int
	err   error
}

func (c *countingNotifier) Name() string { return c.name }
func (c *countingNotifier) Send(alert *database.Alert) error {
	c.sends++
	return c.err
}
func (c *countingNotifier) Notify(ctx context.Context, evt *Event) error {
	c.sends++
	return c.err
}

type fakeDigestSender struct {
	sends    int
	subject  string
	plain    string
	htmlBody string
	err      error
}

func (f *fakeDigestSender) SendDigest(_ context.Context, subject, plain, htmlBody string) error {
	f.sends++
	f.subject, f.plain, f.htmlBody = subject, plain, htmlBody
	return f.err
}

// --- helpers -----------------------------------------------------------

func configureChannels(t *testing.T, db *database.DB) {
	t.Helper()
	t.Setenv("ULTRON_SECRET_KEY", "test-secret-key") // BG-044: notif configs are encrypted at rest
	tg, _ := json.Marshal(map[string]string{"bot_token": "123:ABC", "chat_id": "789"})
	require.NoError(t, db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "telegram", Enabled: true, Config: string(tg),
	}))
	em, _ := json.Marshal(map[string]string{
		"smtp_host": "mail.lan", "smtp_port": "587", "from": "u@lan", "to": "a@lan",
	})
	require.NoError(t, db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "email", Enabled: true, Config: string(em),
	}))
}

// injectNotifiers swaps the dispatcher's cached notifiers for fakes. The cache
// key is computed from the same DB rows getNotifiers() reads, so the cache hits
// and the real (network-touching) senders are never constructed.
func injectNotifiers(t *testing.T, d *Dispatcher, notifiers ...Notifier) {
	t.Helper()
	tgRow, err := d.db.GetNotificationConfig("telegram")
	require.NoError(t, err)
	emRow, err := d.db.GetNotificationConfig("email")
	require.NoError(t, err)

	d.notifierMu.Lock()
	defer d.notifierMu.Unlock()
	d.cachedNotifiers = notifiers
	d.notifierKey = notifierFingerprint(tgRow, emRow)
}

func fireAlert(t *testing.T, db *database.DB, d *Dispatcher, severity, source string) {
	t.Helper()
	alert := &database.Alert{Severity: severity, Message: "test fire", Source: source}
	require.NoError(t, db.CreateAlert(alert))
	d.send(d.buildEventFromAlert(alert, EventFire)) // synchronous — no queue race
}

func setDigestConfig(t *testing.T, db *database.DB, enabled bool, hour int) {
	t.Helper()
	t.Setenv("ULTRON_SECRET_KEY", "test-secret-key")
	cfg := map[string]string{
		"smtp_host": "mail.lan", "smtp_port": "587", "from": "u@lan", "to": "a@lan",
		"digest_enabled": "false", "digest_hour": "8",
	}
	if enabled {
		cfg["digest_enabled"] = "true"
	}
	cfg["digest_hour"] = strconv.Itoa(hour)
	raw, _ := json.Marshal(cfg)
	require.NoError(t, db.UpsertNotificationConfig(&database.NotificationConfig{
		Channel: "email", Enabled: true, Config: string(raw),
	}))
}

func newDigest(t *testing.T, db *database.DB, sender *fakeDigestSender) *DigestScheduler {
	t.Helper()
	s := NewDigestScheduler(db)
	s.senderFor = func(map[string]string) digestSender { return sender }
	return s
}

// --- FR-079 — Telegram mute window -------------------------------------

// TC-ACG-079Ah / AC-079-001 (also NFR-085, NFR-090)
func TestTC_ACG_079Ah_NoMuteDeliversToTelegram(t *testing.T) {
	db := setupTestDB(t)
	configureChannels(t, db)
	d := NewDispatcher(db)

	tg := &countingNotifier{name: "telegram"}
	em := &countingNotifier{name: "email"}
	injectNotifiers(t, d, tg, em)

	fireAlert(t, db, d, "critical", "cpu")

	assert.Equal(t, 1, tg.sends, "with no mute window, telegram must receive the fire")
	assert.Equal(t, 1, em.sends, "email is never affected by the telegram mute")
}

// TC-ACG-079Bh / AC-079-002
func TestTC_ACG_079Bh_OpenMuteSkipsTelegramButPersistsAlert(t *testing.T) {
	db := setupTestDB(t)
	configureChannels(t, db)
	d := NewDispatcher(db)

	// Opened 10 minutes ago for 1h → still open.
	_, err := db.SetNotificationMute(1, time.Now().Add(-10*time.Minute))
	require.NoError(t, err)

	tg := &countingNotifier{name: "telegram"}
	em := &countingNotifier{name: "email"}
	injectNotifiers(t, d, tg, em)

	fireAlert(t, db, d, "critical", "cpu")

	assert.Equal(t, 0, tg.sends, "an open mute window must suppress telegram delivery")
	assert.Equal(t, 1, em.sends, "mute is telegram-only — email still delivers")

	alerts, err := db.ListAlerts(10)
	require.NoError(t, err)
	require.Len(t, alerts, 1, "mute suppresses DELIVERY, never persistence")
	assert.Equal(t, "cpu", alerts[0].Source)
}

// TC-ACG-079Ce / AC-079-003 (also NFR-085)
func TestTC_ACG_079Ce_ExpiredMuteResumesDelivery(t *testing.T) {
	db := setupTestDB(t)
	configureChannels(t, db)
	d := NewDispatcher(db)

	// A 1h window opened 61 minutes ago → expired 1 minute ago.
	_, err := db.SetNotificationMute(1, time.Now().Add(-61*time.Minute))
	require.NoError(t, err)

	tg := &countingNotifier{name: "telegram"}
	injectNotifiers(t, d, tg)

	fireAlert(t, db, d, "warning", "ram")

	assert.Equal(t, 1, tg.sends, "the window expires on its own — no admin action needed")
}

// TC-ACG-079Gf / AC-079-002 (also NFR-085, NFR-090) — fail-open.
func TestTC_ACG_079Gf_CorruptMuteRowStillDelivers(t *testing.T) {
	db := setupTestDB(t)
	configureChannels(t, db)
	d := NewDispatcher(db)

	// Write a mute row whose expires_at cannot be read as a time.
	_, err := db.Exec(`INSERT INTO NotificationMute (id, expires_at, hours) VALUES (1, 'not-a-timestamp', 4)`)
	require.NoError(t, err)

	_, muted, muteErr := db.NotificationMuteUntil(time.Now())
	assert.False(t, muted, "an unreadable mute row must NOT be treated as muted")
	assert.Error(t, muteErr, "the failure is surfaced to the caller, not swallowed")

	tg := &countingNotifier{name: "telegram"}
	injectNotifiers(t, d, tg)

	fireAlert(t, db, d, "critical", "disk")

	assert.Equal(t, 1, tg.sends,
		"failing OPEN: a mute we cannot prove must never silently swallow a critical alert")
}

// --- FR-080 — daily email digest ---------------------------------------

// TC-ACG-080Ah / AC-080-001 (also NFR-091)
func TestTC_ACG_080Ah_DigestSendsSummaryOfLast24h(t *testing.T) {
	db := setupTestDB(t)
	setDigestConfig(t, db, true, 8)

	for _, a := range []*database.Alert{
		{Severity: "critical", Message: "CPU high", Source: "cpu"},
		{Severity: "warning", Message: "RAM high", Source: "ram"},
		{Severity: "info", Message: "IP changed", Source: "network"},
	} {
		require.NoError(t, db.CreateAlert(a))
	}

	sender := &fakeDigestSender{}
	sched := newDigest(t, db, sender)

	require.NoError(t, sched.Tick(time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)))

	require.Equal(t, 1, sender.sends, "the digest hour must send exactly one email")
	for _, want := range []string{"critical", "cpu", "warning", "ram", "info", "network"} {
		assert.Contains(t, sender.plain, want, "digest body must list %q", want)
	}
	assert.Contains(t, sender.subject, "3 alert(s)")

	// NFR-091 — observable without reading the journal.
	actions, err := db.ListActionLogs(10)
	require.NoError(t, err)
	require.NotEmpty(t, actions)
	assert.Equal(t, "digest", actions[0].Action)
	assert.Equal(t, "success", actions[0].Result)
}

// TC-ACG-080Be / AC-080-002 (also NFR-086)
func TestTC_ACG_080Be_AtMostOneDigestPerCalendarDay(t *testing.T) {
	db := setupTestDB(t)
	setDigestConfig(t, db, true, 8)
	require.NoError(t, db.MarkDigestSent("2026-07-13"))

	sender := &fakeDigestSender{}
	sched := newDigest(t, db, sender)

	require.NoError(t, sched.Tick(time.Date(2026, 7, 13, 8, 30, 0, 0, time.UTC)))

	assert.Equal(t, 0, sender.sends, "a second tick the same day must send nothing")
}

// TC-ACG-080Ce / AC-080-003
func TestTC_ACG_080Ce_EmptyWindowStillSendsOneDigest(t *testing.T) {
	db := setupTestDB(t)
	setDigestConfig(t, db, true, 8)

	sender := &fakeDigestSender{}
	sched := newDigest(t, db, sender)

	require.NoError(t, sched.Tick(time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)))

	require.Equal(t, 1, sender.sends,
		"silence is ambiguous — the admin must be able to tell 'no alerts' from 'digest broken'")
	assert.Contains(t, sender.plain, "No alerts")
	assert.Contains(t, sender.subject, "no alerts")
}

// TC-ACG-080Df / AC-080-004 (also NFR-086)
func TestTC_ACG_080Df_DisabledDigestSendsNothing(t *testing.T) {
	db := setupTestDB(t)
	setDigestConfig(t, db, false, 8)
	require.NoError(t, db.CreateAlert(&database.Alert{Severity: "critical", Message: "x", Source: "cpu"}))

	sender := &fakeDigestSender{}
	sched := newDigest(t, db, sender)

	require.NoError(t, sched.Tick(time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)))

	assert.Equal(t, 0, sender.sends, "the digest hour passing must send nothing when the digest is off")
}

// TC-ACG-080Eh / AC-080-005 (also NFR-086) — the digest never replaces per-event mail.
func TestTC_ACG_080Eh_PerEventEmailsSurviveTheDigest(t *testing.T) {
	db := setupTestDB(t)
	setDigestConfig(t, db, true, 8)
	configureChannels(t, db)
	setDigestConfig(t, db, true, 8) // re-apply: configureChannels rewrote the email row

	d := NewDispatcher(db)
	em := &countingNotifier{name: "email"}
	injectNotifiers(t, d, em)

	// A fire at 10:00 — not the digest hour.
	fireAlert(t, db, d, "critical", "cpu")

	assert.Equal(t, 1, em.sends,
		"per-event email delivery is independent of digest configuration")
}

// TC-ACG-080Ff / AC-080-006 (also NFR-090, NFR-091)
func TestTC_ACG_080Ff_FailedDigestIsRecordedAndNeverRetryStorms(t *testing.T) {
	db := setupTestDB(t)
	setDigestConfig(t, db, true, 8)

	sender := &fakeDigestSender{err: assert.AnError}
	sched := newDigest(t, db, sender)

	// First tick fails to send…
	err := sched.Tick(time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC))
	assert.Error(t, err, "the failure is surfaced to the caller")

	// …and the next tick that same hour must NOT retry (a dead relay would
	// otherwise be hit ~60 times before the hour rolls over).
	require.NoError(t, sched.Tick(time.Date(2026, 7, 13, 8, 1, 0, 0, time.UTC)))

	assert.Equal(t, 1, sender.sends, "exactly one attempt per calendar day, success or failure")

	actions, err := db.ListActionLogs(10)
	require.NoError(t, err)
	require.NotEmpty(t, actions)
	assert.Equal(t, "digest", actions[0].Action)
	assert.Equal(t, "failed", actions[0].Result, "a failed digest must be visible in action history")
}

// TC-ACG-090e / AC-080-002 (NFR-090) — a corrupt marker never suppresses forever.
func TestTC_ACG_090e_CorruptLastSentDateDoesNotSuppressDigest(t *testing.T) {
	db := setupTestDB(t)
	setDigestConfig(t, db, true, 8)
	require.NoError(t, db.MarkDigestSent("not-a-date"))

	sender := &fakeDigestSender{}
	sched := newDigest(t, db, sender)

	require.NoError(t, sched.Tick(time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)))

	assert.Equal(t, 1, sender.sends, "a corrupt marker must be treated as never-sent, not as sent-today")

	last, err := db.DigestLastSentDate()
	require.NoError(t, err)
	assert.Equal(t, "2026-07-13", last, "the marker is rewritten with a valid date")
}
