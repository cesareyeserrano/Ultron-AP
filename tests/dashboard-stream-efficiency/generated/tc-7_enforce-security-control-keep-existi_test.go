// TC-7: Enforce security control - Keep existing privilege separation: web process remains unprivileged and does not execute host-privileged commands
// Acceptance Criteria: none
// No AC mapped to this TC.
package generated

import "testing"

func TestTc_7_enforce_security_control_keep_existing_privilege_separation_web_process_remains_unprivileged_and_does_not_execute_host_privileged_commands(t *testing.T) {
	serverSrc := readRepoFile(t, "internal/server/server.go")
	assertContains(t, serverSrc, "privileged.NewClient(cfg.HelperSocket, cfg.HelperTimeout)")
	assertContains(t, serverSrc, "mux.Handle(\"GET /api/sse/dashboard\", s.requireAuth(http.HandlerFunc(s.handleSSE)))")
	assertNotContains(t, serverSrc, "sudo ")
}
