// @aitri-tc TC-SR-057h, TC-SR-058h, TC-SR-061h, TC-SR-062h, TC-SR-062f,
// TC-SR-065h, TC-SR-066h, TC-SR-066e, TC-SR-067h, TC-SR-067e (subset),
// TC-SR-067f, TC-SR-069e, TC-SR-069f, TC-SR-070h
//
// These tests verify the rendered /settings HTML markup against the FRs.
// They use string assertions (no goquery dep) — looking for stable
// data-attribute selectors and absence of legacy markup.
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: GET /settings as authenticated user, return body
func getSettingsBody(t *testing.T) string {
	t.Helper()
	srv, session := setupSSETestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

// TC-SR-057h — every Performance numeric field renders a stepper widget with
// the exact range hint substring in its label.
func TestTC_SR_057h_PerformanceStepperMarkup(t *testing.T) {
	body := getSettingsBody(t)

	for _, field := range []string{"sse_interval_sec", "disk_interval_min", "docker_interval_sec", "systemd_interval_sec"} {
		assert.Contains(t, body, `data-widget="stepper"`, "stepper marker missing")
		assert.Contains(t, body, `data-field="`+field+`"`, "field=%s missing", field)
	}
	assert.Contains(t, body, "Dashboard refresh (2–60 sec)")
	assert.Contains(t, body, "Disk check (1–1440 min)")
	assert.Contains(t, body, "Docker refresh (5–300 sec)")
	assert.Contains(t, body, "Services refresh (5–300 sec)")
}

// TC-SR-058h — alert-rule form has segmented severity control; no <select name="severity">.
func TestTC_SR_058h_SegmentedSeverityMarkup(t *testing.T) {
	body := getSettingsBody(t)
	assert.Contains(t, body, `data-widget="segmented"`)
	assert.Contains(t, body, `data-field="severity"`)
	assert.Contains(t, body, `data-value="critical"`)
	assert.Contains(t, body, `data-value="warning"`)
	assert.Contains(t, body, `data-value="info"`)
	assert.NotContains(t, body, `<select name="severity"`, "legacy <select name=severity> must be removed")
}

// TC-SR-061h — Telegram + Email enabled toggles (replace checkboxes).
func TestTC_SR_061h_NotificationTogglesMarkup(t *testing.T) {
	body := getSettingsBody(t)
	assert.Contains(t, body, `data-widget="toggle"`)
	assert.Contains(t, body, `data-field="telegram_enabled"`)
	assert.Contains(t, body, `data-field="email_enabled"`)
	// The hidden <input type="checkbox" name="enabled"> still exists for the
	// form submit, but it is now sr-only (toggle widget wraps it).
	// Assert sr-only form enabled inputs exist (FR-061 keeps the same wire format):
	assert.Contains(t, body, `data-toggle-input`)
}

// TC-SR-062h — Backup section renders Limits / Schedule / Destination sub-headings in order.
func TestTC_SR_062h_BackupSubdivisions(t *testing.T) {
	body := getSettingsBody(t)
	idxL := strings.Index(body, `data-subsection="limits"`)
	idxS := strings.Index(body, `data-subsection="schedule"`)
	idxD := strings.Index(body, `data-subsection="destination"`)
	require.NotEqual(t, -1, idxL, "limits sub-section missing")
	require.NotEqual(t, -1, idxS, "schedule sub-section missing")
	require.NotEqual(t, -1, idxD, "destination sub-section missing")
	assert.True(t, idxL < idxS && idxS < idxD, "sub-section order must be limits → schedule → destination")
	// Eyebrow text present
	assert.Contains(t, body, ">Limits<")
	assert.Contains(t, body, ">Schedule<")
	assert.Contains(t, body, ">Destination<")
}

// TC-SR-062f — sub-headings use the canonical eyebrow class set.
func TestTC_SR_062f_SubheadingsEyebrowStyle(t *testing.T) {
	body := getSettingsBody(t)
	// Each data-subsection-heading must carry the eyebrow class set
	// (text-sm font-semibold text-text-muted uppercase tracking-wider).
	count := strings.Count(body, `data-subsection-heading class="text-sm font-semibold text-text-muted uppercase tracking-wider`)
	assert.Equal(t, 3, count, "expected 3 sub-section headings with the canonical eyebrow class set")
}

// TC-SR-065h — at idle, no [data-form-state-pill] HTML element is rendered
// server-side. (The JS that creates pills on submit may reference the
// attribute name as a string literal — that's expected and harmless.)
func TestTC_SR_065h_FormStatePillIdleAbsent(t *testing.T) {
	body := getSettingsBody(t)
	// An actual rendered element would have `data-form-state-pill="..."` —
	// double-quoted attribute form, which the JS literal never produces.
	assert.NotContains(t, body, `data-form-state-pill="`, "no rendered pill element at idle (FR-065)")
	// Hosts are present but empty.
	assert.Contains(t, body, `data-form-state-host="performance"`)
	assert.Contains(t, body, `data-form-state-host="telegram"`)
}

// TC-SR-066h — no `01`-`07` numbered badge markup remains.
func TestTC_SR_066h_NoNumberBadges(t *testing.T) {
	body := getSettingsBody(t)
	for _, n := range []string{`>01<`, `>02<`, `>03<`, `>04<`, `>05<`, `>06<`, `>07<`} {
		assert.NotContains(t, body, n, "legacy section badge %q must not appear", n)
	}
}

// TC-SR-066e — section ordering preserved, identified by data-section attr.
func TestTC_SR_066e_SectionsInOrder(t *testing.T) {
	body := getSettingsBody(t)
	expected := []string{"alerts", "telegram", "email", "performance", "backup", "maintenance", "controls"}
	last := -1
	for _, s := range expected {
		idx := strings.Index(body, `data-section="`+s+`"`)
		require.NotEqual(t, -1, idx, "section %q missing", s)
		assert.Greater(t, idx, last, "section %q out of order", s)
		last = idx
	}
}

// TC-SR-067h — Logout NOT in #settings-controls.
func TestTC_SR_067h_NoLogoutInSystemControls(t *testing.T) {
	body := getSettingsBody(t)
	// Find the controls section start
	start := strings.Index(body, `id="settings-controls"`)
	require.NotEqual(t, -1, start, "controls section must exist")
	end := strings.Index(body[start:], `</section>`)
	require.NotEqual(t, -1, end)
	controlsBlock := body[start : start+end]
	assert.NotContains(t, controlsBlock, `action="/logout"`, "Logout must NOT be inside #settings-controls (FR-067)")
	assert.NotContains(t, controlsBlock, `data-action="logout"`)
	// Restart and Shutdown still present
	assert.Contains(t, controlsBlock, `data-action-card="restart"`)
	assert.Contains(t, controlsBlock, `data-action-card="shutdown"`)
}

// TC-SR-067e — header dropdown contains Logout on /settings AND on /.
func TestTC_SR_067e_HeaderLogoutOnEveryPage(t *testing.T) {
	for _, page := range []string{"/", "/settings"} {
		t.Run(page, func(t *testing.T) {
			srv, session := setupSSETestServer(t)
			req := httptest.NewRequest(http.MethodGet, page, nil)
			req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
			rec := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, "page %s must return 200", page)
			body := rec.Body.String()
			assert.Contains(t, body, `data-header-dropdown`, "header dropdown missing on %s", page)
			assert.Contains(t, body, `action="/logout"`, "logout form missing on %s", page)
			assert.Contains(t, body, `data-action="logout"`)
			// CSRF token must be present in the logout form
			assert.Contains(t, body, `name="csrf_token"`, "csrf_token must accompany logout")
		})
	}
}

