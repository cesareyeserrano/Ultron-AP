package notify

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// makeUPSFireEvent builds a UPS fire the way cmd/ultron-ap wires it: no rule
// ConfigID (UPS alerts are not rule-driven), Source always "ups", and a
// per-kind DedupKey so the storm cache can tell the kinds apart.
func makeUPSFireEvent(kind, severity, message string) *Event {
	return &Event{
		Alert:     &database.Alert{Severity: severity, Source: "ups", Message: message},
		Kind:      EventFire,
		Surface:   SurfaceResource,
		DedupKey:  "ups:" + kind,
		Hostname:  "ultron",
		PublicURL: "https://example.com",
	}
}

// Regression: every UPS alert shares Source "ups", so keying the storm cache
// on the source alone made a low-battery CRITICAL collapse into an edit of the
// still-in-window mains-outage WARNING — the operator got no new notification
// for the one alert that matters most. Distinct kinds must each get their own
// chat row.
//
// @aitri-trace FR-021 FR-024
func TestUPS_DistinctKindsDoNotCollapse(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 9, 2, 17, 48, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(),
		makeUPSFireEvent("outage", "warning", "En batería — corte de red detectado")))
	// Well inside FireWindow: source-only keying would have edited here.
	clk.Advance(10 * time.Second)
	require.NoError(t, sender.Notify(context.Background(),
		makeUPSFireEvent("lowbatt", "critical", "Batería baja — UPS en batería crítica")))

	assert.Equal(t, 2, rec.countOf("sendMessage"),
		"a low-battery critical must arrive as its own message, not as an edit of the outage warning")
	assert.Equal(t, 0, rec.countOf("editMessageText"))
}

// The per-kind key must not disable storm protection: a genuine repeat of the
// SAME kind inside the window still collapses into an edit (FR-024).
//
// @aitri-trace FR-021 FR-024 AC-024-001
func TestUPS_SameKindStillCollapses(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 9, 2, 17, 48, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(),
		makeUPSFireEvent("inputvolt", "warning", "Voltaje de entrada fuera de rango: 0 V")))
	clk.Advance(10 * time.Second)
	require.NoError(t, sender.Notify(context.Background(),
		makeUPSFireEvent("inputvolt", "warning", "Voltaje de entrada fuera de rango: 4 V")))

	assert.Equal(t, 1, rec.countOf("sendMessage"), "repeat of the same kind must not re-send")
	assert.Equal(t, 1, rec.countOf("editMessageText"))
}

// A resolve must clear the storm entry of its OWN kind, so the next outage
// starts a fresh chat row rather than editing the previous outage message.
//
// @aitri-trace FR-021 FR-024
func TestUPS_ResolveClearsMatchingKind(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 9, 2, 17, 48, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(),
		makeUPSFireEvent("outage", "warning", "En batería — corte de red detectado")))

	resolve := makeUPSFireEvent("outage", "info", "Red eléctrica restablecida tras 1m10s")
	resolve.Kind = EventResolve
	resolve.ResolvedAt = clk.Now()
	clk.Advance(20 * time.Second)
	require.NoError(t, sender.Notify(context.Background(), resolve))

	// Second outage still inside the original 60s window, but the resolve
	// cleared the entry, so it must be a fresh send.
	clk.Advance(10 * time.Second)
	require.NoError(t, sender.Notify(context.Background(),
		makeUPSFireEvent("outage", "warning", "En batería — corte de red detectado")))

	assert.Equal(t, 3, rec.countOf("sendMessage"))
	assert.Equal(t, 0, rec.countOf("editMessageText"))
}

// ruleIDForEvent precedence: a real rule ConfigID wins; DedupKey is the
// fallback for rule-less sources; bare Source remains the last resort so
// docker/systemd keying is untouched.
//
// @aitri-trace FR-024
func TestRuleIDForEvent_Precedence(t *testing.T) {
	cfgID := int64(42)
	withRule := makeUPSFireEvent("outage", "warning", "x")
	withRule.Alert.ConfigID = &cfgID
	assert.Equal(t, int64(42), ruleIDForEvent(withRule), "a real rule ID wins over DedupKey")

	withKey := makeUPSFireEvent("outage", "warning", "x")
	assert.Equal(t, int64(hashSource("ups:outage")), ruleIDForEvent(withKey))

	bare := makeUPSFireEvent("outage", "warning", "x")
	bare.DedupKey = ""
	assert.Equal(t, int64(hashSource("ups")), ruleIDForEvent(bare),
		"sources without a DedupKey keep the legacy source-hash key")

	// The two UPS kinds must not hash to the same storm entry.
	other := makeUPSFireEvent("lowbatt", "critical", "x")
	assert.NotEqual(t, ruleIDForEvent(withKey), ruleIDForEvent(other))
}
