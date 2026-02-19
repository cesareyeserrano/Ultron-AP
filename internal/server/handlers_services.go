package server

import (
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

	if !result.Success {
		http.Error(w, result.Message, http.StatusInternalServerError)
		return
	}
	s.renderServicesList(w, r)
}

func (s *Server) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	name := r.PathValue("name")
	result := s.systemd.StopService(r.Context(), name)
	s.logServiceAction(r, result)

	if !result.Success {
		http.Error(w, result.Message, http.StatusInternalServerError)
		return
	}
	s.renderServicesList(w, r)
}

func (s *Server) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	name := r.PathValue("name")
	result := s.systemd.RestartService(r.Context(), name)
	s.logServiceAction(r, result)

	if !result.Success {
		http.Error(w, result.Message, http.StatusInternalServerError)
		return
	}
	s.renderServicesList(w, r)
}

func (s *Server) renderServicesList(w http.ResponseWriter, r *http.Request) {
	html := s.renderPartial("partials/services-list.html", s.systemd.Services())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
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

	if err := s.db.LogAction(userID, result.Action, result.ServiceName, resultStr, result.Message); err != nil {
		log.Printf("systemd: failed to log action: %v", err)
	}
}
