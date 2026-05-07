// Package notify is the alert-notification fan-out for Ultron-AP. The
// Notifier interface is implemented by per-channel senders (Telegram, Email)
// and is consumed by Dispatcher.
//
// Two methods exist: the legacy Send(*Alert) and the new Notify(ctx, *Event).
// The engine and the Test Telegram handler use Notify; ad-hoc callers (and
// tests written against the old API) keep working through Send, which
// internally synthesises a minimal Event so a single rendering path covers
// both call sites.
//
// @aitri-trace FR-005 FR-006 FR-024 ADR-05
package notify

import (
	"context"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// Notifier sends alert notifications through a specific channel.
type Notifier interface {
	// Name returns the channel name (used for logs and the dispatcher table).
	Name() string

	// Send is the legacy entry point. Synthesises a minimal Event from the
	// bare alert internally and delegates to the same render path Notify
	// uses, so output is consistent.
	Send(alert *database.Alert) error

	// Notify is the rich entry point used by the alert engine and the Test
	// Telegram button. The Event carries Rule, Kind, Surface, FirstFiredAt,
	// and the Hostname / PublicURL the renderer needs.
	Notify(ctx context.Context, evt *Event) error
}
