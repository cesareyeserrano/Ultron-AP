// TC-7: Enforce security control - Keep existing privilege separation: web process remains unprivileged and does not execute host-privileged commands
// Acceptance Criteria: none
// No AC mapped to this TC.
package generated

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTc_7_enforce_security_control_keep_existing_privilege_separation_web_process_remains_unprivileged_and_does_not_execute_host_privileged_commands(t *testing.T) {
	serverSrc := readRepoFile(t, "internal/server/server.go")
	assertContains(t, serverSrc, "privileged.NewClient(cfg.HelperSocket, cfg.HelperTimeout)")
	assertContains(t, serverSrc, "mux.Handle(\"GET /api/sse/dashboard\", s.requireAuth(http.HandlerFunc(s.handleSSE)))")
	assertNotContains(t, serverSrc, "sudo ")
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func assertContains(t *testing.T, src, needle string) {
	t.Helper()
	if !strings.Contains(src, needle) {
		t.Fatalf("expected to find %q", needle)
	}
}

func assertNotContains(t *testing.T, src, needle string) {
	t.Helper()
	if strings.Contains(src, needle) {
		t.Fatalf("expected to not find %q", needle)
	}
}
