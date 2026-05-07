// Package render produces the Telegram MarkdownV2 body, email HTML body,
// email plain-text fallback, and email subject for a single alert event.
//
// Render is deterministic and side-effect-free: it takes a fully-populated
// Input and returns the four rendered strings plus the truncation step that
// fired. All I/O (journalctl, docker logs, /proc top-process scan) happens
// upstream in the dispatcher; the renderer never blocks on subprocesses.
//
// Truncation order (FR-028):
//  1. journal/log block to ≤300 chars
//  2. trend line removed
//  3. probable-cause line removed
//  4. journal/log block to ≤100 chars + "… (truncated)"
//  5. minimal fallback (severity + metric label + value + threshold + footer)
//
// @aitri-trace FR-016 FR-017 FR-018 FR-019 FR-022 FR-023 FR-027 FR-028 FR-029 NFR-006
package render

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/cause"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/markdown"
)

// MaxBodyBytes is Telegram's hard cap on a sendMessage / editMessageText
// body. Renders that would exceed this trigger the truncation chain.
const MaxBodyBytes = 4096

// MaxSubjectChars is the lock-screen-preview budget on a 375px phone. Used by
// FR-017 AC-017 and the email Subject header.
const MaxSubjectChars = 80

// MaxSurfaceBlock is the FR-020 / FR-021 default limit for journal / docker
// log blocks before the truncation chain kicks in.
const MaxSurfaceBlock = 600

// EventKind identifies whether an alert event is a fresh fire or a recovery
// (resolve). Mirrors the dispatcher's notion; redeclared here so the render
// package is decoupled from the upstream Event type.
type EventKind string

const (
	KindFire    EventKind = "fire"
	KindResolve EventKind = "resolve"
)

// Surface identifies which alert family this event belongs to. Drives which
// surface-specific block (journal vs docker logs vs none) appears in the body.
type Surface string

const (
	SurfaceResource Surface = "resource"
	SurfaceSystemd  Surface = "systemd"
	SurfaceDocker   Surface = "docker"
)

// SystemdData carries the systemd-specific surface fields. Populated by the
// dispatcher when Surface=SurfaceSystemd.
type SystemdData struct {
	Unit          string
	ActiveState   string    // active|inactive|failed|activating|deactivating
	ActiveEnter   time.Time // ActiveEnterTimestamp
	JournalLines  []string  // last 3 lines, already filtered by dispatcher
	JournalErrMsg string    // when set, replaces JournalLines with "journal unavailable: ..."
}

// DockerData carries the docker-specific surface fields.
type DockerData struct {
	Container    string
	Image        string
	State        string // running|exited|paused|restarting|dead
	ExitCode     int    // valid only when State=="exited"
	LogLines     []string
	LogErrMsg    string // when set, replaces LogLines with "docker logs unavailable: ..."
	HasExitCode  bool   // distinguishes "exit 0" from "no exit code"
}

// Trend is the FR-022 5-minute trend hint. Nil ⇒ omit the trend line.
type Trend struct {
	Prior   float64
	Current float64
	PriorAt time.Time // sample timestamp; renderer does NOT re-validate window
	Unit    string    // "%", "°C", etc.
}

// Input is the renderer's sole argument. Build it in the dispatcher and pass
// it as a value (it is small).
type Input struct {
	Alert        *database.Alert       // engine-emitted; required
	Rule         *database.AlertConfig // matching config; nil ⇒ threshold n/a
	Kind         EventKind             // fire | resolve
	Surface      Surface
	FirstFiredAt time.Time     // zero ⇒ render absolute timestamp branch
	ResolvedAt   time.Time     // valid when Kind=resolve
	Now          time.Time     // injected for tests
	Hostname     string        // os.Hostname() at process start; cached upstream
	PublicURL    string        // ULTRON_PUBLIC_URL or derived; renderer never re-derives
	Trend        *Trend        // optional, resource only
	Cause        *cause.Cause  // optional; nil ⇒ omit situation line
	Systemd      *SystemdData  // when Surface=systemd
	Docker       *DockerData   // when Surface=docker
	Test         bool          // true when this is the synthetic Test Telegram preview
}

