package server

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sync"
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
	httpServer  *http.Server
	cfg         *config.Config
	db          *database.DB
	bruteForce  *auth.BruteForceTracker
	loginTokens sync.Map // token(string) -> expiry(time.Time), for login CSRF
	collector   *metrics.Collector
	docker      *docker.Monitor
	systemd     *systemd.Monitor
	alertEng    *alerts.Engine
	sseBroker   *sseBroker
	templates   fs.FS
	tmplCache   map[string]*template.Template // pre-parsed at startup
	startedAt   time.Time
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

	s.parseTemplates()
	s.registerRoutes(mux)
	s.startSSEBroadcast()
	s.startRetentionJob()

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
	mux.Handle("GET /docker", s.requireAuth(http.HandlerFunc(s.handleDockerPage)))
	mux.Handle("GET /services", s.requireAuth(http.HandlerFunc(s.handleServicesPage)))
	mux.Handle("GET /alerts", s.requireAuth(http.HandlerFunc(s.handleAlertsPage)))
	mux.Handle("GET /history", s.requireAuth(http.HandlerFunc(s.handleHistoryPage)))
	mux.Handle("GET /settings", s.requireAuth(http.HandlerFunc(s.handleSettings)))
	mux.Handle("GET /hardware", s.requireAuth(http.HandlerFunc(s.handleHardwarePage)))
	mux.Handle("POST /api/hardware/apply", s.requireAuth(http.HandlerFunc(s.handleHardwareApply)))

	// API routes (require auth)
	mux.Handle("GET /api/sse/dashboard", s.requireAuth(http.HandlerFunc(s.handleSSE)))
	mux.Handle("GET /api/docker/{id}", s.requireAuth(http.HandlerFunc(s.handleDockerDetail)))
	mux.Handle("POST /api/docker/{id}/start", s.requireAuth(http.HandlerFunc(s.handleDockerStart)))
	mux.Handle("POST /api/docker/{id}/stop", s.requireAuth(http.HandlerFunc(s.handleDockerStop)))
	mux.Handle("POST /api/docker/{id}/restart", s.requireAuth(http.HandlerFunc(s.handleDockerRestart)))
	mux.Handle("POST /api/alerts/rules", s.requireAuth(http.HandlerFunc(s.handleAlertRuleCreate)))
	mux.Handle("POST /api/alerts/rules/{id}/toggle", s.requireAuth(http.HandlerFunc(s.handleAlertRuleToggle)))
	mux.Handle("DELETE /api/alerts/rules/{id}", s.requireAuth(http.HandlerFunc(s.handleAlertRuleDelete)))
	mux.Handle("POST /api/alerts/{id}/acknowledge", s.requireAuth(http.HandlerFunc(s.handleAlertAcknowledge)))
	mux.Handle("POST /api/notifications/{channel}", s.requireAuth(http.HandlerFunc(s.handleNotificationSave)))
	mux.Handle("POST /api/services/{name}/start", s.requireAuth(http.HandlerFunc(s.handleServiceStart)))
	mux.Handle("POST /api/services/{name}/stop", s.requireAuth(http.HandlerFunc(s.handleServiceStop)))
	mux.Handle("POST /api/services/{name}/restart", s.requireAuth(http.HandlerFunc(s.handleServiceRestart)))
}

// startRetentionJob runs a daily cleanup of old ActionLog and Alert records.
func (s *Server) startRetentionJob() {
	go func() {
		// First run after 1 minute to avoid competing with startup I/O.
		timer := time.NewTimer(1 * time.Minute)
		defer timer.Stop()
		for {
			<-timer.C
			n, err := s.db.PruneOldData(30)
			if err != nil {
				log.Printf("retention: prune failed: %v", err)
			} else if n > 0 {
				log.Printf("retention: pruned %d records older than 30 days", n)
			}
			timer.Reset(24 * time.Hour)
		}
	}()
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
