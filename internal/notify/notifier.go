package notify

import "github.com/cesareyeserrano/ultron-ap/internal/database"

// Notifier sends alert notifications through a specific channel.
type Notifier interface {
	Send(alert *database.Alert) error
	Name() string
}
