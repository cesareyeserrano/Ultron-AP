//go:build e2e

// Browser-driven end-to-end harness.
//
// Behind the `e2e` build tag on purpose. These tests need a real Chrome binary,
// which a plain `go test ./...` on a machine without one must not fail over —
// the tag keeps the default suite hermetic and makes running them a deliberate
// act: `go test -tags e2e ./internal/server/ -run TestE2E`.
//
// Why this exists: ~20 acceptance criteria across help-page and settings-revamp
// describe things no HTTP-level test can observe — a pill appearing within
// 100ms, an accordion already expanded when a scroll completes, a control
// collapsing at a narrow viewport, the absence of a console exception. They sat
// uncovered as BL-042 and kept both features stuck at 4/5. Asserting the
// server's HTML instead would have been a weaker proxy dressed up as coverage.
//
// @aitri-trace BL-042
package server

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/help"
)

// browserTimeout bounds a whole browser test. Generous: a cold Chrome start on
// a loaded CI runner is slow, and a too-tight bound reads as a product failure
// when it is really a scheduling one.
const browserTimeout = 45 * time.Second

// desktopWidth/desktopHeight put the sidebar on-canvas. See e2eBrowser.
const (
	desktopWidth  = 1280
	desktopHeight = 900
)

// chromePath returns an explicit Chrome binary when the platform hides it
// somewhere chromedp's own lookup does not check. On macOS the app bundle is
// not on PATH; on Linux CI, chromedp finds chromium itself.
func chromePath() string {
	if p := os.Getenv("ULTRON_E2E_CHROME"); p != "" {
		return p
	}
	if runtime.GOOS == "darwin" {
		const mac = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(mac); err == nil {
			return mac
		}
	}
	return ""
}

// e2eServer boots the real handler on a real listening socket with a temp
// database and an authenticated session already stored.
//
// It returns the base URL and the session, so a test can plant the cookie and
// browse as a logged-in operator. The server is the production one — New() with
// the same wiring main.go uses — not a stub: a harness that tests a simplified
// server proves nothing about the shipped one.
func e2eServer(t *testing.T) (string, *database.Session) {
	t.Helper()

	cfg := &config.Config{
		Port:             8080,
		DBPath:           filepath.Join(t.TempDir(), "e2e.db"),
		LogLevel:         "info",
		AdminUser:        "admin",
		AdminPass:        "secret",
		SessionTTL:       24 * time.Hour,
		NetRetentionDays: 30,
		NetInterval:      5 * time.Second,
	}

	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.CreateUser("admin", "$2a$10$dummy"))

	session := &database.Session{
		ID:        "e2e-session-token",
		UserID:    1,
		CSRFToken: "e2e-csrf-token",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.CreateSession(session))

	srv := New(cfg, db, nil, nil, nil, nil, nil)
	ts := httptest.NewServer(srv.httpServer.Handler)
	t.Cleanup(ts.Close)

	return ts.URL, session
}

// e2eBrowser starts headless Chrome and returns a context already carrying the
// session cookie for baseURL, so the first navigation lands authenticated.
func e2eBrowser(t *testing.T, baseURL string, session *database.Session) context.Context {
	t.Helper()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox, // required in most CI containers
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true), // small /dev/shm in containers
	)
	if p := chromePath(); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(cancelAlloc)

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	t.Cleanup(cancelCtx)

	ctx, cancelTimeout := context.WithTimeout(ctx, browserTimeout)
	t.Cleanup(cancelTimeout)

	// A desktop viewport by default. Headless Chrome starts at 756x413, a
	// tablet-ish size at which this layout puts the sidebar off-canvas
	// (measured x = -248): a click on a nav item then lands outside the
	// viewport and silently does nothing, which reads as a broken link rather
	// than a wrong test. A test that wants the mobile layout emulates it
	// explicitly, as the 375px stacking test does.
	require.NoError(t, chromedp.Run(ctx,
		chromedp.EmulateViewport(desktopWidth, desktopHeight),
		chromedp.Navigate(baseURL+"/login"),
		setSessionCookie(baseURL, session.ID),
	), "browser start-up failed — is Chrome installed? set ULTRON_E2E_CHROME to override")

	return ctx
}

