package server

import (
	"database/sql"
	"fmt"
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
	if severity == "" {
		severity = r.FormValue("severity")
	}
	var alerts []database.Alert
	var listErr error

	if severity != "" && isValidSeverity(severity) {
		alerts, listErr = s.db.ListAlertsBySeverity(severity, 100)
	} else {
		alerts, listErr = s.db.ListAlerts(100)
	}
	// F6: don't silently render an empty list on a DB error — an empty result
	// then looks identical to a healthy "no alerts" state. Log it and signal
	// the client so it can show a retry banner instead of a false all-clear.
	if listErr != nil {
		log.Printf("alerts: list query failed (severity=%q): %v", severity, listErr)
		setToast(w, "Could not load alerts, retry shortly", "error")
		http.Error(w, "Failed to load alerts", http.StatusInternalServerError)
		return
	}

	csrfToken := s.sessionCSRFToken(r)

	data := map[string]interface{}{
		"Alerts":    alerts,
		"CSRFToken": csrfToken,
	}

	html := s.renderPartial("partials/alerts-list.html", data)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (s *Server) handleAlertsClear(w http.ResponseWriter, r *http.Request) {
	if !s.validateCSRF(w, r) {
		return
	}
	severity := r.FormValue("severity")
	if severity != "" && !isValidSeverity(severity) {
		severity = ""
	}

	var (
		deleted int64
		userID  *int64
	)
	if uid, ok := UserIDFromContext(r.Context()); ok {
		userID = &uid
	}
	// Atomic: the delete and the audit log row commit together. If the
	// audit insert fails, the alerts are NOT deleted — preserves the
	// FR-006 invariant that no privileged mutation lands without a
	// matching ActionLog row (BG-024 / BL-010).
	err := s.db.WithAuditTx(func(tx *sql.Tx, entry *database.ActionLogEntry) error {
		var dErr error
		deleted, dErr = s.db.DeleteAlertsTx(tx, severity)
		if dErr != nil {
			return dErr
		}
		entry.UserID = userID
		entry.Source = "alerts"
		entry.Action = "clear"
		entry.Target = severity
		entry.Result = "success"
		entry.Details = fmt.Sprintf("deleted=%d", deleted)
		return nil
	})
	if err != nil {
		log.Printf("alerts: failed to clear: %v", err)
		http.Error(w, "Failed to clear alerts", http.StatusInternalServerError)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		setToast(w, fmt.Sprintf("Cleared %d alerts", deleted), "success")
		s.renderAlertsList(w, r)
		return
	}
	http.Redirect(w, r, "/alerts", http.StatusSeeOther)
}
