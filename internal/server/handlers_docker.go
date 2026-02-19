package server

import (
	"log"
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
	result := s.docker.StartContainer(r.Context(), id)
	s.logDockerAction(r, result)

	if !result.Success {
		http.Error(w, result.Message, http.StatusInternalServerError)
		return
	}
	s.renderDockerList(w, r)
}

func (s *Server) handleDockerStop(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	id := r.PathValue("id")
	result := s.docker.StopContainer(r.Context(), id)
	s.logDockerAction(r, result)

	if !result.Success {
		http.Error(w, result.Message, http.StatusInternalServerError)
		return
	}
	s.renderDockerList(w, r)
}

func (s *Server) handleDockerRestart(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	id := r.PathValue("id")
	result := s.docker.RestartContainer(r.Context(), id)
	s.logDockerAction(r, result)

	if !result.Success {
		http.Error(w, result.Message, http.StatusInternalServerError)
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

func (s *Server) logDockerAction(r *http.Request, result docker.ContainerAction) {
	resultStr := "success"
	if !result.Success {
		resultStr = "error"
	}

	var userID *int64
	if uid, ok := UserIDFromContext(r.Context()); ok {
		userID = &uid
	}

	if err := s.db.LogAction(userID, "docker", result.Action, result.ContainerName, resultStr, result.Message); err != nil {
		log.Printf("docker: failed to log action: %v", err)
	}
}
