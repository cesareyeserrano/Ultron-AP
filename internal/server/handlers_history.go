package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

const historyPageSize = 20

type historyPageData struct {
	Logs         []database.ActionLog
	SourceFilter string
	Page         int
	HasNext      bool
}

func (s *Server) handleHistoryPage(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	pageStr := r.URL.Query().Get("page")
	page := 0
	if n, err := strconv.Atoi(pageStr); err == nil && n > 0 {
		page = n
	}

	if source != "" && source != "docker" && source != "systemd" {
		source = ""
	}

	logs, err := s.db.ListActionLogsBySource(source, page, historyPageSize+1)
	if err != nil {
		log.Printf("history: failed to list action logs: %v", err)
		logs = nil
	}

	hasNext := len(logs) > historyPageSize
	if hasNext {
		logs = logs[:historyPageSize]
	}

	data := historyPageData{
		Logs:         logs,
		SourceFilter: source,
		Page:         page,
		HasNext:      hasNext,
	}
	s.render(w, r, "history.html", "History", "history", data)
}

func (s *Server) handleHistoryClear(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	source := r.FormValue("source")
	if source != "" && source != "docker" && source != "systemd" {
		source = ""
	}
	deleted, err := s.db.DeleteActionLogs(source)
	if err != nil {
		log.Printf("history: failed to clear: %v", err)
		http.Error(w, "Failed to clear history", http.StatusInternalServerError)
		return
	}
	s.auditLog(r, "history", "clear", source, fmt.Sprintf("deleted=%d", deleted), true)
	if r.Header.Get("HX-Request") == "true" {
		setToast(w, fmt.Sprintf("Cleared %d history records", deleted), "success")
		next := "/history"
		if source != "" {
			next += "?source=" + source
		}
		w.Header().Set("HX-Redirect", next)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/history", http.StatusSeeOther)
}
