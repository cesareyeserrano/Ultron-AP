// TC-2: Handle edge behavior - When live data is delayed/unavailable, the UI must still present clear placeholders and statuses without layout jumps or unreadable states
// Acceptance Criteria: none
// No AC mapped to this TC.
package generated

import (
	"os"
	"strings"
	"testing"
)

func TestTc2HandleEdgeBehaviorWhenLiveDataIsDelayedOrUnavailable(t *testing.T) {
	dashboardPath := repoFile(t, "web", "templates", "dashboard.html")
	dockerPath := repoFile(t, "web", "templates", "partials", "sse-docker.html")
	systemdPath := repoFile(t, "web", "templates", "partials", "sse-systemd.html")

	dashboard, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("read %s: %v", dashboardPath, err)
	}
	docker, err := os.ReadFile(dockerPath)
	if err != nil {
		t.Fatalf("read %s: %v", dockerPath, err)
	}
	systemd, err := os.ReadFile(systemdPath)
	if err != nil {
		t.Fatalf("read %s: %v", systemdPath, err)
	}

	if !strings.Contains(string(dashboard), "Collecting data...") {
		t.Fatal("expected collecting-data placeholder for delayed metrics/charts")
	}
	if !strings.Contains(string(docker), "Docker not available") {
		t.Fatal("expected deterministic docker unavailable state")
	}
	if !strings.Contains(string(systemd), "Systemd not available") {
		t.Fatal("expected deterministic systemd unavailable state")
	}
}