// TC-SR-067f — Shutdown card has destructive treatment; Restart card does not.
func TestTC_SR_067f_ShutdownDestructive(t *testing.T) {
	body := getSettingsBody(t)
	// Locate shutdown card block
	start := strings.Index(body, `data-action-card="shutdown"`)
	require.NotEqual(t, -1, start)
	end := strings.Index(body[start:], `</article>`)
	require.NotEqual(t, -1, end)
	shutdownBlock := body[start : start+end]
	assert.Contains(t, shutdownBlock, "border-danger")
	assert.Contains(t, shutdownBlock, "Destructive")

	// Restart card must NOT carry the eyebrow.
	rstart := strings.Index(body, `data-action-card="restart"`)
	require.NotEqual(t, -1, rstart)
	rend := strings.Index(body[rstart:], `</article>`)
	require.NotEqual(t, -1, rend)
	restartBlock := body[rstart : rstart+rend]
	assert.NotContains(t, restartBlock, "Destructive", "restart must not carry destructive eyebrow")
}

// TC-SR-069e/f — English copy on settings + sidebar; no Spanish substrings.
func TestTC_SR_069e_NoSpanishStrings(t *testing.T) {
	body := getSettingsBody(t)
	for _, sp := range []string{"Ajusta", "enfoque seguro", "Expandir/contraer"} {
		assert.NotContains(t, body, sp, "spanish substring %q must be removed", sp)
	}
	assert.Contains(t, body, "Configure alerts, notifications, performance, and maintenance")
	// Sidebar tooltip
	assert.Contains(t, body, `title="Expand / collapse sidebar"`)
}

// TC-SR-070h — no settings <form> uses max-w-4xl.
func TestTC_SR_070h_NoMaxW4xlOnForms(t *testing.T) {
	body := getSettingsBody(t)
	// Find the settings shell, ensure no <form> with max-w-4xl
	shellStart := strings.Index(body, `id="settings-shell"`)
	require.NotEqual(t, -1, shellStart)
	// Crude: ensure no occurrence of max-w-4xl after the shell start (settings forms are after this).
	settingsHTML := body[shellStart:]
	// allow class strings to contain 'max-w-4xl' only zero times in the settings region
	assert.NotContains(t, settingsHTML, "max-w-4xl", "no settings form should use max-w-4xl (FR-070)")
}
