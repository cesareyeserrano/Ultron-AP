package notify

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/cause"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/render"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

// MetricsReader is the minimal slice of internal/metrics.Collector the
// dispatcher needs to populate FR-022 trend hints. The interface lives
// here so tests can inject a fake without a metrics.Collector.
//
// @aitri-trace FR-022
type MetricsReader interface {
	// History returns up to n most-recent snapshots in chronological
	// (oldest-first) order; matches metrics.Collector.History.
	History(n int) []metrics.Snapshot
}

// SystemdReader exposes the slice of systemd.Monitor the dispatcher needs
// to look up service state for FR-020 surface blocks.
//
// @aitri-trace FR-020
type SystemdReader interface {
	Services() []systemd.ServiceInfo
}

// DockerReader exposes the slice of docker.Monitor the dispatcher needs
// to look up container state for FR-021 surface blocks.
//
// @aitri-trace FR-021
type DockerReader interface {
	Containers() []docker.ContainerInfo
}

// CauseDeriver is the interface the dispatcher uses to populate the
// FR-029 probable-cause line. Each Resource / Systemd / Docker derivation
// has its own per-source ctx timeout (200ms by default).
//
// @aitri-trace FR-029
type CauseDeriver interface {
	Resource(ctx context.Context, metricID string, surf cause.ResourceData) (*cause.Cause, error)
	Systemd(ctx context.Context, unit string) (*cause.Cause, error)
	Docker(ctx context.Context, container string, exitCode int) (*cause.Cause, error)
}

// dockerExitCodeRE extracts the numeric exit code from docker's status
// string (e.g. "Exited (137) 5 minutes ago" → 137). Used by the surface
// populator below.
var dockerExitCodeRE = regexp.MustCompile(`Exited\s+\((\d+)\)`)

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
// PublicURL at construction. When a MetricsReader is wired, the dispatcher
// also populates Event.Trend for resource fires (FR-022).
type Dispatcher struct {
	db        *database.DB
	queue     chan *Event
	cancel    chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup

	// Notifier cache (BL-030): the send path reuses constructed notifiers — and
	// crucially the TelegramSender's storm.Cache — across fires so coalescing
	// persists. Rebuilt only when the channel config changes.
	notifierMu      sync.Mutex
	cachedNotifiers []Notifier
	notifierKey     string
	janitorStop     chan struct{} // stops the cached telegram sender's storm janitor
	hostname  string
	publicURL string
	metrics   MetricsReader // optional; nil ⇒ trend block omitted
	systemd   SystemdReader // optional; nil ⇒ systemd surface block omitted
	docker    DockerReader  // optional; nil ⇒ docker surface block omitted
	causeDrv  CauseDeriver  // optional; nil ⇒ cause line omitted
}

// SetMetricsReader wires the metrics ring buffer for FR-022 trend hints.
// Safe to call before Start().
//
// @aitri-trace FR-022
func (d *Dispatcher) SetMetricsReader(r MetricsReader) {
	d.metrics = r
}

// SetSystemdReader wires the systemd monitor for FR-020 surface blocks.
//
// @aitri-trace FR-020
func (d *Dispatcher) SetSystemdReader(r SystemdReader) {
	d.systemd = r
}

// SetDockerReader wires the docker monitor for FR-021 surface blocks.
//
// @aitri-trace FR-021
func (d *Dispatcher) SetDockerReader(r DockerReader) {
	d.docker = r
}

