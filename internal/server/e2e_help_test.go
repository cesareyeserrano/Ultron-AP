//go:build e2e

// Browser tests for the seven help-page acceptance criteria that no HTTP-level
// test can observe: where things land on screen, what a real layout does with
// JavaScript switched off, and whether a CSS animation actually runs.
//
// These are the criteria that kept help-page at 4/5 (BL-042). They were
// declared as e2e in Phase 3 and had no implementation, so the AC-coverage gate
// blocked Phase 5.
//
// Run: go test -tags e2e ./internal/server/ -run TestTC_HP
//
// @aitri-trace FR-050 FR-051 FR-054 FR-056 BL-042
package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpEntry is a glossary entry that exists in the bundled data, used as the
// concrete subject of the layout assertions.
const helpEntry = "entry-cpu-percent"

// @aitri-tc TC-HP-050e — at a 375 px viewport the plain body renders ABOVE the
// technical one, and the two overlap horizontally, i.e. they stack rather than
// sit side by side (AC-050-002).
//
// The desktop layout puts them in two columns; mobile must stack them with the
// plain voice first. That is a fact about the rendered box positions, which is
// exactly what no server-side test can see.
func TestTC_HP_050e(t *testing.T) {
	baseURL, session := e2eHelpServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var plain, tech box
	require.NoError(t, chromedp.Run(ctx,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(baseURL+"/help"),
		chromedp.WaitVisible("#"+helpEntry, chromedp.ByID),
		boundingBox("#"+helpEntry+` [data-voice="plain"]`, &plain),
		boundingBox("#"+helpEntry+` [data-voice="technical"]`, &tech),
	))

	require.NotZero(t, plain.H, "the plain body must be laid out")
	require.NotZero(t, tech.H, "the technical body must be laid out")

	assert.Less(t, plain.Y, tech.Y,
		"at 375px the plain voice must sit ABOVE the technical one (plain y=%.1f, technical y=%.1f)", plain.Y, tech.Y)

	// Stacked, not columns: their horizontal extents must overlap almost fully.
	overlap := minF(plain.X+plain.W, tech.X+tech.W) - maxF(plain.X, tech.X)
	assert.Greaterf(t, overlap, 0.8*minF(plain.W, tech.W),
		"stacked bodies must share the column width; overlap=%.1f plainW=%.1f techW=%.1f", overlap, plain.W, tech.W)
}

// @aitri-tc TC-HP-050f — with JavaScript disabled both voices stay visible: the
// two-voice layout is pure CSS and nothing script-driven hides either
// (AC-050-003).
func TestTC_HP_050f(t *testing.T) {
	baseURL, session := e2eHelpServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var plainW, techW float64
	require.NoError(t, chromedp.Run(ctx,
		disableJavaScript(),
		chromedp.Navigate(baseURL+"/help"),
		chromedp.Evaluate(`document.querySelector('#`+helpEntry+` [data-voice="plain"]').offsetWidth`, &plainW),
		chromedp.Evaluate(`document.querySelector('#`+helpEntry+` [data-voice="technical"]').offsetWidth`, &techW),
	))

	assert.Greater(t, plainW, 0.0, "the plain voice must remain visible without JS")
	assert.Greater(t, techW, 0.0, "the technical voice must remain visible without JS")
}

// @aitri-tc TC-HP-051e — arriving on an entry's hash paints a highlight that
// then fades, driven by CSS alone (AC-051-002).
//
// The assertion is on the COMPUTED background colour at two moments: opaque
// shortly after arrival, transparent once the 1.5s animation has finished. A
// server-side test could only confirm that a stylesheet mentions an animation,
// which proves nothing about whether it runs.
func TestTC_HP_051e(t *testing.T) {
	baseURL, session := e2eHelpServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var early, late string
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/help#"+helpEntry),
		chromedp.WaitVisible("#"+helpEntry, chromedp.ByID),
		chromedp.Sleep(100*time.Millisecond),
		computedStyle("#"+helpEntry, "background-color", &early),
		chromedp.Sleep(2*time.Second), // the animation is 1.5s
		computedStyle("#"+helpEntry, "background-color", &late),
	))

	assert.NotEqual(t, "rgba(0, 0, 0, 0)", early,
		"the :target highlight must be painted shortly after arrival, got %q", early)
	assert.Equal(t, "rgba(0, 0, 0, 0)", late,
		"the highlight must have faded to transparent after the animation, got %q", late)
}

