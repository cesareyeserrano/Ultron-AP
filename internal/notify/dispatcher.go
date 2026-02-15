package notify

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// Dispatcher manages notification channels and dispatches alerts asynchronously.
type Dispatcher struct {
	db     *database.DB
	queue  chan *database.Alert
	cancel chan struct{}
	wg     sync.WaitGroup
}

// NewDispatcher creates a notification dispatcher.
func NewDispatcher(db *database.DB) *Dispatcher {
	return &Dispatcher{
		db:    db,
		queue: make(chan *database.Alert, 100),
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

// Dispatch queues an alert for notification. Non-blocking; drops if queue full.
func (d *Dispatcher) Dispatch(alert *database.Alert) {
	select {
	case d.queue <- alert:
	default:
		log.Println("notify: queue full, dropping alert notification")
	}
}

func (d *Dispatcher) run() {
	for {
		select {
		case <-d.cancel:
			// Drain remaining
			for {
				select {
				case alert := <-d.queue:
					d.send(alert)
				default:
					return
				}
			}
		case alert := <-d.queue:
			d.send(alert)
		}
	}
}

func (d *Dispatcher) send(alert *database.Alert) {
	notifiers := d.buildNotifiers()
	for _, n := range notifiers {
		if err := n.Send(alert); err != nil {
			log.Printf("notify: %s send failed: %v", n.Name(), err)
		}
	}
}

func (d *Dispatcher) buildNotifiers() []Notifier {
	var notifiers []Notifier

	// Telegram
	if tg, err := d.db.GetNotificationConfig("telegram"); err == nil && tg != nil && tg.Enabled {
		var cfg map[string]string
		if err := json.Unmarshal([]byte(tg.Config), &cfg); err == nil {
			notifiers = append(notifiers, NewTelegramSender(cfg["bot_token"], cfg["chat_id"]))
		}
	}

	return notifiers
}
