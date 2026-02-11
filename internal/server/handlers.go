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
	s.render(w, r, "dashboard.html", "Dashboard", "dashboard", nil)
}

// handlePlaceholderPage returns a handler for future pages that shows a "coming soon" message.
func (s *Server) handlePlaceholderPage(title, activePage string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, r, "placeholder.html", title, activePage, nil)
	}
}