// Output is what Render returns.
type Output struct {
	TelegramMD    string
	EmailHTML     string
	EmailPlain    string
	EmailSubject  string
	TruncatedStep string // none|journal_300|trend|cause|journal_100|minimal
	CauseSource   string // proc|journal|exitcode|none — for NFR-007 log line
}

// friendlyLabels maps engine metric identifiers (and historical aliases) to
// the human label used in subjects and metric lines.
//
// @aitri-trace FR-017 AC-017-001
var friendlyLabels = map[string]string{
	"cpu":       "CPU usage",
	"cpu_usage": "CPU usage",
	"ram":       "RAM usage",
	"mem":       "RAM usage",
	"memory":    "RAM usage",
	"mem_usage": "RAM usage",
	"disk":      "Disk usage",
	"disk_usage": "Disk usage",
	"temp":      "CPU temperature",
	"cpu_temp":  "CPU temperature",
	"temperature": "CPU temperature",
}

// metricUnit returns the per-metric display unit (used by the metric line
// rendering, e.g. "92%" vs "78°C"). Defaults to "" for unknown metrics.
func metricUnit(metric string) string {
	switch metric {
	case "cpu", "cpu_usage", "ram", "mem", "memory", "mem_usage", "disk", "disk_usage":
		return "%"
	case "temp", "cpu_temp", "temperature":
		return "°C"
	}
	return ""
}

