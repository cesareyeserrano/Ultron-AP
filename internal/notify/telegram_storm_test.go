package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/storm"
)

// telegramAPIRecorder is a roundTripFunc that records every call to the
// stubbed Telegram API and returns canned responses keyed by endpoint
// suffix (e.g. "sendMessage", "editMessageText").
type telegramAPIRecorder struct {
	mu       sync.Mutex
	calls    []recordedCall
	response map[string]canned // suffix → response
}

type recordedCall struct {
	endpoint string // last URL path component, e.g. "sendMessage"
	body     map[string]any
}

type canned struct {
	status int
	body   string
}

func newRecorder() *telegramAPIRecorder {
	return &telegramAPIRecorder{
		response: map[string]canned{
			"sendMessage":     {200, `{"ok":true,"result":{"message_id":555}}`},
			"editMessageText": {200, `{"ok":true,"result":{"message_id":555}}`},
		},
	}
}

func (r *telegramAPIRecorder) roundTrip(req *http.Request) (*http.Response, error) {
	parts := strings.Split(req.URL.Path, "/")
	suffix := parts[len(parts)-1]

	var payload map[string]any
	if req.Body != nil {
		_ = json.NewDecoder(req.Body).Decode(&payload)
	}
	r.mu.Lock()
	r.calls = append(r.calls, recordedCall{endpoint: suffix, body: payload})
	resp := r.response[suffix]
	r.mu.Unlock()
	if resp.status == 0 {
		resp = canned{status: 200, body: `{"ok":true}`}
	}
	return &http.Response{
		StatusCode: resp.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(resp.body)),
	}, nil
}

func (r *telegramAPIRecorder) snapshot() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedCall, len(r.calls))
	copy(out, r.calls)
	return out
}

func (r *telegramAPIRecorder) countOf(suffix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := 0
	for _, call := range r.calls {
		if call.endpoint == suffix {
			c++
		}
	}
	return c
}

// newStormTelegramSender builds a TelegramSender wired against the
// recorder + a controllable clock so the storm cache TTL can be advanced
// deterministically.
func newStormTelegramSender(t *testing.T, rec *telegramAPIRecorder, clk *fakeClock) *TelegramSender {
	t.Helper()
	return &TelegramSender{
		botToken: "test-token",
		chatID:   "12345",
		client:   &http.Client{Transport: roundTripFunc(rec.roundTrip)},
		storm:    storm.New(clk.Now),
		now:      clk.Now,
	}
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(initial time.Time) *fakeClock { return &fakeClock{t: initial} }
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func makeFireEvent(ruleID int64, value float64) *Event {
	cfgID := ruleID
	return &Event{
		Alert: &database.Alert{
			ConfigID: &cfgID,
			Severity: "critical",
			Source:   "cpu",
			Value:    &value,
		},
		Rule:         &database.AlertConfig{ID: ruleID, Metric: "cpu", Operator: ">", Threshold: 80, Severity: "critical"},
		Kind:         EventFire,
		Surface:      SurfaceResource,
		FirstFiredAt: time.Now().Add(-30 * time.Second),
		Hostname:     "ultron",
		PublicURL:    "https://example.com",
	}
}

// TestTC_TMU_024hi covers FR-024 / AC-024-001 at the integration layer:
// fire #1 produces exactly one sendMessage call against the stubbed
// Telegram API; storm.Cache has the message_id stamped.
//
// @aitri-trace FR-024 AC-024-001 BL-024 TC-TMU-024hi
func TestTC_TMU_024hi_FirstFireSendsAndCaches(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 5, 7, 16, 0, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 92.4)))

	assert.Equal(t, 1, rec.countOf("sendMessage"))
	assert.Equal(t, 0, rec.countOf("editMessageText"))
	mid, fc, ok := sender.storm.Snapshot(7)
	require.True(t, ok)
	assert.Equal(t, int64(555), mid)
	assert.Equal(t, 1, fc)
}

// TestTC_TMU_024ei covers FR-024 / AC-024-001 at the integration layer:
// a second fire 45s after the first issues editMessageText (NOT a fresh
// send) with "(2 fires)" injected into the rendered body's subject line.
//
// @aitri-trace FR-024 AC-024-001 BL-024 TC-TMU-024ei
func TestTC_TMU_024ei_SecondFireWithinWindowEdits(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 5, 7, 16, 0, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 92.4)))
	clk.Advance(45 * time.Second)
	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 93.0)))

	assert.Equal(t, 1, rec.countOf("sendMessage"), "second fire must not re-send")
	assert.Equal(t, 1, rec.countOf("editMessageText"))

	editCall := rec.snapshot()[1]
	assert.Equal(t, "editMessageText", editCall.endpoint)
	assert.Equal(t, float64(555), editCall.body["message_id"])
	text, _ := editCall.body["text"].(string)
	// The storm counter is injected as MarkdownV2-escaped "\(2 fires\)";
	// Telegram renders the backslashes invisible so the user sees
	// "(2 fires)" verbatim.
	assert.Contains(t, text, `\(2 fires\)`, "edited body must show storm counter")
}

