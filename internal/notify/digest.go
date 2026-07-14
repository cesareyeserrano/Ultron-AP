package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// digestTickInterval is how often the scheduler wakes. One minute bounds the
// send to the first minute of the digest hour while costing the Pi a config
// read and an integer compare per tick — the 24h alert query runs once a day.
const digestTickInterval = time.Minute

// digestSender is the slice of EmailSender the digest needs. It exists so a
// test can drive Tick() with a fake transport and assert exactly how many sends
// happened, with no SMTP and no wall clock.
type digestSender interface {
	SendDigest(ctx context.Context, subject, plain, htmlBody string) error
}

// DigestScheduler sends one email per calendar day summarising the previous 24
// hours of alerts (FR-080).
//
// It is additive: per-event alert emails keep going out on every fire through
// the dispatcher. This is a second, independent caller of the same SMTP
// channel (NFR-086).
type DigestScheduler struct {
	db     *database.DB
	cancel context.CancelFunc

	// senderFor builds the transport from the stored email config. Overridden
	// in tests; nil means "use the real EmailSender".
	senderFor func(cfg map[string]string) digestSender
}

func NewDigestScheduler(db *database.DB) *DigestScheduler {
	return &DigestScheduler{db: db}
}

// Start runs the scheduler until Stop or ctx cancellation.
func (s *DigestScheduler) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	go func() {
		ticker := time.NewTicker(digestTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := s.Tick(now); err != nil {
					// Never fatal: a digest failure must not take down the
					// panel (NFR-090). It is already recorded in ActionLog.
					log.Printf("digest: tick failed: %v", err)
				}
			}
		}
	}()
	log.Println("Digest scheduler started")
}

func (s *DigestScheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Tick performs one scheduler evaluation at the given instant.
//
// The clock is a parameter, not time.Now(), because every acceptance criterion
// of FR-080 is of the form "given this stored state at this instant, does
// exactly one email go out?" — passing the instant in makes those assertions
// exact and keeps the tests free of sleeps and wall-clock races.
func (s *DigestScheduler) Tick(now time.Time) error {
	cfg, enabled, hour, err := s.digestConfig()
	if err != nil {
		return err
	}
	if !enabled {
		return nil // AC-080-004
	}
	if now.Hour() != hour {
		return nil
	}

	today := now.Format(database.DigestDateLayout)
	last, err := s.db.DigestLastSentDate()
	if err != nil {
		return fmt.Errorf("digest: read last-sent date: %w", err)
	}
	if last == today {
		return nil // AC-080-002 — at most one per calendar day
	}

	alerts, err := s.db.AlertsSince(now.Add(-24 * time.Hour))
	if err != nil {
		return fmt.Errorf("digest: read alerts: %w", err)
	}

	subject, plain, htmlBody := renderDigest(alerts, now)

	sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sendErr := s.sender(cfg).SendDigest(sendCtx, subject, plain, htmlBody)

	// Mark on COMPLETION, not on success. A relay that is down would otherwise
	// be retried on every tick for the rest of the hour (~60 sends); the
	// failure is surfaced in ActionLog and the journal instead (ADR-3).
	if markErr := s.db.MarkDigestSent(today); markErr != nil {
		log.Printf("digest: could not mark %s as sent: %v", today, markErr)
	}

	result, details := "success", fmt.Sprintf("%d alert(s) in the last 24h", len(alerts))
	if sendErr != nil {
		result = "failed"
		details = sendErr.Error()
	}
	if logErr := s.db.LogAction(nil, "digest", "digest", "email", result, details); logErr != nil {
		log.Printf("digest: could not record action: %v", logErr)
	}
	log.Printf("digest: sent=%t alerts=%d date=%s", sendErr == nil, len(alerts), today)

	if sendErr != nil {
		return fmt.Errorf("digest: send failed: %w", sendErr)
	}
	return nil
}

// digestConfig reads the email channel's stored configuration and the two
// digest keys inside it. An email channel that is disabled entirely disables
// the digest with it — a digest is an email, so it cannot outlive its channel.
func (s *DigestScheduler) digestConfig() (cfg map[string]string, enabled bool, hour int, err error) {
	row, err := s.db.GetNotificationConfig("email")
	if err != nil {
		return nil, false, 0, fmt.Errorf("digest: read email config: %w", err)
	}
	if row == nil || !row.Enabled {
		return nil, false, 0, nil
	}

	cfg = map[string]string{}
	if err := json.Unmarshal([]byte(row.Config), &cfg); err != nil {
		return nil, false, 0, fmt.Errorf("digest: parse email config: %w", err)
	}

	if !strings.EqualFold(cfg["digest_enabled"], "true") {
		return cfg, false, 0, nil
	}

	hour, convErr := strconv.Atoi(cfg["digest_hour"])
	if convErr != nil || hour < 0 || hour > 23 {
		// A config written before this feature has no digest_hour. Treat an
		// absent/unparseable hour as 8am rather than disabling silently — the
		// admin explicitly turned the digest on.
		hour = 8
	}
	return cfg, true, hour, nil
}

func (s *DigestScheduler) sender(cfg map[string]string) digestSender {
	if s.senderFor != nil {
		return s.senderFor(cfg)
	}
	return NewEmailSender(
		cfg["smtp_host"], cfg["smtp_port"],
		cfg["smtp_user"], cfg["smtp_password"],
		cfg["from"], cfg["to"],
	)
}

// renderDigest builds the summary body. An empty window still produces an
// email that says so (AC-080-003): silence is ambiguous — the admin cannot
// tell "no alerts" from "the digest is broken" — so the digest states it.
func renderDigest(alerts []database.Alert, now time.Time) (subject, plain, htmlBody string) {
	date := now.Format("Mon 2 Jan 2006")

	if len(alerts) == 0 {
		subject = fmt.Sprintf("[Ultron-AP] Daily digest — no alerts (%s)", date)
		plain = fmt.Sprintf("Daily digest for %s\n\nNo alerts fired in the last 24 hours.\n", date)
		htmlBody = fmt.Sprintf(
			"<h2>Daily digest — %s</h2><p>No alerts fired in the last 24 hours.</p>",
			html.EscapeString(date))
		return subject, plain, htmlBody
	}

	counts := map[string]int{}
	for _, a := range alerts {
		counts[a.Severity]++
	}
	summary := fmt.Sprintf("%d critical · %d warning · %d info",
		counts["critical"], counts["warning"], counts["info"])

	subject = fmt.Sprintf("[Ultron-AP] Daily digest — %d alert(s) (%s)", len(alerts), date)

	var p, h strings.Builder
	fmt.Fprintf(&p, "Daily digest for %s\n\n%d alert(s) in the last 24 hours: %s\n\n", date, len(alerts), summary)
	fmt.Fprintf(&h, "<h2>Daily digest — %s</h2><p>%d alert(s) in the last 24 hours: %s</p><ul>",
		html.EscapeString(date), len(alerts), html.EscapeString(summary))

	for _, a := range alerts {
		ts := a.CreatedAt.Format("15:04:05")
		fmt.Fprintf(&p, "  [%s] %-8s %-20s %s\n", ts, a.Severity, a.Source, a.Message)
		// Alert message/source are attacker-influenceable (a container name, a
		// unit name), so the HTML body escapes them.
		fmt.Fprintf(&h, "<li><code>%s</code> <strong>%s</strong> %s — %s</li>",
			html.EscapeString(ts), html.EscapeString(a.Severity),
			html.EscapeString(a.Source), html.EscapeString(a.Message))
	}
	h.WriteString("</ul>")

	return subject, p.String(), h.String()
}