// titleCase converts a snake_case identifier to a Title Case label, used as
// the FR-017 AC-017-002 fallback when no friendly label is mapped.
func titleCase(snake string) string {
	parts := strings.Split(snake, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(p)
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

// severityGlyph maps severity to the emoji used in fire subjects.
func severityGlyph(sev string) string {
	switch sev {
	case "critical":
		return "🔴"
	case "warning":
		return "🟡"
	case "info":
		return "🔵"
	default:
		return "🔴"
	}
}

// hostname returns the hostname for the subject, falling back to "host" when
// empty (FR-017's resilience requirement — never produce an empty subject).
func hostnameOrDefault(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return "host"
	}
	return h
}

// metricLabel resolves the friendly label for in.Alert.Source / in.Rule.Metric.
// Returns the label and an "unmapped" flag the dispatcher uses to emit the
// FR-017 AC-017-002 info-level log entry.
func metricLabel(in Input) (label string, unmapped bool) {
	candidates := make([]string, 0, 3)
	if in.Rule != nil && in.Rule.Metric != "" {
		candidates = append(candidates, in.Rule.Metric)
	}
	if in.Alert != nil {
		// Source can be either a bare metric ("cpu") or a prefixed surface
		// identifier ("docker:name"). Strip any prefix for label lookup.
		src := in.Alert.Source
		if idx := strings.Index(src, ":"); idx >= 0 {
			src = src[:idx]
		}
		if src != "" {
			candidates = append(candidates, src)
		}
	}
	for _, c := range candidates {
		if v, ok := friendlyLabels[c]; ok {
			return v, false
		}
	}
	// Fallback: title-case the first non-empty candidate.
	for _, c := range candidates {
		return titleCase(c), true
	}
	return "Alert", true
}

// Render is the entry point. It NEVER returns an empty body and NEVER
// returns an error after recovering from a panic — the minimal-fallback
// path always succeeds (NFR-006).
//
// @aitri-trace FR-016 FR-017 FR-018 FR-019 FR-022 FR-023 FR-027 FR-028 NFR-006
func Render(in Input) (out Output) {
	defer func() {
		if r := recover(); r != nil {
			out = renderMinimalFallback(in, fmt.Sprintf("panic: %v", r))
		}
	}()

	if in.Now.IsZero() {
		in.Now = time.Now()
	}

	// Build the candidate body with all blocks; the truncation chain prunes
	// it down if it exceeds MaxBodyBytes.
	body, plain, html, sub := buildAll(in)
	out.TelegramMD = body
	out.EmailPlain = plain
	out.EmailHTML = html
	out.EmailSubject = sub
	out.TruncatedStep = "none"
	out.CauseSource = causeSource(in.Cause)

	// Fast path: under cap, done.
	if utf8.RuneCountInString(body) <= MaxBodyBytes && len(body) <= MaxBodyBytes {
		return
	}

	// Truncation chain.
	for _, step := range []struct {
		name  string
		apply func(*Input)
	}{
		{"journal_300", func(in *Input) { truncateSurfaceBlock(in, 300) }},
		{"trend", func(in *Input) { in.Trend = nil }},
		{"cause", func(in *Input) { in.Cause = nil }},
		{"journal_100", func(in *Input) { truncateSurfaceBlock(in, 100) }},
	} {
		step.apply(&in)
		body, plain, html, sub = buildAll(in)
		if len(body) <= MaxBodyBytes {
			out.TelegramMD = body
			out.EmailPlain = plain
			out.EmailHTML = html
			out.EmailSubject = sub
			out.TruncatedStep = step.name
			return
		}
	}

	// Last resort: minimal fallback.
	return renderMinimalFallback(in, "size cap exceeded after all truncation steps")
}

func buildAll(in Input) (telegramMD, emailPlain, emailHTML, emailSubject string) {
	subject, _ := buildSubject(in)
	emailSubject = stripLeadingEmoji(subject)
	if utf8.RuneCountInString(emailSubject) > MaxSubjectChars {
		emailSubject = truncateRunes(emailSubject, MaxSubjectChars)
	}

	blocks := buildBlocks(in)

	// Telegram: subject + blocks, escaped per block authoring rules.
	var bMD strings.Builder
	bMD.WriteString(subject)
	for _, b := range blocks {
		if b.telegramMD == "" {
			continue
		}
		bMD.WriteString("\n")
		bMD.WriteString(b.telegramMD)
	}
	telegramMD = bMD.String()

	// Plain (used as email plain-text alternative).
	var bP strings.Builder
	bP.WriteString(stripLeadingEmoji(subject))
	for _, b := range blocks {
		if b.plain == "" {
			continue
		}
		bP.WriteString("\n")
		bP.WriteString(b.plain)
	}
	emailPlain = bP.String()

	// HTML.
	var bH strings.Builder
	bH.WriteString(`<h2 data-block="subject">`)
	bH.WriteString(htmlEscape(stripLeadingEmoji(subject)))
	bH.WriteString(`</h2>`)
	for _, b := range blocks {
		if b.html == "" {
			continue
		}
		bH.WriteString(b.html)
	}
	emailHTML = bH.String()
	return
}

// blockOut is one logical content block produced for all three output forms.
type blockOut struct {
	name       string // for HTML data-block attribute
	telegramMD string
	plain      string
	html       string
}

// buildBlocks assembles every block in canonical order. Empty blocks are
// dropped at write time so the order is preserved without n/a clutter.
func buildBlocks(in Input) []blockOut {
	out := make([]blockOut, 0, 8)
	if b, ok := buildMetricLine(in); ok {
		out = append(out, b)
	}
	if b, ok := buildTimestampLine(in); ok {
		out = append(out, b)
	}
	if b, ok := buildSurfaceBlock(in); ok {
		out = append(out, b)
	}
	if b, ok := buildTrendLine(in); ok {
		out = append(out, b)
	}
	if b, ok := buildCauseLine(in); ok {
		out = append(out, b)
	}
	if b, ok := buildFooter(in); ok {
		out = append(out, b)
	}
	return out
}

// buildSubject implements FR-017 + FR-018 (severity glyph + ALERT FIRED /
// RESOLVED + storm counter when relevant).
func buildSubject(in Input) (string, bool) {
	host := hostnameOrDefault(in.Hostname)
	hostEsc := markdown.EscapeV2(host)

	if in.Kind == KindResolve {
		dur := durationStr(in.ResolvedAt.Sub(in.FirstFiredAt))
		// Resolve subject: ✓ <Friendly Metric> RESOLVED on host — active for X
		label, _ := metricLabel(in)
		labelEsc := markdown.EscapeV2(label)
		base := fmt.Sprintf("✓ %s RESOLVED on %s — active for %s", labelEsc, hostEsc, dur)
		return enforceSubjectLength(base), true
	}

	prefix := "TEST — "
	if !in.Test {
		prefix = ""
	}

	severity := strings.ToLower(safeStr(in.Alert, "Severity"))
	glyph := severityGlyph(severity)
	label, _ := metricLabel(in)
	labelEsc := markdown.EscapeV2(label)

	// Word form of severity for the subject line — uppercase the severity
	// keyword consistent with the "ALERT FIRED" register.
	severityWord := severity
	if severity == "" {
		severityWord = "alert"
	}
	subject := fmt.Sprintf("%s%s %s %s on %s", prefix, glyph, labelEsc, severityWord, hostEsc)
	return enforceSubjectLength(subject), true
}

// enforceSubjectLength guards FR-017 / TC-TMU-017f against subject overflow.
func enforceSubjectLength(s string) string {
	if utf8.RuneCountInString(s) <= MaxSubjectChars {
		return s
	}
	return truncateRunes(s, MaxSubjectChars-1) + "…"
}

// buildMetricLine implements FR-016 (threshold-aware) and the elapsed-since
// suffix ("for 1m 20s") when FirstFiredAt is set on a fire (FR-019).
func buildMetricLine(in Input) (blockOut, bool) {
	if in.Alert == nil || in.Kind == KindResolve {
		return blockOut{}, false
	}
	value := ""
	if in.Alert.Value != nil {
		// Match historical FormatAlertMessage output: integer % for resource
		// metrics; one-decimal otherwise.
		unit := metricUnit(safeMetricID(in))
		switch unit {
		case "%":
			value = fmt.Sprintf("%.0f%s", *in.Alert.Value, unit)
		case "°C":
			value = fmt.Sprintf("%.0f%s", *in.Alert.Value, unit)
		default:
			value = fmt.Sprintf("%.1f", *in.Alert.Value)
		}
	}

	label, _ := metricLabel(in)
	// Short label form: "CPU 92%" instead of "CPU usage 92%" — the subject
	// already carries "usage" / "temperature" so the metric line uses the
	// shorter family token.
	short := strings.SplitN(label, " ", 2)[0]
	threshold := "(threshold n/a)"
	if in.Rule != nil && in.Rule.Operator != "" {
		// Render thresholds as integer when a resource metric is involved.
		unit := metricUnit(safeMetricID(in))
		switch unit {
		case "%", "°C":
			threshold = fmt.Sprintf("(threshold %s %.0f%s)", in.Rule.Operator, in.Rule.Threshold, unit)
		default:
			threshold = fmt.Sprintf("(threshold %s %.1f)", in.Rule.Operator, in.Rule.Threshold)
		}
	}

	suffix := ""
	if !in.FirstFiredAt.IsZero() {
		d := in.Now.Sub(in.FirstFiredAt)
		if d > 0 {
			suffix = " for " + durationStr(d)
		}
	}

	main := fmt.Sprintf("ALERT FIRED — %s %s %s%s", short, value, threshold, suffix)
	return blockOut{
		name:       "metric",
		telegramMD: main,
		plain:      main,
		html:       "<p data-block=\"metric\">" + htmlEscape(main) + "</p>",
	}, true
}

// buildTimestampLine implements FR-019 fallback path. Mutually exclusive
// with the elapsed suffix on the metric line.
func buildTimestampLine(in Input) (blockOut, bool) {
	if !in.FirstFiredAt.IsZero() {
		return blockOut{}, false
	}
	if in.Alert == nil {
		return blockOut{}, false
	}
	utcStr := in.Alert.CreatedAt.UTC().Format(time.RFC3339)
	if in.Alert.CreatedAt.IsZero() {
		utcStr = in.Now.UTC().Format(time.RFC3339)
	}
	local := in.Alert.CreatedAt
	if local.IsZero() {
		local = in.Now
	}
	localStr := local.Format("2006-01-02 15:04:05 MST")
	main := fmt.Sprintf("%s (local: %s)", utcStr, localStr)
	return blockOut{
		name:       "timestamp",
		telegramMD: markdown.EscapeV2(main),
		plain:      main,
		html:       "<p data-block=\"timestamp\">" + htmlEscape(main) + "</p>",
	}, true
}

// buildSurfaceBlock dispatches to the systemd / docker variants.
func buildSurfaceBlock(in Input) (blockOut, bool) {
	switch in.Surface {
	case SurfaceSystemd:
		if in.Systemd == nil {
			return blockOut{}, false
		}
		return buildSystemdBlock(in.Systemd), true
	case SurfaceDocker:
		if in.Docker == nil {
			return blockOut{}, false
		}
		return buildDockerBlock(in.Docker), true
	}
	return blockOut{}, false
}

func buildSystemdBlock(d *SystemdData) blockOut {
	unitEsc := markdown.EscapeV2(d.Unit)
	state := markdown.EscapeV2(d.ActiveState)
	since := "never"
	if !d.ActiveEnter.IsZero() {
		since = d.ActiveEnter.UTC().Format(time.RFC3339)
	}
	header := fmt.Sprintf("%s · %s · active since %s", unitEsc, state, markdown.EscapeV2(since))
	body := ""
	switch {
	case d.JournalErrMsg != "":
		body = "journal unavailable: " + markdown.EscapeV2(d.JournalErrMsg)
	case len(d.JournalLines) == 0:
		body = "no recent journal entries"
	default:
		joined := strings.Join(d.JournalLines, "\n")
		joined = capJournalText(joined, MaxSurfaceBlock)
		body = mdEscapeMultiline(joined)
	}
	telegramMD := header + "\n" + body

	// Plain (no escapes; raw text).
	plainHeader := fmt.Sprintf("%s · %s · active since %s", d.Unit, d.ActiveState, since)
	plainBody := ""
	switch {
	case d.JournalErrMsg != "":
		plainBody = "journal unavailable: " + d.JournalErrMsg
	case len(d.JournalLines) == 0:
		plainBody = "no recent journal entries"
	default:
		plainBody = capJournalText(strings.Join(d.JournalLines, "\n"), MaxSurfaceBlock)
	}
	plain := plainHeader + "\n" + plainBody

	html := `<section data-block="surface-systemd">` +
		"<p>" + htmlEscape(plainHeader) + "</p>" +
		"<pre><code>" + htmlEscape(plainBody) + "</code></pre>" +
		`</section>`
	return blockOut{name: "surface", telegramMD: telegramMD, plain: plain, html: html}
}

func buildDockerBlock(d *DockerData) blockOut {
	containerEsc := markdown.EscapeV2(d.Container)
	imageEsc := markdown.EscapeV2(d.Image)
	state := markdown.EscapeV2(d.State)
	header := fmt.Sprintf("%s · %s · %s", containerEsc, imageEsc, state)
	if d.HasExitCode && d.State == "exited" {
		header += fmt.Sprintf(" · exit code %d", d.ExitCode)
	}
	body := ""
	switch {
	case d.LogErrMsg != "":
		body = "docker logs unavailable: " + markdown.EscapeV2(d.LogErrMsg)
	case len(d.LogLines) == 0:
		body = "no recent log lines"
	default:
		joined := strings.Join(d.LogLines, "\n")
		joined = capJournalText(joined, MaxSurfaceBlock)
		body = mdEscapeMultiline(joined)
	}
	telegramMD := header + "\n" + body

	plainHeader := fmt.Sprintf("%s · %s · %s", d.Container, d.Image, d.State)
	if d.HasExitCode && d.State == "exited" {
		plainHeader += fmt.Sprintf(" · exit code %d", d.ExitCode)
	}
	plainBody := ""
	switch {
	case d.LogErrMsg != "":
		plainBody = "docker logs unavailable: " + d.LogErrMsg
	case len(d.LogLines) == 0:
		plainBody = "no recent log lines"
	default:
		plainBody = capJournalText(strings.Join(d.LogLines, "\n"), MaxSurfaceBlock)
	}
	plain := plainHeader + "\n" + plainBody

	html := `<section data-block="surface-docker">` +
		"<p>" + htmlEscape(plainHeader) + "</p>" +
		"<pre><code>" + htmlEscape(plainBody) + "</code></pre>" +
		`</section>`
	return blockOut{name: "surface", telegramMD: telegramMD, plain: plain, html: html}
}

// buildTrendLine implements FR-022.
func buildTrendLine(in Input) (blockOut, bool) {
	if in.Trend == nil {
		return blockOut{}, false
	}
	if in.Surface != SurfaceResource {
		return blockOut{}, false
	}
	delta := in.Trend.Current - in.Trend.Prior
	sign := "+"
	if delta < 0 {
		sign = ""
	}
	unit := in.Trend.Unit
	if unit == "" {
		unit = "%"
	}
	main := fmt.Sprintf("trend: %.0f%s → %.0f%s (Δ %s%.0f%s)",
		in.Trend.Prior, unit, in.Trend.Current, unit, sign, delta, unit)
	return blockOut{
		name:       "trend",
		telegramMD: markdown.EscapeV2(main),
		plain:      main,
		html:       `<p data-block="trend">` + htmlEscape(main) + `</p>`,
	}, true
}

// buildCauseLine implements FR-029.
func buildCauseLine(in Input) (blockOut, bool) {
	if in.Cause == nil || in.Kind == KindResolve {
		return blockOut{}, false
	}
	line := in.Cause.Line
	return blockOut{
		name:       "cause",
		telegramMD: markdown.EscapeV2(line),
		plain:      line,
		html:       `<p data-block="cause">` + htmlEscape(line) + `</p>`,
	}, true
}

// buildFooter implements FR-023. ALWAYS present.
func buildFooter(in Input) (blockOut, bool) {
	base := strings.TrimRight(in.PublicURL, "/")
	if base == "" {
		base = "http://localhost"
	}
	url := base + "/alerts"
	telegram := fmt.Sprintf("[Open dashboard](%s)", url)
	plain := "Open dashboard: " + url
	html := fmt.Sprintf(`<p data-block="footer"><a href="%s">Open dashboard</a></p>`, htmlAttr(url))
	return blockOut{
		name:       "footer",
		telegramMD: telegram,
		plain:      plain,
		html:       html,
	}, true
}

// renderMinimalFallback is the NFR-006 last-resort path. It NEVER reads from
// in.Trend / in.Cause / in.Systemd / in.Docker so a panic in any of those
// blocks cannot cascade.
func renderMinimalFallback(in Input, reason string) Output {
	host := hostnameOrDefault(in.Hostname)
	label, _ := metricLabel(in)
	short := strings.SplitN(label, " ", 2)[0]
	severity := strings.ToLower(safeStr(in.Alert, "Severity"))
	glyph := severityGlyph(severity)
	value := ""
	if in.Alert != nil && in.Alert.Value != nil {
		value = fmt.Sprintf("%.1f", *in.Alert.Value)
	}
	threshold := "n/a"
	if in.Rule != nil && in.Rule.Operator != "" {
		threshold = fmt.Sprintf("%s %.1f", in.Rule.Operator, in.Rule.Threshold)
	}
	url := strings.TrimRight(in.PublicURL, "/") + "/alerts"

	subject := fmt.Sprintf("%s %s %s on %s", glyph, markdown.EscapeV2(label), severity, markdown.EscapeV2(host))
	body := fmt.Sprintf("%s\n%s %s (threshold %s)\n[Open dashboard](%s)",
		subject, short, value, threshold, url)
	emailSubject := fmt.Sprintf("%s %s on %s", label, severity, host)
	emailPlain := fmt.Sprintf("%s\n%s %s (threshold %s)\nOpen dashboard: %s",
		emailSubject, short, value, threshold, url)
	emailHTML := fmt.Sprintf(`<h2 data-block="subject">%s</h2><p>%s %s (threshold %s)</p><p><a href="%s">Open dashboard</a></p>`,
		htmlEscape(emailSubject), htmlEscape(short), htmlEscape(value), htmlEscape(threshold), htmlAttr(url))

	_ = reason // logged by the dispatcher via NFR-006; kept for future field
	return Output{
		TelegramMD:    body,
		EmailPlain:    emailPlain,
		EmailHTML:     emailHTML,
		EmailSubject:  emailSubject,
		TruncatedStep: "minimal",
		CauseSource:   string(cause.SourceNone),
	}
}

// truncateSurfaceBlock applies the journal/log-block reduction step inline by
// rewriting the Input's surface data. Called by the truncation chain.
func truncateSurfaceBlock(in *Input, max int) {
	switch in.Surface {
	case SurfaceSystemd:
		if in.Systemd != nil && len(in.Systemd.JournalLines) > 0 {
			joined := strings.Join(in.Systemd.JournalLines, "\n")
			joined = capJournalText(joined, max)
			in.Systemd.JournalLines = []string{joined}
		}
	case SurfaceDocker:
		if in.Docker != nil && len(in.Docker.LogLines) > 0 {
			joined := strings.Join(in.Docker.LogLines, "\n")
			joined = capJournalText(joined, max)
			in.Docker.LogLines = []string{joined}
		}
	}
}

// capJournalText truncates s to the first max bytes, suffixing
// "… (truncated)" when truncation actually occurred.
func capJournalText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	const suffix = "… (truncated)"
	if max <= len(suffix) {
		return suffix
	}
	return s[:max-len(suffix)] + suffix
}

