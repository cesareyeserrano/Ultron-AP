package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"
)

// PageData holds common data passed to all page templates.
type PageData struct {
	Title      string
	ActivePage string
	Uptime     string
	Username   string
	CSRFToken  string
	Content    interface{}
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, title string, activePage string, content interface{}) {
	tmpl, err := template.ParseFS(s.templates,
		"templates/base.html",
		"templates/partials/sidebar.html",
		"templates/partials/header.html",
		fmt.Sprintf("templates/%s", page),
	)
	if err != nil {
		log.Printf("Failed to parse templates for %s: %v", page, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Build page data
	data := PageData{
		Title:      title,
		ActivePage: activePage,
		Uptime:     formatUptime(time.Since(s.startedAt)),
		Content:    content,
	}

	// Get username from session context
	if userID, ok := UserIDFromContext(r.Context()); ok {
		user, err := s.db.GetUserByID(userID)
		if err == nil && user != nil {
			data.Username = user.Username
		}
	}

	// Get CSRF token from session
	if cookie, err := r.Cookie("session"); err == nil {
		session, err := s.db.GetSession(cookie.Value)
		if err == nil && session != nil {
			data.CSRFToken = session.CSRFToken
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Failed to execute template %s: %v", page, err)
	}
}

// formatUptime formats a duration into a human-readable string like "2d 5h 30m" or "45m".
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
