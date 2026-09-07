package server

import (
	"net/http"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
)

type dockerPageData struct {
	Containers []docker.ContainerInfo
	Available  bool
}

func (s *Server) handleDockerPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "docker.html", "Docker", "docker", s.dockerData())
}

func (s *Server) handleDockerLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || s.docker == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	logs, err := s.docker.FetchLogs(r.Context(), id, 100)
	if err != nil {
		http.Error(w, "Failed to fetch logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(logs))
}

// dockerData snapshots both the container list AND whether it could be read.
// The two must travel together: the template decides "cannot read" before it
// decides "read fine, none found", and rendering the empty state on a read
// failure is exactly the defect this feature fixes (FR-091).
func (s *Server) dockerData() dockerPageData {
	return dockerPageData{
		Containers: s.docker.Containers(),
		Available:  s.docker.Available(),
	}
}
