package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/auth"
	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/web"
)

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	db         *database.DB
	bruteForce *auth.BruteForceTracker
	collector  *metrics.Collector
	templates  fs.FS
	startedAt  time.Time
}

func New(cfg *config.Config, db *database.DB, collector *metrics.Collector) *Server {
	mux := http.NewServeMux()

	s := &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr(),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		cfg:        cfg,
		db:         db,
		bruteForce: auth.NewBruteForceTracker(),
		collector:  collector,
		templates:  web.Templates,
		startedAt:  time.Now(),
	}

	s.registerRoutes(mux)

	return s
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Public routes (no auth)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)

	// Static files
	staticFS, _ := fs.Sub(web.Static, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Protected routes (require auth)
	mux.Handle("POST /logout", s.requireAuth(http.HandlerFunc(s.handleLogout)))
	mux.Handle("GET /", s.requireAuth(http.HandlerFunc(s.handleDashboard)))
	mux.Handle("GET /docker", s.requireAuth(http.HandlerFunc(s.handlePlaceholderPage("Docker", "docker"))))
	mux.Handle("GET /services", s.requireAuth(http.HandlerFunc(s.handlePlaceholderPage("Services", "services"))))
	mux.Handle("GET /alerts", s.requireAuth(http.HandlerFunc(s.handlePlaceholderPage("Alerts", "alerts"))))
	mux.Handle("GET /settings", s.requireAuth(http.HandlerFunc(s.handlePlaceholderPage("Settings", "settings"))))
}

func (s *Server) Start() error {
	log.Printf("Server started on %s", s.cfg.Addr())
	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}
