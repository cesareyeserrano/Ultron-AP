package server

import (
	"context"
	"fmt"
	"html"
	"net/http"

	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

type servicesPageData struct {
	Services     []systemd.ServiceInfo
	ProcessStats map[string]ProcessConsumer
	Available    bool
}

func (s *Server) handleServicesPage(w http.ResponseWriter, r *http.Request) {
	data := servicesPageData{
		Services:     s.systemd.Services(),
		ProcessStats: collectProcessUsage(),
		Available:    s.systemd.Available(),
	}
	s.render(w, r, "services.html", "Services", "services", data)
}

func (s *Server) handleServiceStart(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, s.systemd.StartService)
}

func (s *Server) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, s.systemd.StopService)
}

func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, s.systemd.RestartService)
}

// serviceAction is the shared body for the service start/stop/restart endpoints
// (D3), which differed only in the systemd method invoked.
func (s *Server) serviceAction(w http.ResponseWriter, r *http.Request, action func(context.Context, string) systemd.ServiceAction) {
	if !s.validateCSRF(w, r) {
		return
	}
	name := r.PathValue("name")
	res := action(r.Context(), name)
	s.auditLog(r, "systemd", res.Action, res.ServiceName, res.Message, res.Success)
	s.renderServicesResult(w, r, res)
}

// renderServicesResult renders the services list with an optional error banner.
// Always returns 200 so HTMX swaps the content (avoids raw error text replacing the list).
func (s *Server) renderServicesResult(w http.ResponseWriter, r *http.Request, result systemd.ServiceAction) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !result.Success {
		setToast(w, "Failed: "+result.Message, "error")
		fmt.Fprintf(w,
			`<div class="rounded-lg bg-danger/10 border border-danger/30 p-3 mb-3 text-sm text-danger flex items-center gap-2">`+
				`<svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">`+
				`<circle cx="12" cy="12" r="10"/><line x1="15" x2="9" y1="9" y2="15"/><line x1="9" x2="15" y1="9" y2="15"/>`+
				`</svg>%s</div>`,
			html.EscapeString(result.Message),
		)
	} else {
		actionPast := "Started"
		if result.Action == "stop" {
			actionPast = "Stopped"
		} else if result.Action == "restart" {
			actionPast = "Restarted"
		}
		setToast(w, actionPast+" service: "+result.ServiceName, "success")
	}
	listHTML := s.renderPartial("partials/services-list.html", servicesPageData{
		Services:     s.systemd.Services(),
		ProcessStats: collectProcessUsage(),
		Available:    s.systemd.Available(),
	})
	w.Write([]byte(listHTML))
}
