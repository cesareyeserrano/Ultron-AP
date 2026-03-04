package server

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

type logSourceOption struct {
	Value string
	Label string
}

type logsPageData struct {
	Sources []logSourceOption
}

func (s *Server) handleSystemRestart(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	log.Println("system: restart requested via UI")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	err := s.privileged.Shutdown(ctx, "restart")
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
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	err := s.privileged.Shutdown(ctx, "poweroff")
	s.auditLog(r, "system", "shutdown", "host", "", err == nil)

	if err != nil {
		log.Printf("system: shutdown failed: %v", err)
		fmt.Fprintf(w, `<span class="text-danger text-xs">Error: %s</span>`, html.EscapeString(err.Error()))
		return
	}

	fmt.Fprint(w, `<span class="text-text-muted text-xs">Shutting down...</span>`)
}

func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	sources := []logSourceOption{
		{Value: "ultron-ap", Label: "Ultron-AP Service"},
		{Value: "docker", Label: "Docker Daemon"},
		{Value: "kernel", Label: "Kernel (dmesg)"},
		{Value: "cpu", Label: "CPU Usage Snapshot"},
		{Value: "memory", Label: "Memory Usage Snapshot"},
		{Value: "pironman", Label: "Pironman Logs"},
		{Value: "homeassistant", Label: "Home Assistant Logs"},
	}
	if s.systemd != nil {
		services := s.systemd.Services()
		sort.SliceStable(services, func(i, j int) bool { return services[i].Name < services[j].Name })
		for _, svc := range services {
			name := strings.TrimSpace(svc.Name)
			if name == "" {
				continue
			}
			sources = append(sources, logSourceOption{
				Value: "service:" + name,
				Label: "Service: " + name,
			})
		}
	}
	s.render(w, r, "logs.html", "System Logs", "logs", logsPageData{Sources: sources})
}

func (s *Server) handleFetchSystemLogs(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	lines := 100

	if source == "" {
		source = "ultron-ap"
	}
	if !isValidLogSource(source) {
		http.Error(w, "Invalid log source", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out, err := s.privileged.SystemLogs(ctx, source, lines)
	if err != nil {
		log.Printf("system: failed to fetch logs for %s: %v", source, err)
		http.Error(w, "Failed to fetch logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(out))
}

func isValidLogSource(source string) bool {
	switch source {
	case "ultron-ap", "docker", "kernel", "cpu", "memory", "pironman", "homeassistant":
		return true
	default:
		return strings.HasPrefix(source, "service:")
	}
}

func (s *Server) handleTailscaleStatus(w http.ResponseWriter, r *http.Request) {
	html := s.renderPartial("partials/tailscale-peers.html", gatherTailscaleData())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