// SetCauseDeriver wires the FR-029 probable-cause derivation. Pass
// cause.New(procReader) at process start.
//
// @aitri-trace FR-029
func (d *Dispatcher) SetCauseDeriver(c CauseDeriver) {
	d.causeDrv = c
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

// Stop drains the queue and stops processing. It is idempotent and safe to
// call before Start() (no-op) or more than once — guarded by stopOnce + a nil
// check so it never panics on a double close or a nil cancel channel.
func (d *Dispatcher) Stop() {
	if d.cancel == nil {
		return // never started
	}
	d.stopOnce.Do(func() {
		close(d.cancel)
		d.wg.Wait()
		// Run loop has exited, so no send is building notifiers — stop the
		// cached telegram sender's storm janitor (BL-030).
		d.notifierMu.Lock()
		if d.janitorStop != nil {
			close(d.janitorStop)
			d.janitorStop = nil
		}
		d.notifierMu.Unlock()
		log.Println("Notification dispatcher stopped")
	})
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
//
// Emits one NFR-007 structured info line per Event (after fan-out
// completes) with rule_id, severity, surface, bytes_sent, render_ms,
// truncated_step, cause_source, channels_attempted, channels_failed.
//
// @aitri-trace NFR-007
func (d *Dispatcher) send(evt *Event) {
	if evt == nil {
		return
	}
	d.populateEventDefaults(evt)

	// Render once at the dispatcher level so we can attribute byte
	// count, ms, and truncation step to this Event in the structured
	// log line. The notifiers re-render internally — render is pure and
	// fast (≤50ms p95), the double-cost is acceptable for the
	// observability win.
	t0 := time.Now()
	out := render.Render(buildRenderInputFromEvent(evt))
	renderMs := time.Since(t0).Milliseconds()

	notifiers := d.getNotifiers()
	channelsFailed := 0
	for _, n := range notifiers {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := n.Notify(ctx, evt); err != nil {
			log.Printf("notify: %s notify failed: %v", n.Name(), err)
			channelsFailed++
		}
		cancel()
	}

	d.logSend(evt, out, renderMs, len(notifiers), channelsFailed)
}

// logSend emits a single structured info line summarising the dispatch
// (NFR-007). Format is space-separated key=value pairs so it's
// grep-friendly without needing a JSON parser; matches the project's
// existing log.Printf style. Empty / unknown values are omitted rather
// than logged as "0".
func (d *Dispatcher) logSend(evt *Event, out render.Output, renderMs int64, channels, failed int) {
	if evt == nil {
		return
	}
	ruleID := int64(0)
	if evt.Alert != nil && evt.Alert.ConfigID != nil {
		ruleID = *evt.Alert.ConfigID
	}
	severity := ""
	if evt.Alert != nil {
		severity = evt.Alert.Severity
	}
	causeSource := out.CauseSource
	if causeSource == "" {
		causeSource = "none"
	}
	log.Printf("notify.send rule_id=%d severity=%s surface=%s kind=%s bytes_sent=%d render_ms=%d truncated_step=%s cause_source=%s channels=%d failed=%d",
		ruleID, severity, evt.Surface, evt.Kind,
		len(out.TelegramMD), renderMs,
		out.TruncatedStep, causeSource,
		channels, failed)
}

// populateEventDefaults fills in fields that the engine may have left empty
// (Hostname, PublicURL, Rule, Trend). The engine emits the Event with
// surface + kind already set; the dispatcher resolves rule lookup centrally
// AND looks up the 5-minute prior sample for resource fires (FR-022) so each
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
	if evt.Trend == nil && evt.Kind == EventFire && evt.Surface == SurfaceResource && d.metrics != nil {
		evt.Trend = d.lookupTrend(evt)
	}
	if evt.Kind == EventFire {
		d.populateSurfaceBlocks(evt)
		d.populateCause(evt)
	}
}

// populateSurfaceBlocks fills evt.Systemd / evt.Docker for the matching
// surface (FR-020 / FR-021). Errors are non-fatal: an unavailable monitor
// or unknown unit/container leaves the surface field nil and the renderer
// emits no surface block.
//
// @aitri-trace FR-020 FR-021
func (d *Dispatcher) populateSurfaceBlocks(evt *Event) {
	if evt == nil || evt.Alert == nil {
		return
	}
	switch evt.Surface {
	case SurfaceSystemd:
		if d.systemd == nil || evt.Systemd != nil {
			return
		}
		evt.Systemd = d.buildSystemdData(evt)
	case SurfaceDocker:
		if d.docker == nil || evt.Docker != nil {
			return
		}
		evt.Docker = d.buildDockerData(evt)
	}
}

// buildSystemdData looks up the unit referenced by Alert.Source, fetches
// its current ServiceInfo, and pairs it with a journalctl tail (via the
// cause deriver) to produce a render.SystemdData.
func (d *Dispatcher) buildSystemdData(evt *Event) *render.SystemdData {
	unit := strings.TrimPrefix(evt.Alert.Source, "systemd:")
	if unit == "" {
		return nil
	}
	out := &render.SystemdData{Unit: unit}
	for _, s := range d.systemd.Services() {
		if s.Name == unit {
			out.ActiveState = s.ActiveState
			out.ActiveEnter = s.Since
			break
		}
	}
	if out.ActiveState == "" {
		// Service not in the cache; render with the alert's severity hint.
		out.ActiveState = "failed"
	}
	if d.causeDrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		// Use the cause package's journal helper for the tail; on error,
		// the surface block falls back to "journal unavailable".
		if c, err := d.causeDrv.Systemd(ctx, unit); err == nil && c != nil {
			// The cause line is "last error: <line>"; strip the prefix
			// so the surface block shows the raw line(s).
			out.JournalLines = []string{strings.TrimPrefix(c.Line, "last error: ")}
		} else if err != nil {
			out.JournalErrMsg = err.Error()
		}
	}
	return out
}