// @aitri-tc TC-HP-051f — a hash matching no element is ignored gracefully: the
// page renders at the top and the browser console reports nothing
// (AC-051-003).
func TestTC_HP_051f(t *testing.T) {
	baseURL, session := e2eHelpServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	pageErrs := collectPageErrors(ctx)

	var scrollY float64
	var entriesPresent bool
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/help#no-such-entry-anywhere"),
		chromedp.WaitVisible("#help-entries", chromedp.ByID),
		chromedp.Evaluate(`window.scrollY`, &scrollY),
		chromedp.Evaluate(`!!document.getElementById('help-entries')`, &entriesPresent),
	))

	assert.True(t, entriesPresent, "the page must still render its content")
	assert.Zero(t, scrollY, "an unmatched hash must leave the page at the top, got scrollY=%.0f", scrollY)
	assert.Empty(t, pageErrs(), "an unmatched hash must raise no console error")
}

// @aitri-tc TC-HP-054e — a filter term that matches nothing in a category hides
// that whole category, so no empty header is left behind (AC-054-002).
func TestTC_HP_054e(t *testing.T) {
	baseURL, session := e2eHelpServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var hiddenCount, visibleCount int
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/help"),
		chromedp.WaitVisible("#help-filter", chromedp.ByID),
		chromedp.SendKeys("#help-filter", "cpu", chromedp.ByID),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('section[data-category]'))
		                     .filter(s => s.offsetParent === null).length`, &hiddenCount),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('section[data-category]'))
		                     .filter(s => s.offsetParent !== null).length`, &visibleCount),
	))

	assert.Greater(t, hiddenCount, 0,
		"at least one category has no 'cpu' entry and must be hidden entirely, not left as an empty header")
	assert.Greater(t, visibleCount, 0,
		"the category that does match must stay visible")
}

// @aitri-tc TC-HP-054f — with JavaScript disabled every entry is visible and
// the page is fully usable; the filter input is simply inert (AC-054-003).
//
// This is the graceful-degradation promise: the glossary is server-rendered
// content, not a JS app.
func TestTC_HP_054f(t *testing.T) {
	baseURL, session := e2eHelpServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var total, visible int
	var filterPresent bool
	require.NoError(t, chromedp.Run(ctx,
		disableJavaScript(),
		chromedp.Navigate(baseURL+"/help"),
		chromedp.Evaluate(`document.querySelectorAll('article.help-entry').length`, &total),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('article.help-entry'))
		                     .filter(e => e.offsetWidth > 0).length`, &visible),
		chromedp.Evaluate(`!!document.getElementById('help-filter')`, &filterPresent),
	))

	require.Greater(t, total, 0, "the glossary must render server-side")
	assert.Equal(t, total, visible, "every entry must be visible with JS off (%d of %d)", visible, total)
	assert.True(t, filterPresent, "the filter input is still in the DOM, just inert")
}

// @aitri-tc TC-HP-056f — the Help nav item navigates to /help in the SAME tab
// (AC-056-002).
func TestTC_HP_056f(t *testing.T) {
	baseURL, session := e2eHelpServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var target string
	var finalURL string
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/"),
		chromedp.WaitVisible(`a[href="/help"]`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('a[href="/help"]').getAttribute('target') || ""`, &target),
		chromedp.Click(`a[href="/help"]`, chromedp.ByQuery),
		chromedp.WaitVisible("#help-page", chromedp.ByID),
		chromedp.Location(&finalURL),
	))

	assert.Empty(t, target, `the Help link must not carry target="_blank"`)
	assert.True(t, strings.HasSuffix(finalURL, "/help"),
		"clicking Help must land on /help, got %q", finalURL)
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

var _ = context.Background