// TestTC_TMU_024wi covers FR-024 / AC-024-002: a fire arriving after
// the 60s window produces a fresh sendMessage and replaces the cache.
//
// @aitri-trace FR-024 AC-024-002 BL-024 TC-TMU-024wi
func TestTC_TMU_024wi_FireAfterWindowSendsFresh(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 5, 7, 16, 0, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 92.4)))
	clk.Advance(70 * time.Second) // beyond storm.FireWindow
	rec.mu.Lock()
	rec.response["sendMessage"] = canned{200, `{"ok":true,"result":{"message_id":777}}`}
	rec.mu.Unlock()
	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 91.0)))

	assert.Equal(t, 2, rec.countOf("sendMessage"))
	assert.Equal(t, 0, rec.countOf("editMessageText"))
	mid, fc, ok := sender.storm.Snapshot(7)
	require.True(t, ok)
	assert.Equal(t, int64(777), mid, "cache replaced with new message_id")
	assert.Equal(t, 1, fc, "counter reset for the fresh chat row")
}

// TestTC_TMU_024ri covers FR-024 / AC-024-003: a resolve event clears
// the cache so a subsequent fire begins a fresh chat row.
//
// @aitri-trace FR-024 AC-024-003 BL-024 TC-TMU-024ri
func TestTC_TMU_024ri_ResolveClearsCacheAndNextFireIsFreshSend(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 5, 7, 16, 0, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 92.4)))
	require.Equal(t, 1, rec.countOf("sendMessage"))

	// Resolve event for the same rule.
	resolve := makeFireEvent(7, 50.0)
	resolve.Kind = EventResolve
	resolve.ResolvedAt = clk.Now()
	require.NoError(t, sender.Notify(context.Background(), resolve))
	if _, _, ok := sender.storm.Snapshot(7); ok {
		t.Fatalf("storm cache still has rule 7 after resolve")
	}

	// Re-fire after resolve.
	clk.Advance(time.Second)
	rec.mu.Lock()
	rec.response["sendMessage"] = canned{200, `{"ok":true,"result":{"message_id":888}}`}
	rec.mu.Unlock()
	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 95.0)))

	assert.Equal(t, 3, rec.countOf("sendMessage"), "fire + resolve + fire = 3 sendMessage calls")
	assert.Equal(t, 0, rec.countOf("editMessageText"), "no edits across resolve")
}

// TestTC_TMU_024fi covers FR-024 / AC-024-005: a 400 response with
// 'message is not modified' from editMessageText is swallowed (Notify
// returns nil) and the cache state is unchanged.
//
// @aitri-trace FR-024 AC-024-005 BL-024 TC-TMU-024fi
func TestTC_TMU_024fi_NotModifiedSwallowed(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 5, 7, 16, 0, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 92.4)))
	clk.Advance(20 * time.Second)
	rec.mu.Lock()
	rec.response["editMessageText"] = canned{
		400,
		`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`,
	}
	rec.mu.Unlock()

	err := sender.Notify(context.Background(), makeFireEvent(7, 92.4))
	assert.NoError(t, err, "spurious 'not modified' must not surface as error")
	mid, fc, ok := sender.storm.Snapshot(7)
	require.True(t, ok)
	assert.Equal(t, int64(555), mid, "cache message_id preserved")
	assert.Equal(t, 1, fc, "RecordEdit not called when API returned 400")
}

// TestTC_TMU_024di covers FR-024 / AC-024-001 'message to edit not
// found' fallback: the cache is cleared and a fresh sendMessage is
// issued. Operator still receives an alert for the new fire.
//
// @aitri-trace FR-024 BL-024 TC-TMU-024di
func TestTC_TMU_024di_MessageToEditNotFoundFallsBackToSend(t *testing.T) {
	rec := newRecorder()
	clk := newFakeClock(time.Date(2026, 5, 7, 16, 0, 0, 0, time.UTC))
	sender := newStormTelegramSender(t, rec, clk)

	require.NoError(t, sender.Notify(context.Background(), makeFireEvent(7, 92.4)))
	clk.Advance(20 * time.Second)
	rec.mu.Lock()
	rec.response["editMessageText"] = canned{
		400,
		`{"ok":false,"error_code":400,"description":"Bad Request: message to edit not found"}`,
	}
	rec.response["sendMessage"] = canned{200, `{"ok":true,"result":{"message_id":999}}`}
	rec.mu.Unlock()

	err := sender.Notify(context.Background(), makeFireEvent(7, 92.4))
	assert.NoError(t, err)
	assert.Equal(t, 1, rec.countOf("editMessageText"), "edit attempted once")
	assert.Equal(t, 2, rec.countOf("sendMessage"), "fallback send issued after edit-not-found")
	mid, _, ok := sender.storm.Snapshot(7)
	require.True(t, ok)
	assert.Equal(t, int64(999), mid, "cache replaced with new message_id from fallback send")
}
