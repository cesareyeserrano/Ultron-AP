// The Docker screen is read-only since the C2 hardening. These tests assert
// two things the old suite could not: that "cannot read" and "nothing to show"
// are rendered as DIFFERENT states, and that the container control routes are
// absent rather than merely unlinked.
//
// @aitri-trace FR-091 FR-094 NFR-098 NFR-099 US-091 US-094
package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
	dockerpkg "github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
)

// unavailableText is the phrase the error state renders. Kept as one constant
// so a copy change cannot silently make these assertions vacuous.
const unavailableText = "Container data unavailable"

// emptyText is the phrase the empty state renders — the message that used to
// appear (wrongly) when the helper was down.
const emptyText = "No containers found"

func downSource() *fakeDockerSource {
	return &fakeDockerSource{listErr: privileged.ErrUnavailable}
}

// @aitri-tc TC-DVH-030h — with the helper up the page lists containers and
// shows no unavailability notice (AC-091-003).
func TestTC_DVH_030h(t *testing.T) {
	srv, session := setupDockerTestServer(t, &fakeDockerSource{list: sampleDockerContainers()})
	rec := getDockerPage(t, srv, session)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "web-app")
	assert.Contains(t, body, "db")
	assert.NotContains(t, body, unavailableText, "a healthy read must not show the error state")
}

// @aitri-tc TC-DVH-031f — with the helper down the page says so (AC-091-001).
func TestTC_DVH_031f(t *testing.T) {
	srv, session := setupDockerTestServer(t, downSource())
	rec := getDockerPage(t, srv, session)

	require.Equal(t, http.StatusOK, rec.Code, "a helper failure must not become an HTTP error")
	assert.Contains(t, rec.Body.String(), unavailableText)
}

// @aitri-tc TC-DVH-032f — with the helper down the page must NOT claim there
// are no containers. This is the exact defect the feature exists to fix: the
// old page rendered the empty state and the unavailability chip together, and
// the empty state was the dominant message (AC-091-002).
func TestTC_DVH_032f(t *testing.T) {
	srv, session := setupDockerTestServer(t, downSource())
	body := getDockerPage(t, srv, session).Body.String()

	assert.NotContains(t, body, emptyText,
		"unavailable and empty are mutually exclusive states")
	assert.Contains(t, body, unavailableText)
}

// @aitri-tc TC-DVH-033e — a confirmed read of zero containers shows the empty
// state, not the error state (AC-091-002).
func TestTC_DVH_033e(t *testing.T) {
	srv, session := setupDockerTestServer(t, &fakeDockerSource{list: []dockerpkg.ContainerInfo{}})
	body := getDockerPage(t, srv, session).Body.String()

	assert.Contains(t, body, emptyText, "a confirmed empty read shows the empty state")
	assert.NotContains(t, body, unavailableText, "a successful read is not an error")
}

// @aitri-tc TC-DVH-034e — the rest of the panel keeps serving while Docker
// cannot be read (AC-091-004).
func TestTC_DVH_034e(t *testing.T) {
	srv, session := setupDockerTestServer(t, downSource())

	for _, path := range []string{"/", "/alerts", "/network"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code, "%s must keep serving", path)
		assert.NotContains(t, rec.Body.String(), unavailableText,
			"%s must not inherit the Docker error state", path)
	}
}

// @aitri-tc TC-DVH-035h — the error notice meets WCAG AA against its own
// background, computed from the real tokens in input.css (NFR-100).
func TestTC_DVH_035h(t *testing.T) {
	tokens := loadCSSTokens(t)
	require.Contains(t, tokens, "error-text")
	require.Contains(t, tokens, "error-bg")

	ratio := contrastRatio(tokens["error-text"], tokens["error-bg"])
	assert.GreaterOrEqualf(t, ratio, 4.5,
		"--color-error-text on --color-error-bg is %.2f:1, below WCAG AA body text", ratio)
}