// buildDockerData looks up the container referenced by Alert.Source,
// fetches its current ContainerInfo, parses any "Exited (N)" exit code
// from Status, and pairs it with a docker logs tail.
func (d *Dispatcher) buildDockerData(evt *Event) *render.DockerData {
	name := strings.TrimPrefix(evt.Alert.Source, "docker:")
	if name == "" {
		return nil
	}
	out := &render.DockerData{Container: name, State: "exited"}
	for _, c := range d.docker.Containers() {
		if c.Name == name {
			out.Image = c.Image
			out.State = c.State
			if m := dockerExitCodeRE.FindStringSubmatch(c.Status); len(m) == 2 {
				if code, err := strconv.Atoi(m[1]); err == nil {
					out.ExitCode = code
					out.HasExitCode = true
				}
			}
			break
		}
	}
	if d.causeDrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if c, err := d.causeDrv.Docker(ctx, name, out.ExitCode); err == nil && c != nil {
			out.LogLines = []string{strings.TrimPrefix(strings.TrimPrefix(c.Line, "cause: "), "last error: ")}
		} else if err != nil {
			out.LogErrMsg = err.Error()
		}
	}
	return out
}

// populateCause drives the FR-029 probable-cause derivation per surface.
// The cause line is independent of the systemd/docker surface block —
// for resource fires it adds "top: ffmpeg (78%)"; for systemd it's the
// "last error: …" line; for docker it's the exit-code mapping.
//
// @aitri-trace FR-029
func (d *Dispatcher) populateCause(evt *Event) {
	if evt == nil || evt.Alert == nil || evt.Cause != nil || d.causeDrv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	switch evt.Surface {
	case SurfaceResource:
		c, err := d.causeDrv.Resource(ctx, metricKey(evt), causeResourceDataFor(evt))
		if err == nil && c != nil {
			evt.Cause = c
		}
	case SurfaceSystemd:
		// Systemd cause line shares the journal-tail derivation; reuse
		// the deriver here to avoid double-spawn.
		c, err := d.causeDrv.Systemd(ctx, strings.TrimPrefix(evt.Alert.Source, "systemd:"))
		if err == nil && c != nil {
			evt.Cause = c
		}
	case SurfaceDocker:
		exit := 0
		if evt.Docker != nil && evt.Docker.HasExitCode {
			exit = evt.Docker.ExitCode
		}
		c, err := d.causeDrv.Docker(ctx, strings.TrimPrefix(evt.Alert.Source, "docker:"), exit)
		if err == nil && c != nil {
			evt.Cause = c
		}
	}
}

// causeResourceDataFor extracts the FR-029 disk + temperature surface
// inputs for the cause helper. CPU and RAM derivation only need the
// metric ID; disk needs the largest mount %free; temperature needs the
// current value + 5-minute trend (already populated above).
func causeResourceDataFor(evt *Event) cause.ResourceData {
	out := cause.ResourceData{}
	if evt.Trend != nil {
		out.TempCurrentC = evt.Trend.Current
		out.TempTrendDeltaC = evt.Trend.Current - evt.Trend.Prior
	}
	return out
}

