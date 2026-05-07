package notify

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// Dispatcher manages notification channels and dispatches alerts to each
// configured notifier. It exposes two enqueue paths:
//
//   - Dispatch(*Alert) — legacy path for callers that only have a database
//     alert row. Synthesises a minimal Event internally.
//   - DispatchEvent(*Event) — rich path used by the alert engine and the
//     Test Telegram handler.
//
// The dispatcher resolves Event.Rule via db.GetAlertConfig once per send
// (alert dispatch is rare; ~1/min worst case) and caches Hostname /
// PublicURL at construction.
type Dispatcher struct {
	db        *database.DB
	queue     chan *Event
	cancel    chan struct{}
	wg        sync.WaitGroup
	hostname  string
	publicURL string
}

// NewDispatcher creates a notification dispatcher.
func NewDispatcher(db *database.DB) *Dispatcher {
	host, _ := os.Hostname()
	if host == "" {
		host = "host"
	}
	publicURL := strings.TrimRight(os.Getenv("ULTRON_PUBLIC_URL"), "/")
	if publicURL == "" {
		publicURL = "http://localhost"
	}
	return &Dispatcher{
		db:        db,
		queue:     make(chan *Event, 100),
		hostname:  host,
		publicURL: publicURL,
	}
}

// Start begins processing the notification queue.
func (d *Dispatcher) Start() {
	d.cancel = make(chan struct{})
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.run()
	}()
	log.Println("Notification dispatcher started")
}

// Stop drains the queue and stops processing.
func (d *Dispatcher) Stop() {
	close(d.cancel)
	d.wg.Wait()
	log.Println("Notification dispatcher stopped")
}

// Dispatch is the legacy enqueue path. Builds a minimal Event from the bare
// alert and delegates to DispatchEvent.
func (d *Dispatcher) Dispatch(alert *database.Alert) {
	if alert == nil {
		return
	}
	d.DispatchEvent(d.buildEventFromAlert(alert, EventFire))
}

// DispatchEvent enqueues a rich Event for fan-out. Non-blocking; drops if
// the queue is full so a notification storm cannot back up the engine.
func (d *Dispatcher) DispatchEvent(evt *Event) {
	if evt == nil {
		return
	}
	select {
	case d.queue <- evt:
	default:
		log.Println("notify: queue full, dropping notification")
	}
}

// NotifyTest synchronously dispatches a synthetic CPU-fire event through the
// configured Telegram notifier. Used by the Test Telegram button (FR-026).
//
// @aitri-trace FR-026
func (d *Dispatcher) NotifyTest(ctx context.Context) error {
	notifiers := d.buildNotifiers()
	var tg Notifier
	for _, n := range notifiers {
		if n.Name() == "telegram" {
			tg = n
			break
		}
	}
	if tg == nil {
		return errNoTelegramConfigured
	}
	evt := d.syntheticTestEvent()
	return tg.Notify(ctx, evt)
}

func (d *Dispatcher) run() {
	for {
		select {
		case <-d.cancel:
			// Drain remaining events.
			for {
				select {
				case evt := <-d.queue:
					d.send(evt)
				default:
					return
				}
			}
		case evt := <-d.queue:
			d.send(evt)
		}
	}
}

// send fans out an Event to all configured notifiers via Notify, with a
// 5-second per-notifier timeout to keep one slow channel from blocking the
// queue.
func (d *Dispatcher) send(evt *Event) {
	if evt == nil {
		return
	}
	d.populateEventDefaults(evt)
	notifiers := d.buildNotifiers()
	for _, n := range notifiers {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := n.Notify(ctx, evt); err != nil {
			log.Printf("notify: %s notify failed: %v", n.Name(), err)
		}
		cancel()
	}
}

// populateEventDefaults fills in fields that the engine may have left empty
// (Hostname, PublicURL, Rule). The engine emits the Event with surface +
// kind already set; the dispatcher resolves rule lookup centrally so each
// notifier sees a fully-populated Event.
func (d *Dispatcher) populateEventDefaults(evt *Event) {
	if evt.Hostname == "" {
		evt.Hostname = d.hostname
	}
	if evt.PublicURL == "" {
		evt.PublicURL = d.publicURL
	}
	if evt.Surface == "" && evt.Alert != nil {
		evt.Surface = SurfaceFromSource(evt.Alert.Source)
	}
	if evt.Rule == nil && evt.Alert != nil && evt.Alert.ConfigID != nil && *evt.Alert.ConfigID > 0 {
		if rule, err := d.db.GetAlertConfig(*evt.Alert.ConfigID); err == nil && rule != nil {
			evt.Rule = rule
		}
	}
}

// buildEventFromAlert builds a minimal Event from an Alert + EventKind.
// Hostname / PublicURL come from the dispatcher (cached at construction).
func (d *Dispatcher) buildEventFromAlert(alert *database.Alert, kind EventKind) *Event {
	return &Event{
		Alert:     alert,
		Kind:      kind,
		Surface:   SurfaceFromSource(alert.Source),
		Hostname:  d.hostname,
		PublicURL: d.publicURL,
	}
}

// syntheticTestEvent returns a CPU-fire Event for the Test Telegram button
// (FR-026). Marked Test=true would propagate to the renderer if the Test
// flag was on Event; we keep the Test flag at the render layer to avoid
// leaking UX state to the notification taxonomy. Instead, we mark the
// alert message with a "TEST — " prefix the renderer leaves alone.
func (d *Dispatcher) syntheticTestEvent() *Event {
	val := 92.4
	cfgID := int64(0)
	now := time.Now().UTC()
	alert := &database.Alert{
		Severity:  "critical",
		Message:   "TEST — synthetic CPU fire from settings page",
		Source:    "cpu",
		Value:     &val,
		ConfigID:  &cfgID,
		CreatedAt: now,
	}
	rule := &database.AlertConfig{
		ID:        0,
		Name:      "TEST",
		Metric:    "cpu",
		Operator:  ">",
		Threshold: 80.0,
		Severity:  "critical",
	}
	return &Event{
		Alert:        alert,
		Rule:         rule,
		Kind:         EventFire,
		Surface:      SurfaceResource,
		FirstFiredAt: now.Add(-90 * time.Second),
		Hostname:     "TEST — " + d.hostname,
		PublicURL:    d.publicURL,
	}
}

// buildNotifiers reads the configured notification channels from SQLite and
// returns the Notifier list for the current send.
func (d *Dispatcher) buildNotifiers() []Notifier {
	var notifiers []Notifier

	if tg, err := d.db.GetNotificationConfig("telegram"); err == nil && tg != nil && tg.Enabled {
		var cfg map[string]string
		if err := json.Unmarshal([]byte(tg.Config), &cfg); err == nil {
			notifiers = append(notifiers, NewTelegramSender(cfg["bot_token"], cfg["chat_id"]))
		}
	}
	if em, err := d.db.GetNotificationConfig("email"); err == nil && em != nil && em.Enabled {
		var cfg map[string]string
		if err := json.Unmarshal([]byte(em.Config), &cfg); err == nil {
			notifiers = append(notifiers, NewEmailSender(
				cfg["smtp_host"], cfg["smtp_port"],
				cfg["smtp_user"], cfg["smtp_password"],
				cfg["from"], cfg["to"],
			))
		}
	}
	return notifiers
}

// errNoTelegramConfigured is returned by NotifyTest when no enabled telegram
// channel is found.
var errNoTelegramConfigured = simpleErr("telegram channel not configured or disabled")

type simpleErr string

func (s simpleErr) Error() string { return string(s) }
