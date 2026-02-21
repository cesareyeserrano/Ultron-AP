package server

import (
	"fmt"
	"html/template"
	"log"
)

// parseTemplates pre-parses every template once at startup and stores the
// results in s.tmplCache. This eliminates the ParseFS call on each SSE
// broadcast and each page render, which was a significant CPU overhead on
// constrained hardware (Raspberry Pi).
//
// Keys in the cache follow the path relative to "templates/", e.g.:
//   - "partials/sse-metrics.html"  — SSE / HTMX partials
//   - "dashboard.html"             — full page templates
//   - "login.html"                 — standalone login page
func (s *Server) parseTemplates() {
	s.tmplCache = make(map[string]*template.Template)

	// FuncMap shared by all SSE / HTMX partials.
	partialFuncs := template.FuncMap{
		"formatBytes":    formatBytes,
		"formatPercent":  formatPercent,
		"tempColor":      tempColor,
		"healthColor":    healthColor,
		"svcHealthColor": svcHealthColor,
		"shortID":        shortID,
		"sparklineSVG":   sparklineSVG,
		"formatTemp":     formatTemp,
		"deref":          derefFloat,
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
	}

	partials := []string{
		"partials/sse-metrics.html",
		"partials/sse-docker.html",
		"partials/sse-systemd.html",
		"partials/sse-charts.html",
		"partials/services-list.html",
		"partials/docker-list.html",
		"partials/docker-detail.html",
		"partials/alerts-list.html",
		"partials/alert-rules-table.html",
		"partials/hardware-form.html",
		"partials/tailscale-peers.html",
	}

	for _, name := range partials {
		tmpl, err := template.New("").Funcs(partialFuncs).ParseFS(s.templates, "templates/"+name)
		if err != nil {
			log.Fatalf("templates: parse partial %s: %v", name, err)
		}
		s.tmplCache[name] = tmpl
	}

	// Standalone login page (no {{define}} wrapper).
	loginTmpl, err := template.ParseFS(s.templates, "templates/login.html")
	if err != nil {
		log.Fatalf("templates: parse login.html: %v", err)
	}
	s.tmplCache["login.html"] = loginTmpl

	// FuncMap for full page templates.
	pageFuncs := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
	}

	base := []string{
		"templates/base.html",
		"templates/partials/sidebar.html",
		"templates/partials/header.html",
	}

	type pageSpec struct {
		name  string
		extra []string
	}

	pages := []pageSpec{
		{"dashboard.html", []string{"templates/partials/tailscale-peers.html"}},
		{"docker.html", []string{"templates/partials/docker-list.html"}},
		{"services.html", []string{"templates/partials/services-list.html"}},
		{"alerts.html", []string{"templates/partials/alerts-list.html"}},
		{"history.html", nil},
		{"settings.html", []string{"templates/partials/alert-rules-table.html"}},
		{"hardware.html", []string{"templates/partials/hardware-form.html"}},
		{"placeholder.html", nil},
	}

	for _, p := range pages {
		patterns := make([]string, 0, len(base)+1+len(p.extra))
		patterns = append(patterns, base...)
		patterns = append(patterns, fmt.Sprintf("templates/%s", p.name))
		patterns = append(patterns, p.extra...)

		tmpl, err := template.New("base.html").Funcs(pageFuncs).ParseFS(s.templates, patterns...)
		if err != nil {
			log.Fatalf("templates: parse page %s: %v", p.name, err)
		}
		s.tmplCache[p.name] = tmpl
	}
}
