// TC-8: Enforce security control - Do not introduce new external endpoints or unauthenticated routes for dashboard stream changes
// Acceptance Criteria: none
// No AC mapped to this TC.
package generated

import "testing"

func TestTc_8_enforce_security_control_do_not_introduce_new_external_endpoints_or_unauthenticated_routes_for_dashboard_stream_changes(t *testing.T) {
	serverSrc := readRepoFile(t, "internal/server/server.go")
	assertContains(t, serverSrc, "mux.HandleFunc(\"GET /health\", s.handleHealth)")
	assertContains(t, serverSrc, "mux.HandleFunc(\"GET /login\", s.handleLoginPage)")
	assertContains(t, serverSrc, "mux.HandleFunc(\"POST /login\", s.handleLogin)")
	assertContains(t, serverSrc, "mux.Handle(\"GET /api/sse/dashboard\", s.requireAuth(http.HandlerFunc(s.handleSSE)))")
	assertNotContains(t, serverSrc, "mux.HandleFunc(\"GET /api/sse/dashboard\"")
}
