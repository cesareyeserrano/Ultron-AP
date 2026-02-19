package server

import (
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
