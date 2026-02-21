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
	data := dockerPageData{
		Containers: s.docker.Containers(),
		Available:  s.docker.Available(),
	}
	s.render(w, r, "docker.html", "Docker", "docker", data)
}

func (s *Server) handleDockerStart(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	id := r.PathValue("id")
	res := s.docker.StartContainer(r.Context(), id)
	s.auditLog(r, "docker", res.Action, res.ContainerName, res.Message, res.Success)

	if !res.Success {
		http.Error(w, res.Message, http.StatusInternalServerError)
		return
	}
	s.renderDockerList(w, r)
}

func (s *Server) handleDockerStop(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	id := r.PathValue("id")
	res := s.docker.StopContainer(r.Context(), id)
	s.auditLog(r, "docker", res.Action, res.ContainerName, res.Message, res.Success)

	if !res.Success {
		http.Error(w, res.Message, http.StatusInternalServerError)
		return
	}
	s.renderDockerList(w, r)
}

func (s *Server) handleDockerRestart(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	id := r.PathValue("id")
	res := s.docker.RestartContainer(r.Context(), id)
	s.auditLog(r, "docker", res.Action, res.ContainerName, res.Message, res.Success)

	if !res.Success {
		http.Error(w, res.Message, http.StatusInternalServerError)
		return
	}
	s.renderDockerList(w, r)
}

func (s *Server) renderDockerList(w http.ResponseWriter, r *http.Request) {
	containers := s.docker.Containers()
	html := s.renderPartial("partials/docker-list.html", containers)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

