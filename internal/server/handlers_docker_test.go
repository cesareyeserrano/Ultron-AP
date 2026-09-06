// Docker page handler tests.
//
// The panel is read-only over Docker since the C2 hardening: the former
// start/stop/restart handler tests are gone with the handlers themselves, and
// what replaces them are TC-DVH-060h/061f/062f in docker_readonly_test.go,
// which assert those routes are ABSENT.
//
// @aitri-trace FR-088 FR-091 FR-094
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	dockerpkg "github.com/cesareyeserrano/ultron-ap/internal/docker"
)

// fakeDockerSource stands in for the privileged helper. It satisfies the
// unexported containerSource interface in internal/docker structurally — the
// method set is exported, so an outside package can implement it without
// naming the type.
type fakeDockerSource struct {
	list      []dockerpkg.ContainerInfo
	listErr   error
	detail    *dockerpkg.ContainerDetail
	detailErr error
	logs      string
	logsErr   error
}

func (f *fakeDockerSource) List(_ context.Context) ([]dockerpkg.ContainerInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakeDockerSource) Inspect(_ context.Context, _ string) (*dockerpkg.ContainerDetail, error) {
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	return f.detail, nil
}

func (f *fakeDockerSource) Logs(_ context.Context, _ string, _ int) (string, error) {
	if f.logsErr != nil {
		return "", f.logsErr
	}
	return f.logs, nil
}

func sampleDockerContainers() []dockerpkg.ContainerInfo {
	return []dockerpkg.ContainerInfo{
		{ID: "abc123def456789012345678", Name: "web-app", Image: "nginx:latest",
			State: "running", Status: "Up 2 hours", Health: dockerpkg.HealthRunning},
		{ID: "def456ghi789012345678901", Name: "db", Image: "postgres:16",
			State: "exited", Status: "Exited (0) 1 hour ago", Health: dockerpkg.HealthStopped},
	}
}

// setupDockerTestServer wires a server whose Docker monitor reads from src.
func setupDockerTestServer(t *testing.T, src *fakeDockerSource) (*Server, *database.Session) {
	t.Helper()

	cfg := &config.Config{SessionTTL: 24 * time.Hour}
	db, err := database.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	session := &database.Session{
		ID:        "docker-test-session",
		UserID:    1,
		CSRFToken: "test-csrf",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.CreateSession(session))

	dockerMon := dockerpkg.NewMonitorWithSource(src)
	ctx, cancel := context.WithCancel(context.Background())
	dockerMon.Start(ctx)
	// Wait for the first refresh to populate the cache.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if src.listErr != nil || len(dockerMon.Containers()) == len(src.list) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Cleanup(func() { cancel(); dockerMon.Stop() })

	srv := New(cfg, db, nil, nil, dockerMon, nil, nil)
	return srv, session
}

func getDockerPage(t *testing.T, srv *Server, session *database.Session) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/docker", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: session.ID})
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func TestDockerPage_RendersContainers(t *testing.T) {
	srv, session := setupDockerTestServer(t, &fakeDockerSource{list: sampleDockerContainers()})
	rec := getDockerPage(t, srv, session)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "web-app")
	assert.Contains(t, body, "nginx:latest")
	assert.Contains(t, body, "db")
}

func TestDockerPage_RequiresAuth(t *testing.T) {
	srv, _ := setupDockerTestServer(t, &fakeDockerSource{list: sampleDockerContainers()})

	req := httptest.NewRequest(http.MethodGet, "/docker", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusOK, rec.Code, "an unauthenticated request must not see the page")
}
