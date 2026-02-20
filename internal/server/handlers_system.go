package server

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"os/exec"
)

func (s *Server) handleSystemRestart(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	log.Println("system: restart requested via UI")
	if err := exec.Command("sudo", "-n", "shutdown", "-r", "now").Start(); err != nil {
		log.Printf("system: restart failed: %v", err)
		fmt.Fprintf(w, `<span class="text-danger text-xs">Error: %s</span>`, html.EscapeString(err.Error()))
		return
	}

	fmt.Fprint(w, `<span class="text-text-muted text-xs">Restarting...</span>`)
}

func (s *Server) handleSystemShutdown(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	if r.FormValue("confirm") != "shutdown" {
		http.Error(w, "confirmation required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	log.Println("system: shutdown requested via UI")
	if err := exec.Command("sudo", "-n", "shutdown", "-h", "now").Start(); err != nil {
		log.Printf("system: shutdown failed: %v", err)
		fmt.Fprintf(w, `<span class="text-danger text-xs">Error: %s</span>`, html.EscapeString(err.Error()))
		return
	}

	fmt.Fprint(w, `<span class="text-text-muted text-xs">Shutting down...</span>`)
}

func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	html := s.renderPartial("partials/tailscale-peers.html", gatherTailscaleData())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