// @aitri-tc TC-DVH-036h — the row keeps its detail affordance (AC-091-003).
func TestTC_DVH_036h(t *testing.T) {
	srv, session := setupDockerTestServer(t, &fakeDockerSource{list: sampleDockerContainers()})
	body := getDockerPage(t, srv, session).Body.String()

	assert.Contains(t, body, `hx-get="/api/docker/abc123def456789012345678"`,
		"the row must still expand into its detail")
	assert.Contains(t, body, `hx-target="#detail-abc123def456"`)
}

// @aitri-tc TC-DVH-060h — the page offers no container action at all
// (AC-094-001).
func TestTC_DVH_060h(t *testing.T) {
	srv, session := setupDockerTestServer(t, &fakeDockerSource{list: sampleDockerContainers()})
	body := getDockerPage(t, srv, session).Body.String()

	// One of the sample containers is stopped, so the old markup would have
	// rendered an enabled Start control here.
	for _, action := range []string{"start", "stop", "restart"} {
		assert.NotContains(t, body, "/api/docker/abc123def456789012345678/"+action,
			"no %s route may be referenced from the page", action)
		assert.NotContains(t, body, "/api/docker/def456ghi789012345678901/"+action,
			"no %s route may be referenced from the page", action)
	}
	assert.False(t, strings.Contains(body, `hx-confirm="Stop container`),
		"the stop confirmation must be gone with the control")
}

// @aitri-tc TC-DVH-061f — POST to the old start route 404s. It is absent from
// the mux rather than registered-and-refused: a 403 would confirm to a prober
// that the route still exists (AC-094-002).
func TestTC_DVH_061f(t *testing.T) {
	srv, session := setupDockerTestServer(t, &fakeDockerSource{list: sampleDockerContainers()})

	rec := postForm(t, srv, session, "/api/docker/abc123def456789012345678/start")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// @aitri-tc TC-DVH-062f — the container control code is GONE from the tree,
// not merely unrouted (AC-094-004).
//
// The 404s below prove the routes are unreachable; the file checks prove the
// code that served them no longer exists. Both matter: dead handlers left in
// place are an invitation to re-register them, which would hand the web app
// write access to Docker again — exactly the C2 finding.
func TestTC_DVH_062f(t *testing.T) {
	_, err := os.Stat("../docker/controls.go")
	assert.Truef(t, os.IsNotExist(err),
		"internal/docker/controls.go must be deleted, not left unreferenced (stat err=%v)", err)

	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, readErr := os.ReadFile(e.Name())
		require.NoError(t, readErr)
		for _, h := range []string{"func (s *Server) handleDockerStart", "func (s *Server) handleDockerStop", "func (s *Server) handleDockerRestart"} {
			assert.NotContainsf(t, string(b), h, "%s still defines %s", e.Name(), h)
		}
	}

	srv, session := setupDockerTestServer(t, &fakeDockerSource{list: sampleDockerContainers()})
	for _, action := range []string{"stop", "restart"} {
		rec := postForm(t, srv, session, "/api/docker/abc123def456789012345678/"+action)
		assert.Equalf(t, http.StatusNotFound, rec.Code, "%s must 404", action)
	}
}

