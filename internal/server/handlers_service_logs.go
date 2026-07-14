package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/logfilter"
	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

// serviceLogLines is the tail depth the drawer fetches (FR-081 / AC-081-001).
const serviceLogLines = 100

// serviceLogsData is what partials/service-logs.html renders.
type serviceLogsData struct {
	Name    string
	Content string // already redacted by the helper's logfilter; escaped by html/template
	Error   string // set when the helper is unavailable — an empty panel would be ambiguous
}

// handleServiceLogs handles GET /api/services/{name}/logs (FR-081).
//
// It adds NO new privileged capability: the tail is fetched with the SAME
// allow-listed "logs" action the /logs page already uses, over the same Unix
// socket, and the root helper re-validates the unit name against serviceNameRe
// before it invokes journalctl (NFR-088). The panel validates too — defence in
// depth, and it lets an obviously bad name fail fast with a 400.
func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	source := "service:" + name

	// Validate against the SAME unit-name authority the start/stop/restart path
	// uses (systemd.IsValidServiceName). isValidLogSource only checks the
	// "service:" prefix, so it would happily pass "service:--version" through.
	if name == "" || !systemd.IsValidServiceName(name) || !isValidLogSource(source) {
		http.Error(w, "Invalid service name", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	out, err := s.privileged.SystemLogs(ctx, source, serviceLogLines)
	if err != nil {
		log.Printf("services: failed to fetch logs for %s: %v", source, err)

		// A 5xx would leave the drawer empty, and an empty drawer reads as
		// "this unit has no logs" — a different fact. Render the failure so the
		// admin can tell the two apart (AC-081-004).
		msg := "Could not read logs — the privileged helper is unavailable."
		if !errors.Is(err, privileged.ErrUnavailable) {
			msg = "Could not read logs for this unit."
		}
		s.writeServiceLogs(w, serviceLogsData{Name: name, Error: msg})
		return
	}

	// The helper already redacts (PolicyJournal) before the bytes cross the
	// socket. Re-running the filter here is defence in depth: it means a
	// future caller that reaches the drawer by another path — or a helper
	// running an older build — still cannot render a leaked token into the
	// browser (AC-081-003). html/template escapes the result.
	safe := logfilter.Filter([]byte(out), logfilter.PolicyJournal, logfilter.MaxBytes)

	s.writeServiceLogs(w, serviceLogsData{Name: name, Content: strings.TrimRight(string(safe), "\n")})
}

func (s *Server) writeServiceLogs(w http.ResponseWriter, data serviceLogsData) {
	html := s.renderPartial("partials/service-logs.html", data)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
