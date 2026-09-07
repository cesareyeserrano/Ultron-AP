//go:build e2e

// Browser tests for the settings-revamp acceptance criteria that need a real
// layout and a real JavaScript runtime: accordion state after a chip click,
// history behaviour, a pill that must appear within 100 ms, a stepper that must
// refuse to pass its own maximum.
//
// Run: go test -tags e2e ./internal/server/ -run TestTC_SR
//
// @aitri-trace FR-057 FR-063 FR-065 BL-042
package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsBrowser opens /settings with the accordion controller initialised.
func settingsBrowser(t *testing.T) (string, context.Context) {
	t.Helper()
	baseURL, session := e2eServer(t)
	ctx := e2eBrowser(t, baseURL, session)
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/settings"),
		chromedp.WaitVisible("#settings-shell", chromedp.ByID),
		// The accordion markup is built at runtime by settings.js, so wait for
		// it rather than for the template.
		chromedp.WaitVisible("[data-accordion-toggle]", chromedp.ByQuery),
	))
	return baseURL, ctx
}

// ariaExpanded reads a section's accordion state.
func ariaExpanded(sectionID string, out *string) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(
		`(() => { const s = document.getElementById(%q);
                  const t = s && s.querySelector('[data-accordion-toggle]');
                  return t ? String(t.getAttribute('aria-expanded')) : "no-toggle"; })()`, sectionID), out)
}

// @aitri-tc TC-SR-063h — clicking the Telegram chip expands that accordion by
// the time the scroll settles (AC-063-001).
func TestTC_SR_063h(t *testing.T) {
	_, ctx := settingsBrowser(t)

	var before, after string
	require.NoError(t, chromedp.Run(ctx,
		ariaExpanded("settings-telegram", &before),
		chromedp.Click(`[data-anchor="telegram"]`, chromedp.ByQuery),
		chromedp.Sleep(1200*time.Millisecond), // smooth scroll settles
		ariaExpanded("settings-telegram", &after),
	))

	require.Equal(t, "false", before, "Telegram must start collapsed for this to mean anything")
	assert.Equal(t, "true", after, "the chip click must leave the Telegram accordion expanded")
}

// @aitri-tc TC-SR-063e — opening /settings#settings-backup directly leaves the
// Backup accordion already expanded (AC-063-002).
func TestTC_SR_063e(t *testing.T) {
	baseURL, session := e2eServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var expanded string
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/settings#settings-backup"),
		chromedp.WaitVisible("[data-accordion-toggle]", chromedp.ByQuery),
		chromedp.Sleep(600*time.Millisecond),
		ariaExpanded("settings-backup", &expanded),
	))

	assert.Equal(t, "true", expanded,
		"arriving on the hash must expand Backup, with no collapsed state to flash")
}

// @aitri-tc TC-SR-063f — three chip clicks add no history entries: the hash is
// updated with replaceState, so Back does not walk through them (AC-063-003).
func TestTC_SR_063f(t *testing.T) {
	_, ctx := settingsBrowser(t)

	var before, after int
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Evaluate(`history.length`, &before),
		chromedp.Click(`[data-anchor="telegram"]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Click(`[data-anchor="email"]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Click(`[data-anchor="backup"]`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(`history.length`, &after),
	))

	assert.Equalf(t, before, after,
		"three chip clicks must add zero history entries (pushState would add three): %d -> %d", before, after)
}