// postForm issues an authenticated, CSRF-bearing POST so a 404 cannot be
// mistaken for an auth or CSRF rejection.
func postForm(t *testing.T, srv *Server, session *database.Session, path string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

// --- Regression: what the Docker change must NOT break ---

// @aitri-tc TC-DVH-110h — the services page keeps all three systemd controls.
// Retiring container controls must not touch service controls; they are
// different capabilities that happened to share a UI component (NFR-098).
func TestTC_DVH_110h(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, action := range []string{"start", "stop", "restart"} {
		assert.Containsf(t, body, "/"+action+`"`, "systemd rows must keep the %s control", action)
	}
}

// @aitri-tc TC-DVH-111f — an option-shaped unit name is still refused
// (NFR-098).
func TestTC_DVH_111f(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/"+url.PathEscape("-M evil")+"/restart",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	// The handler answers 200 by design so HTMX can swap an error banner in
	// place of raw error text (renderServicesResult). The security property is
	// therefore NOT the status code — it is that no systemctl ran.
	assert.False(t, runner.ranAction(),
		"an option-shaped unit name must be refused before any systemctl runs; invocations=%v",
		runner.invocations)
}

// @aitri-tc TC-DVH-112e — a service action is still written to the audit
// trail (NFR-098).
func TestTC_DVH_112e(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	form := url.Values{"csrf_token": {session.CSRFToken}}
	req := httptest.NewRequest(http.MethodPost, "/api/services/nginx.service/restart",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusNotFound, rec.Code, "the service action route must still exist")

	actions, err := srv.db.ListActionLogs(20)
	require.NoError(t, err)
	found := false
	for _, a := range actions {
		if strings.Contains(a.Target, "nginx") && a.Action == "restart" {
			found = true
		}
	}
	assert.True(t, found, "the service action must appear in the audit trail: %+v", actions)
}

// @aitri-tc TC-DVH-120h — the dashboard still renders while Docker cannot be
// read (NFR-099).
func TestTC_DVH_120h(t *testing.T) {
	srv, session := setupDockerTestServer(t, downSource())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), unavailableText,
		"the Docker error state belongs to the Docker section only")
}

// @aitri-tc TC-DVH-121f — the Docker failure does not propagate to other
// routes (NFR-099).
func TestTC_DVH_121f(t *testing.T) {
	srv, session := setupDockerTestServer(t, downSource())

	// /services is exercised under its own fixture in TC-DVH-110h: this
	// server has no systemd monitor wired, so requesting it here would test
	// the fixture, not the isolation.
	for _, path := range []string{"/alerts", "/network"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)

		assert.Equalf(t, http.StatusOK, rec.Code, "%s must keep serving", path)
		assert.NotContainsf(t, rec.Body.String(), unavailableText,
			"%s must not show the Docker error state", path)
	}
}

// @aitri-tc TC-DVH-122e — the dashboard's Docker tile distinguishes the two
// states as well. The tile already checked availability before emptiness; the
// page partial did not, and that mismatch is what this feature closes
// (NFR-099).
func TestTC_DVH_122e(t *testing.T) {
	srvDown, sessionDown := setupDockerTestServer(t, downSource())
	tileDown := srvDown.renderPartial("partials/sse-docker.html", map[string]any{
		"DockerAvail": srvDown.docker.Available(),
		"Containers":  srvDown.docker.Containers(),
	})
	_ = sessionDown

	srvEmpty, _ := setupDockerTestServer(t, &fakeDockerSource{list: []dockerpkg.ContainerInfo{}})
	tileEmpty := srvEmpty.renderPartial("partials/sse-docker.html", map[string]any{
		"DockerAvail": srvEmpty.docker.Available(),
		"Containers":  srvEmpty.docker.Containers(),
	})

	assert.Contains(t, tileDown, "Docker not available")
	assert.NotContains(t, tileDown, emptyText, "unavailable must not read as empty")

	assert.Contains(t, tileEmpty, emptyText)
	assert.NotContains(t, tileEmpty, "Docker not available", "empty must not read as unavailable")
}

// @aitri-tc TC-DVH-063e — retiring the container controls left the systemd
// controls untouched. They shared a UI component but are different
// capabilities: only Docker sits behind the privilege boundary this feature
// draws (AC-094-003).
func TestTC_DVH_063e(t *testing.T) {
	runner := &mockCommandRunner{output: listUnitsOutput()}
	srv, session := setupServiceTestServer(t, runner)

	req := httptest.NewRequest(http.MethodGet, "/services", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, action := range []string{"start", "stop", "restart"} {
		assert.Regexpf(t, `hx-post="/api/services/[^"]+/`+action+`"`, body,
			"the systemd %s control must survive the Docker change", action)
	}
}
