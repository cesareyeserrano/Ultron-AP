package server

import (
	"encoding/json"
	"log"
	"net/http"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.setDashboardChartWindow(r.URL.Query().Get("window"))
	dd := s.gatherDashboardData()
	dd.Tailscale = gatherTailscaleData() // only on page load, not in SSE loop
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

// auditLog records an action in the database.
func (s *Server) auditLog(r *http.Request, source, action, target, message string, success bool) {
	result := "success"
	if !success {
		result = "error"
	}

	var userID *int64
	if uid, ok := UserIDFromContext(r.Context()); ok {
		userID = &uid
	}

	if err := s.db.LogAction(userID, source, action, target, result, message); err != nil {
		log.Printf("%s: failed to log action: %v", source, err)
	}
}