// setSessionCookie installs the panel's session cookie for the test server's
// origin using the CDP network domain.
func setSessionCookie(baseURL, value string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		u, err := url.Parse(baseURL)
		if err != nil {
			return err
		}
		expiry := cdp.TimeSinceEpoch(time.Now().Add(24 * time.Hour))
		return network.SetCookie("session", value).
			WithDomain(u.Hostname()).
			WithPath("/").
			WithHTTPOnly(true).
			WithExpires(&expiry).
			Do(ctx)
	}
}

// TestE2E_Harness_Smoke proves the whole chain works — Chrome starts, the real
// server serves, the planted cookie authenticates — before any behavioural test
// depends on it. If this fails, nothing below it is meaningful.
func TestE2E_Harness_Smoke(t *testing.T) {
	baseURL, session := e2eServer(t)
	ctx := e2eBrowser(t, baseURL, session)

	var title, bodyText string
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/"),
		chromedp.Title(&title),
		chromedp.Text("body", &bodyText, chromedp.ByQuery),
	))

	assert.NotEmpty(t, title, "the dashboard must render a title")
	assert.NotContains(t, bodyText, "Sign in",
		"the planted session cookie must land us authenticated, not on the login page")
}

// e2eHelpServer is e2eServer with the help service attached — /help answers 503
// without it.
func e2eHelpServer(t *testing.T) (string, *database.Session) {
	t.Helper()

	cfg := &config.Config{
		Port: 8080, DBPath: filepath.Join(t.TempDir(), "e2e.db"), LogLevel: "info",
		AdminUser: "admin", AdminPass: "secret", SessionTTL: 24 * time.Hour,
		NetRetentionDays: 30, NetInterval: 5 * time.Second,
	}
	db, err := database.New(cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.CreateUser("admin", "$2a$10$dummy"))

	session := &database.Session{
		ID: "e2e-session-token", UserID: 1, CSRFToken: "e2e-csrf-token",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.CreateSession(session))

	svc, err := help.New(func(string, ...interface{}) {})
	require.NoError(t, err)

	srv := New(cfg, db, nil, nil, nil, nil, nil)
	srv.SetHelp(svc)

	ts := httptest.NewServer(srv.httpServer.Handler)
	t.Cleanup(ts.Close)
	return ts.URL, session
}

// box is an element's viewport-relative bounding rectangle, read from the live
// layout. This is the whole reason a browser is involved: no HTTP-level test can
// tell you where something ended up on screen.
type box struct {
	X, Y, W, H float64
}

// boundingBox reads getBoundingClientRect for a selector.
func boundingBox(sel string, out *box) chromedp.Action {
	return chromedp.Evaluate(
		`(() => { const e = document.querySelector(`+"`"+sel+"`"+`);
                  if (!e) return null;
                  const r = e.getBoundingClientRect();
                  return {X: r.x, Y: r.y, W: r.width, H: r.height}; })()`, out)
}

// disableJavaScript turns script execution off for the rest of the session.
// Used by the graceful-degradation tests: the promise is that the glossary is
// server-rendered content, and the only honest way to check that is to take the
// scripts away and look.
func disableJavaScript() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return emulation.SetScriptExecutionDisabled(true).Do(ctx)
	}
}

// computedStyle reads one resolved CSS property of an element — the value the
// browser actually painted with, after the cascade and any running animation.
func computedStyle(sel, prop string, out *string) chromedp.Action {
	return chromedp.Evaluate(
		`getComputedStyle(document.querySelector(`+"`"+sel+"`"+`)).getPropertyValue('`+prop+`').trim()`,
		out)
}

// collectPageErrors starts recording uncaught JavaScript exceptions and returns
// a snapshot function. A page that "works" while throwing in the console is not
// working, and this is the only level at which that is visible.
func collectPageErrors(ctx context.Context) func() []string {
	var mu sync.Mutex
	var errs []string
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if e, ok := ev.(*cdpruntime.EventExceptionThrown); ok {
			mu.Lock()
			errs = append(errs, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), errs...)
	}
}

// awaitPromise makes chromedp.Evaluate wait for a returned Promise instead of
// handing back the Promise object. Needed by any assertion that has to observe
// something over time inside the page — a pill that must appear within 100 ms
// cannot be measured from Go, because the round trip would dominate the
// measurement.
func awaitPromise(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams {
	return p.WithAwaitPromise(true)
}
