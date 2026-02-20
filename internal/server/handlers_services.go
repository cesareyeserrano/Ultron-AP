package server

import (
	"fmt"
	"html"
	"log"
	"net/http"

	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

type servicesPageData struct {
	Services  []systemd.ServiceInfo
	Available bool
}

func (s *Server) handleServicesPage(w http.ResponseWriter, r *http.Request) {
	data := servicesPageData{
		Services:  s.systemd.Services(),
		Available: s.systemd.Available(),
	}
	s.render(w, r, "services.html", "Services", "services", data)
}

func (s *Server) handleServiceStart(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	name := r.PathValue("name")
	result := s.systemd.StartService(r.Context(), name)
	s.logServiceAction(r, result)
	s.renderServicesResult(w, r, result)
}

func (s *Server) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	name := r.PathValue("name")
	result := s.systemd.StopService(r.Context(), name)
	s.logServiceAction(r, result)
	s.renderServicesResult(w, r, result)
}

func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	name := r.PathValue("name")
	result := s.systemd.RestartService(r.Context(), name)
	s.logServiceAction(r, result)
	s.renderServicesResult(w, r, result)
}

// renderServicesResult renders the services list with an optional error banner.
// Always returns 200 so HTMX swaps the content (avoids raw error text replacing the list).
func (s *Server) renderServicesResult(w http.ResponseWriter, r *http.Request, result systemd.ServiceAction) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !result.Success {
		fmt.Fprintf(w,
			`<div class="rounded-lg bg-danger/10 border border-danger/30 p-3 mb-3 text-sm text-danger flex items-center gap-2">`+
				`<svg class="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">`+
				`<circle cx="12" cy="12" r="10"/><line x1="15" x2="9" y1="9" y2="15"/><line x1="9" x2="15" y1="9" y2="15"/>`+
				`</svg>%s</div>`,
			html.EscapeString(result.Message),
		)
	}
	listHTML := s.renderPartial("partials/services-list.html", s.systemd.Services())
	w.Write([]byte(listHTML))
}

func (s *Server) logServiceAction(r *http.Request, result systemd.ServiceAction) {
	resultStr := "success"
	if !result.Success {
		resultStr = "error"
	}

	var userID *int64
	if uid, ok := UserIDFromContext(r.Context()); ok {
		userID = &uid
	}

	if err := s.db.LogAction(userID, "systemd", result.Action, result.ServiceName, resultStr, result.Message); err != nil {
		log.Printf("systemd: failed to log action: %v", err)
	}
}
