package server

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// PageData holds common data passed to all page templates.
type PageData struct {
	Title        string
	ActivePage   string
	Uptime       string
	Username     string
	CSRFToken    string
	Version      string
	AssetVersion string
	Content      interface{}
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, title string, activePage string, content interface{}) {
	tmpl, ok := s.tmplCache[page]
	if !ok {
		log.Printf("render: page template not in cache: %s", page)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Build page data
	data := PageData{
		Title:        title,
		ActivePage:   activePage,
		Uptime:       formatUptime(time.Since(s.startedAt)),
		Version:      Version,
		AssetVersion: s.assetVersion,
		Content:      content,
	}

	// Get username from session context
	if userID, ok := UserIDFromContext(r.Context()); ok {
		user, err := s.db.GetUserByID(userID)
		if err == nil && user != nil {
			data.Username = user.Username
		}
	}

	data.CSRFToken = s.sessionCSRFToken(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Failed to execute template %s: %v", page, err)
	}
}

func (s *Server) sessionCSRFToken(r *http.Request) string {
	cookie, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	session, err := s.db.GetSession(cookie.Value)
	if err != nil || session == nil {
		return ""
	}
	return session.CSRFToken
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
