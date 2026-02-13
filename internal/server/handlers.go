package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	dd := s.gatherDashboardData()
	s.render(w, r, "dashboard.html", "Dashboard", "dashboard", dd)
}

func (s *Server) handleDockerDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || s.docker == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	detail, err := s.docker.ContainerDetail(r.Context(), id)
	if err != nil {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	html := s.renderPartial("partials/docker-detail.html", detail)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handlePlaceholderPage returns a handler for future pages that shows a "coming soon" message.
func (s *Server) handlePlaceholderPage(title, activePage string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, r, "placeholder.html", title, activePage, nil)
	}
}