// lookupTrend scans the metrics ring buffer for a snapshot taken between
// 4m30s and 5m30s ago and returns a Trend struct comparing it to the
// alert's current value. Returns nil when no sample is in the window or
// the metric is non-resource.
//
// @aitri-trace FR-022 AC-022-001 AC-022-002
func (d *Dispatcher) lookupTrend(evt *Event) *render.Trend {
	if evt == nil || evt.Alert == nil || evt.Alert.Value == nil {
		return nil
	}
	// Pull a generous window (default eval cadence is 5s; 75 snapshots
	// covers ~6 min comfortably).
	hist := d.metrics.History(120)
	if len(hist) == 0 {
		return nil
	}
	now := time.Now()
	windowStart := now.Add(-5*time.Minute - 30*time.Second)
	windowEnd := now.Add(-4*time.Minute - 30*time.Second)
	var prior *metrics.Snapshot
	for i := range hist {
		ts := hist[i].Timestamp
		if ts.After(windowStart) && ts.Before(windowEnd) {
			prior = &hist[i]
			break
		}
	}
	if prior == nil {
		return nil
	}
	priorVal, ok := extractMetricValue(metricKey(evt), prior)
	if !ok {
		return nil
	}
	return &render.Trend{
		Prior:   priorVal,
		Current: *evt.Alert.Value,
		PriorAt: prior.Timestamp,
		Unit:    metricUnitForKey(metricKey(evt)),
	}
}

// metricKey returns the engine-canonical metric identifier for the event,
// stripping any "docker:" / "systemd:" prefix.
func metricKey(evt *Event) string {
	if evt == nil {
		return ""
	}
	if evt.Rule != nil && evt.Rule.Metric != "" {
		return evt.Rule.Metric
	}
	if evt.Alert != nil {
		return evt.Alert.Source
	}
	return ""
}

// extractMetricValue mirrors the engine's metric extractor (kept private
// here to avoid a public dependency on internal/alerts). Returns the
// numeric value and ok=false when the metric isn't covered.
func extractMetricValue(metric string, snap *metrics.Snapshot) (float64, bool) {
	if snap == nil {
		return 0, false
	}
	switch metric {
	case "cpu", "cpu_usage":
		return snap.CPU.TotalPercent, true
	case "ram", "mem_usage", "memory":
		return snap.RAM.Percent, true
	case "disk", "disk_usage":
		var max float64
		for _, p := range snap.Disks {
			if p.Percent > max {
				max = p.Percent
			}
		}
		return max, true
	case "temp", "cpu_temp", "temperature":
		if snap.Temperature != nil {
			return *snap.Temperature, true
		}
		return 0, false
	}
	return 0, false
}

// metricUnitForKey returns the FR-022 trend display unit per metric.
func metricUnitForKey(metric string) string {
	switch metric {
	case "temp", "cpu_temp", "temperature":
		return "°C"
	}
	return "%"
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

// getNotifiers returns the send-path notifier list, reusing the previously
// constructed notifiers (and their long-lived storm cache) when the channel
// config is unchanged. Rebuilding on every send was why storm coalescing never
// worked in production — each send got a fresh, empty cache (BL-030).
func (d *Dispatcher) getNotifiers() []Notifier {
	tgRow, _ := d.db.GetNotificationConfig("telegram")
	emRow, _ := d.db.GetNotificationConfig("email")
	key := notifierFingerprint(tgRow, emRow)

	d.notifierMu.Lock()
	defer d.notifierMu.Unlock()
	if key == d.notifierKey && d.cachedNotifiers != nil {
		return d.cachedNotifiers
	}

	// Config changed (or first build) — stop the previous janitor and rebuild.
	if d.janitorStop != nil {
		close(d.janitorStop)
		d.janitorStop = nil
	}
	notifiers := d.buildNotifiers()
	for _, n := range notifiers {
		if ts, ok := n.(*TelegramSender); ok {
			stop := make(chan struct{})
			d.janitorStop = stop
			ts.StartJanitor(stop)
			break
		}
	}
	d.cachedNotifiers = notifiers
	d.notifierKey = key
	return notifiers
}

// notifierFingerprint summarises the channel config so getNotifiers can detect
// changes and rebuild. A nil/disabled row contributes an empty marker.
func notifierFingerprint(tg, em *database.NotificationConfig) string {
	var b strings.Builder
	for _, row := range []*database.NotificationConfig{tg, em} {
		if row != nil && row.Enabled {
			b.WriteString(row.Config)
		}
		b.WriteByte('|')
	}
	return b.String()
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
