// Guards for two findings that shipped to production unnoticed because nothing
// checked for them: an unauthenticated directory listing of the asset tree, and
// internal traceability ids inside HTML comments that browsers receive verbatim.
//
// @aitri-trace BL-038 BL-039 BG-080
package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStaticServesFilesButNotListings — real assets keep serving; a bare
// directory 404s instead of rendering an index.
//
// /static/ cannot require auth: the browser fetches CSS and JS before a session
// exists. That makes the listing an unauthenticated inventory of the panel's
// assets, which is why the directory case has to be closed at the filesystem.
func TestStaticServesFilesButNotListings(t *testing.T) {
	srv, _ := setupTestServerWithSession(t)

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)
		return rec
	}

	// A real asset still serves, unauthenticated.
	css := get("/static/css/app.css")
	require.Equal(t, http.StatusOK, css.Code, "the stylesheet must stay reachable without a session")
	assert.NotEmpty(t, css.Body.String())

	// Directories do not.
	for _, dir := range []string{"/static/", "/static/css/", "/static/js/", "/static/js/widgets/"} {
		rec := get(dir)
		assert.Equalf(t, http.StatusNotFound, rec.Code, "%s must not render a listing", dir)
		body := rec.Body.String()
		assert.NotContainsf(t, body, "<a href=", "%s leaked an index of links", dir)
	}
}

// TestNoTraceIDsInServedHTML — no Aitri trace id may sit in an HTML comment.
//
// Scoped to `<!-- -->` on purpose: Go template comments ({{/* … */}}) are
// stripped before rendering and never reach a browser, so banning those would
// cost real maintainer context for no security gain.
func TestNoTraceIDsInServedHTML(t *testing.T) {
	// Assembled so this file does not match its own search.
	traceRe := regexp.MustCompile(`(FR|` + "BG" + `|AC|TC)-[0-9]+`)
	htmlComment := regexp.MustCompile(`(?s)<!--.*?-->`)

	var leaks []string
	err := filepath.WalkDir("../../web/templates", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, c := range htmlComment.FindAllString(string(b), -1) {
			if m := traceRe.FindString(c); m != "" {
				leaks = append(leaks, path+": "+m)
			}
		}
		return nil
	})
	require.NoError(t, err)

	assert.Empty(t, leaks,
		"trace ids in HTML comments reach the browser verbatim; move them to docs/js-trace-map.md: %v", leaks)
}
