package server

import (
	"log"
	"net/http"
	"strconv"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

type alertsPageData struct {
	Alerts         []database.Alert
	UnackCount     int
	SeverityFilter string
}

func (s *Server) handleAlertsPage(w http.ResponseWriter, r *http.Request) {
	severity := r.URL.Query().Get("severity")
	var alerts []database.Alert
	var err error

	if severity != "" && isValidSeverity(severity) {
		alerts, err = s.db.ListAlertsBySeverity(severity, 100)
	} else {
		alerts, err = s.db.ListAlerts(100)
		severity = ""
	}

	if err != nil {
		log.Printf("alerts page: failed to list alerts: %v", err)
	}

	unackCount, _ := s.db.UnacknowledgedAlertCount()

	data := alertsPageData{
		Alerts:         alerts,
		UnackCount:     unackCount,
		SeverityFilter: severity,
	}

	s.render(w, r, "alerts.html", "Alerts", "alerts", data)
}

func (s *Server) handleAlertAcknowledge(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.db.AcknowledgeAlert(id); err != nil {
		log.Printf("alerts: failed to acknowledge: %v", err)
		http.Error(w, "Failed to acknowledge", http.StatusInternalServerError)
		return
	}

	// Return updated alert list
	s.renderAlertsList(w, r)
}

func (s *Server) renderAlertsList(w http.ResponseWriter, r *http.Request) {
	severity := r.URL.Query().Get("severity")
	var alerts []database.Alert

	if severity != "" && isValidSeverity(severity) {
		alerts, _ = s.db.ListAlertsBySeverity(severity, 100)
	} else {
		alerts, _ = s.db.ListAlerts(100)
	}

	html := s.renderPartial("partials/alerts-list.html", alerts)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
