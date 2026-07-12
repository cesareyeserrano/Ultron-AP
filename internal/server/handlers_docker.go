package server

import (
	"context"
	"net/http"

	"github.com/cesareyeserrano/ultron-ap/internal/docker"
)

type dockerPageData struct {
	Containers []docker.ContainerInfo
	Available  bool
}

func (s *Server) handleDockerPage(w http.ResponseWriter, r *http.Request) {
	data := dockerPageData{
		Containers: s.docker.Containers(),
		Available:  s.docker.Available(),
	}
	s.render(w, r, "docker.html", "Docker", "docker", data)
}

func (s *Server) handleDockerStart(w http.ResponseWriter, r *http.Request) {
	s.dockerAction(w, r, "Started", s.docker.StartContainer)
}

func (s *Server) handleDockerStop(w http.ResponseWriter, r *http.Request) {
	s.dockerAction(w, r, "Stopped", s.docker.StopContainer)
}

func (s *Server) handleDockerRestart(w http.ResponseWriter, r *http.Request) {
	s.dockerAction(w, r, "Restarted", s.docker.RestartContainer)
}

// dockerAction is the shared body for the start/stop/restart endpoints (D3),
// which differed only in the container method invoked and the success verb.
func (s *Server) dockerAction(w http.ResponseWriter, r *http.Request, verb string, action func(context.Context, string) docker.ContainerAction) {
	if !s.validateCSRF(w, r) {
		return
	}
	id := r.PathValue("id")
	res := action(r.Context(), id)
	s.auditLog(r, "docker", res.Action, res.ContainerName, res.Message, res.Success)

	if !res.Success {
		setToast(w, "Failed: "+res.Message, "error")
		http.Error(w, res.Message, http.StatusInternalServerError)
		return
	}
	setToast(w, verb+" container: "+res.ContainerName, "success")
	s.renderDockerList(w, r)
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

func (s *Server) renderDockerList(w http.ResponseWriter, r *http.Request) {
	containers := s.docker.Containers()
	html := s.renderPartial("partials/docker-list.html", containers)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

