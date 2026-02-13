package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/alerts"
	"github.com/cesareyeserrano/ultron-ap/internal/auth"
	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
	"github.com/cesareyeserrano/ultron-ap/web"
)

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	db         *database.DB
	bruteForce *auth.BruteForceTracker
	collector  *metrics.Collector
	docker     *docker.Monitor
	systemd    *systemd.Monitor
	alertEng   *alerts.Engine
	sseBroker  *sseBroker
	templates  fs.FS
	startedAt  time.Time
}

func New(cfg *config.Config, db *database.DB, collector *metrics.Collector, dockerMon *docker.Monitor, systemdMon *systemd.Monitor, alertEng *alerts.Engine) *Server {
	mux := http.NewServeMux()

	s := &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr(),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 0, // Disabled for SSE long-lived connections
			IdleTimeout:  120 * time.Second,
		},
		cfg:        cfg,
		db:         db,
		bruteForce: auth.NewBruteForceTracker(),
		collector:  collector,
		docker:     dockerMon,
		systemd:    systemdMon,
		alertEng:   alertEng,
		sseBroker:  newSSEBroker(),
		templates:  web.Templates,
		startedAt:  time.Now(),
	}

	s.registerRoutes(mux)
	s.startSSEBroadcast()

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
	mux.Handle("GET /settings", s.requireAuth(http.HandlerFunc(s.handleSettings)))

	// API routes (require auth)
	mux.Handle("GET /api/sse/dashboard", s.requireAuth(http.HandlerFunc(s.handleSSE)))
	mux.Handle("GET /api/docker/{id}", s.requireAuth(http.HandlerFunc(s.handleDockerDetail)))
	mux.Handle("POST /api/alerts/rules", s.requireAuth(http.HandlerFunc(s.handleAlertRuleCreate)))
	mux.Handle("POST /api/alerts/rules/{id}/toggle", s.requireAuth(http.HandlerFunc(s.handleAlertRuleToggle)))
	mux.Handle("DELETE /api/alerts/rules/{id}", s.requireAuth(http.HandlerFunc(s.handleAlertRuleDelete)))
	mux.Handle("POST /api/notifications/{channel}", s.requireAuth(http.HandlerFunc(s.handleNotificationSave)))
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