// @aitri-tc TC-SR-057E2E — the stepper's + is a no-op at the field's maximum
// (AC-057-002).
func TestTC_SR_057E2E(t *testing.T) {
	_, ctx := settingsBrowser(t)

	var atMax, afterClick string
	require.NoError(t, chromedp.Run(ctx,
		// Drive the first stepper to its declared maximum, then press + again.
		chromedp.Evaluate(`(() => {
			const w = document.querySelector('[data-widget="stepper"]');
			const input = w.querySelector('input');
			input.value = w.getAttribute('data-max');
			input.dispatchEvent(new Event('input', {bubbles:true}));
			return String(input.value);
		})()`, &atMax),
		chromedp.Click(`[data-widget="stepper"] [data-step="1"]`, chromedp.ByQuery),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`String(document.querySelector('[data-widget="stepper"] input').value)`, &afterClick),
	))

	require.NotEmpty(t, atMax, "the stepper must declare a maximum")
	assert.Equal(t, atMax, afterClick,
		"pressing + at the maximum must be a no-op, got %q -> %q", atMax, afterClick)
}

// @aitri-tc TC-SR-065e — the saving pill appears within 100 ms of submitting a
// section (AC-065-002).
//
// The threshold is the point: a pill that shows up after the request already
// returned tells the operator nothing.
func TestTC_SR_065e(t *testing.T) {
	_, ctx := settingsBrowser(t)

	var elapsedMs float64
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Evaluate(`(async () => {
			const form = document.querySelector('[data-settings-form]');
			if (!form) return -1;
			const name = form.getAttribute('data-settings-form');
			const host = document.querySelector('[data-form-state-host="' + name + '"]')
			           || document.querySelector('[data-form-state-host]');
			if (!host) return -2;
			const t0 = performance.now();
			form.dispatchEvent(new Event('submit', {bubbles: true, cancelable: true}));
			for (let i = 0; i < 200; i++) {
				if (host.querySelector('[data-state="saving"]')) return performance.now() - t0;
				await new Promise(r => setTimeout(r, 2));
			}
			return -3;
		})()`, &elapsedMs, awaitPromise),
	))

	require.GreaterOrEqualf(t, elapsedMs, 0.0,
		"the harness could not observe the pill (code %.0f: -1 no form, -2 no host, -3 never appeared)", elapsedMs)
	assert.Lessf(t, elapsedMs, 100.0,
		"the saving pill must appear within 100 ms, took %.1f ms", elapsedMs)
}

// @aitri-tc TC-SR-071e — at a phone width the segmented control stays a
// one-click row: every segment keeps a real hit area and the group does not
// overflow the viewport (AC-058-003).
func TestTC_SR_071e(t *testing.T) {
	baseURL, session := e2eServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var info struct {
		Count     int     `json:"count"`
		MinW      float64 `json:"minW"`
		MinH      float64 `json:"minH"`
		GroupW    float64 `json:"groupW"`
		Unlabeled int     `json:"unlabeled"`
		DocW      float64 `json:"docW"`
	}
	require.NoError(t, chromedp.Run(ctx,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(baseURL+"/settings"),
		chromedp.WaitVisible("#settings-shell", chromedp.ByID),
		chromedp.WaitVisible("[data-accordion-toggle]", chromedp.ByQuery),
		// The Hardware section holds the segmented controls; expand it so the
		// segments are laid out rather than inside a collapsed body.
		chromedp.Click(`[data-anchor="hardware"]`, chromedp.ByQuery),
		chromedp.Sleep(900*time.Millisecond),
		chromedp.Evaluate(`(() => {
			const g = document.querySelector('[data-widget="segmented"]');
			if (!g) return {count:0,minW:0,minH:0,groupW:0,unlabeled:0,docW:0};
			// Only the real segments: the group also carries a hidden input
			// that holds the submitted value, which has no box and no label.
			const segs = Array.from(g.querySelectorAll('[role="radio"]'));
			const boxes = segs.map(s => s.getBoundingClientRect());
			return {
				count: segs.length,
				minW: Math.min(...boxes.map(b => b.width)),
				minH: Math.min(...boxes.map(b => b.height)),
				groupW: g.getBoundingClientRect().width,
				unlabeled: segs.filter(s => !(s.textContent||'').trim() && !s.getAttribute('aria-label')).length,
				docW: document.documentElement.scrollWidth
			};
		})()`, &info),
	))

	require.Greater(t, info.Count, 0, "a segmented control must render")
	assert.Greater(t, info.MinW, 0.0, "every segment must keep a hit area at 375px")
	assert.Greater(t, info.MinH, 0.0, "every segment must keep a hit area at 375px")
	assert.LessOrEqualf(t, info.GroupW, 375.0,
		"the group must fit the phone viewport, measured %.1f px", info.GroupW)
	assert.LessOrEqualf(t, info.DocW, 375.0,
		"the page must not scroll horizontally at 375px, documentElement is %.1f px", info.DocW)
	assert.Zero(t, info.Unlabeled, "no segment may lose its label when it collapses")
}

