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
	err := exec.Command("sudo", "-n", "shutdown", "-r", "now").Start()
	s.auditLog(r, "system", "restart", "host", "", err == nil)

	if err != nil {
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
	err := exec.Command("sudo", "-n", "shutdown", "-h", "now").Start()
	s.auditLog(r, "system", "shutdown", "host", "", err == nil)

	if err != nil {
		log.Printf("system: shutdown failed: %v", err)
		fmt.Fprintf(w, `<span class="text-danger text-xs">Error: %s</span>`, html.EscapeString(err.Error()))
		return
	}

	fmt.Fprint(w, `<span class="text-text-muted text-xs">Shutting down...</span>`)
}


func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "logs.html", "System Logs", "logs", nil)
}

func (s *Server) handleFetchSystemLogs(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	lines := 100

	var cmd *exec.Cmd
	switch source {
	case "ultron-ap":
		cmd = exec.Command("sudo", "-n", "journalctl", "-u", "ultron-ap", "-n", fmt.Sprintf("%d", lines), "--no-pager")
	case "docker":
		cmd = exec.Command("sudo", "-n", "journalctl", "-u", "docker", "-n", fmt.Sprintf("%d", lines), "--no-pager")
	case "kernel":
		cmd = exec.Command("sudo", "-n", "dmesg", "-T", "|", "tail", "-n", fmt.Sprintf("%d", lines))
		// Note: dmesg with pipe needs bash
		cmd = exec.Command("bash", "-c", fmt.Sprintf("sudo -n dmesg -T | tail -n %d", lines))
	default:
		http.Error(w, "Invalid log source", http.StatusBadRequest)
		return
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("system: failed to fetch logs for %s: %v", source, err)
		errMsg := string(out)
		if errMsg == "" {
			errMsg = err.Error()
		}
		http.Error(w, "Failed to fetch logs: "+errMsg, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(out)
}

func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	html := s.renderPartial("partials/tailscale-peers.html", gatherTailscaleData())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