// mdEscapeMultiline escapes each rune of s for MarkdownV2 while preserving
// line breaks (which are not specials).
func mdEscapeMultiline(s string) string {
	return markdown.EscapeV2(s)
}

// safeStr fetches a named string field from *database.Alert without panicking
// when the alert is nil.
func safeStr(a *database.Alert, field string) string {
	if a == nil {
		return ""
	}
	switch field {
	case "Severity":
		return a.Severity
	case "Message":
		return a.Message
	case "Source":
		return a.Source
	}
	return ""
}

// safeMetricID picks the engine-canonical metric identifier from rule then
// alert source; used for unit selection.
func safeMetricID(in Input) string {
	if in.Rule != nil && in.Rule.Metric != "" {
		return in.Rule.Metric
	}
	if in.Alert != nil {
		s := in.Alert.Source
		if idx := strings.Index(s, ":"); idx >= 0 {
			return s[:idx]
		}
		return s
	}
	return ""
}

// stripLeadingEmoji returns s without a leading severity glyph (and the
// space after it). Used to produce the email subject (FR-027 AC-027-...).
func stripLeadingEmoji(s string) string {
	for _, glyph := range []string{"🔴 ", "🟡 ", "🔵 ", "✓ "} {
		if strings.HasPrefix(s, glyph) {
			return strings.TrimPrefix(s, glyph)
		}
	}
	return s
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	out := make([]rune, 0, n)
	for _, r := range s {
		if len(out) == n {
			break
		}
		out = append(out, r)
	}
	return string(out)
}

// durationStr formats a Duration as "1h 02m 03s", dropping leading zero
// components.
func durationStr(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	d -= time.Duration(h) * time.Hour
	m := int(d.Minutes())
	d -= time.Duration(m) * time.Minute
	s := int(d.Seconds())
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// causeSource returns a short string for the NFR-007 log line.
func causeSource(c *cause.Cause) string {
	if c == nil {
		return string(cause.SourceNone)
	}
	return string(c.Source)
}

// htmlEscape is a tiny stdlib-only escaper for use in plain string concat.
// We avoid html/template here because the renderer's templates are static
// fragments; the only attacker-controlled content is already markdown-escaped
// upstream — but for HTML output we still need <,>,&," escaping.
func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

// htmlAttr is htmlEscape with one extra: backslash. Used for href attributes.
func htmlAttr(s string) string {
	return htmlEscape(s)
}