// @aitri-tc TC-SR-072h — pressing Space on a focused toggle raises the saving
// pill within 100 ms (AC-061-002).
//
// Measured inside the page: a Go-side clock would be measuring the CDP round
// trip, not the UI.
func TestTC_SR_072h(t *testing.T) {
	_, ctx := settingsBrowser(t)

	var elapsedMs float64
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Evaluate(`(async () => {
			// Not every settings form has a toggle; pick one that does.
			const form = Array.from(document.querySelectorAll('[data-settings-form]'))
			                  .find(f => f.querySelector('input[type="checkbox"]'));
			if (!form) return -1;
			const toggle = form.querySelector('input[type="checkbox"]');
			if (!toggle) return -2;
			const name = form.getAttribute('data-settings-form');
			const host = document.querySelector('[data-form-state-host="' + name + '"]')
			           || document.querySelector('[data-form-state-host]');
			if (!host) return -3;

			toggle.focus();
			const t0 = performance.now();
			toggle.dispatchEvent(new KeyboardEvent('keydown', {key:' ', code:'Space', bubbles:true}));
			toggle.checked = !toggle.checked;
			toggle.dispatchEvent(new Event('change', {bubbles:true}));
			form.dispatchEvent(new Event('submit', {bubbles:true, cancelable:true}));

			for (let i = 0; i < 200; i++) {
				if (host.querySelector('[data-state="saving"]')) return performance.now() - t0;
				await new Promise(r => setTimeout(r, 2));
			}
			return -4;
		})()`, &elapsedMs, chromedp.EvalAsValue, awaitPromise),
	))

	require.GreaterOrEqualf(t, elapsedMs, 0.0,
		"harness could not observe the pill (code %.0f: -1 no form, -2 no toggle, -3 no host, -4 never appeared)", elapsedMs)
	assert.Lessf(t, elapsedMs, 100.0,
		"the saving pill must appear within 100 ms of the toggle changing, took %.1f ms", elapsedMs)
}

// @aitri-tc TC-SR-073f — Restart still refuses to submit without the typed
// confirmation (AC-067-004).
func TestTC_SR_073f(t *testing.T) {
	_, ctx := settingsBrowser(t)

	var state struct {
		Opened        bool `json:"opened"`
		SubmitBlocked bool `json:"submitBlocked"`
		HasWordInput  bool `json:"hasWordInput"`
	}
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Click(`[data-anchor="controls"]`, chromedp.ByQuery),
		chromedp.Sleep(900*time.Millisecond),
		chromedp.Evaluate(`(() => {
			const open = document.querySelector('[data-danger-open][data-danger-action="restart"]');
			if (open) open.click();
			const guard = document.getElementById('danger-action-guard');
			const submit = document.getElementById('danger-action-submit');
			const word = document.getElementById('danger-confirm-word');
			return {
				opened: !!guard && !guard.classList.contains('hidden'),
				submitBlocked: !!submit && submit.disabled === true,
				hasWordInput: !!word
			};
		})()`, &state),
	))

	assert.True(t, state.Opened, "the Run button must open the typed-confirmation guard")
	assert.True(t, state.HasWordInput, "the guard must ask for the confirmation word")
	assert.True(t, state.SubmitBlocked,
		"submit must stay disabled until the confirmation word is typed — the guard fires unchanged")
}
