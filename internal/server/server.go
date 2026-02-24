package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/alerts"
	"github.com/cesareyeserrano/ultron-ap/internal/auth"
	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/notify"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
	"github.com/cesareyeserrano/ultron-ap/web"
)

const Version = "v1.0.0"

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	db         *database.DB
	bruteForce *auth.BruteForceTracker
	// bruteForceSweepNs throttles stale brute-force attempt cleanup cadence.
	bruteForceSweepNs atomic.Int64
	loginTokens       sync.Map // token(string) -> expiry(time.Time), for login CSRF
	// loginTokenSweepNs throttles expired login-token cleanup cadence.
	loginTokenSweepNs atomic.Int64
	collector         *metrics.Collector
	reader            *metrics.SystemReader
	docker            *docker.Monitor
	systemd           *systemd.Monitor
	alertEng          *alerts.Engine
	sseBroker         *sseBroker
	templates         fs.FS
	tmplCache         map[string]*template.Template // pre-parsed at startup
	startedAt         time.Time

	// sseIntervalNs holds the SSE broadcast interval as nanoseconds (atomic).
	sseIntervalNs atomic.Int64

	// backupIntervalHours holds the automated backup interval in hours (atomic).
	backupIntervalHours atomic.Int64

	// Alert count TTL cache — avoids a DB query on every SSE tick.
	alertCountMu     sync.Mutex
	alertCountCached int
	alertCountExpiry time.Time
}

func New(cfg *config.Config, db *database.DB, reader *metrics.SystemReader, collector *metrics.Collector, dockerMon *docker.Monitor, systemdMon *systemd.Monitor, alertEng *alerts.Engine) *Server {
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
		reader:     reader,
		collector:  collector,
		docker:     dockerMon,
		systemd:    systemdMon,
		alertEng:   alertEng,
		sseBroker:  newSSEBroker(),
		templates:  web.Templates,
		startedAt:  time.Now(),
	}
	s.sseIntervalNs.Store(int64(5 * time.Second))
	s.backupIntervalHours.Store(24) // Default 24h

	s.parseTemplates()
	s.registerRoutes(mux)
	s.startSSEBroadcast()
	s.startRetentionJob()
	s.startBackupJob()

	return s
}

// ApplyPerformanceConfig applies all configurable intervals at once.
// Safe to call from main at startup and from the settings save handler.
func (s *Server) ApplyPerformanceConfig(cfg database.PerformanceConfig) {
	if cfg.SSEIntervalSec >= 2 {
		s.sseIntervalNs.Store(int64(time.Duration(cfg.SSEIntervalSec) * time.Second))
	}
	if cfg.BackupIntervalHours >= 1 {
		s.backupIntervalHours.Store(int64(cfg.BackupIntervalHours))
	}
	if s.reader != nil && cfg.DiskIntervalMin >= 1 {
		s.reader.SetDiskInterval(time.Duration(cfg.DiskIntervalMin) * time.Minute)
	}
	if s.docker != nil && cfg.DockerIntervalSec >= 5 {
		s.docker.SetInterval(time.Duration(cfg.DockerIntervalSec) * time.Second)
	}
	if s.systemd != nil && cfg.SystemdIntervalSec >= 5 {
		s.systemd.SetInterval(time.Duration(cfg.SystemdIntervalSec) * time.Second)
	}
}

// startBackupJob runs automated backups at the configured interval.
func (s *Server) startBackupJob() {
	go func() {
		// First run after 15 minutes to avoid competing with startup I/O.
		timer := time.NewTimer(15 * time.Minute)
		defer timer.Stop()
		for {
			<-timer.C

			// Always get fresh interval
			intervalHours := s.backupIntervalHours.Load()
			if intervalHours < 1 {
				intervalHours = 24
			}

			log.Printf("backup: running automated backup (interval=%dh)", intervalHours)
			err := s.performAutomatedBackup()
			s.recordBackupOutcome(err)

			// Schedule next run
			log.Printf("backup: next automated backup scheduled in %dh", intervalHours)
			timer.Reset(time.Duration(intervalHours) * time.Hour)
		}
	}()
}

func (s *Server) recordBackupOutcome(err error) {
	if err != nil {
		log.Printf("backup: automated backup failed: %v", err)
		_ = s.db.LogAction(nil, "backup", "automated", "database", "error", err.Error())
		return
	}
	_ = s.db.LogAction(nil, "backup", "automated", "database", "success", "automated backup completed")
}

func (s *Server) performAutomatedBackup() error {
	log.Println("backup: starting automated backup job...")

	// 1. Create local backup file
	backupDir := filepath.Join(filepath.Dir(s.cfg.DBPath), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Printf("backup: failed to create backup dir: %v", err)
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("ultron-%s.db", timestamp))

	if err := s.db.Backup(backupPath); err != nil {
		log.Printf("backup: failed to create local backup: %v", err)
		return fmt.Errorf("failed to create local backup: %w", err)
	}
	log.Printf("backup: local backup created at %s", backupPath)

	// Always enforce local retention after creating a backup, even if remote upload fails.
	defer func() {
		files, err := os.ReadDir(backupDir)
		if err != nil {
			log.Printf("backup: retention read failed: %v", err)
			return
		}
		if len(files) <= 7 {
			return
		}
		for i := 0; i < len(files)-7; i++ {
			path := filepath.Join(backupDir, files[i].Name())
			if err := os.Remove(path); err != nil {
				log.Printf("backup: retention remove failed for %s: %v", path, err)
			}
		}
	}()

	// 2. Send to Telegram if configured
	nc, err := s.db.GetNotificationConfig("telegram")
	if err != nil || nc == nil || !nc.Enabled {
		return fmt.Errorf("telegram notifications are not enabled or configured")
	}

	var configMap map[string]string
	if err := json.Unmarshal([]byte(nc.Config), &configMap); err != nil {
		return fmt.Errorf("failed to parse telegram configuration: %w", err)
	}

	botToken := configMap["bot_token"]
	chatID := configMap["chat_id"]
	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram bot token or chat ID is missing")
	}

	sender := notify.NewTelegramSender(botToken, chatID)
	hostname, _ := os.Hostname()
	caption := fmt.Sprintf("\xf0\x9f\x92\xbe *Ultron-AP Automated Backup*\n\nTimestamp: `%s` \nVersion: `%s` \nDevice: `%s` \nStatus: `Success`",
		time.Now().Format("2006-01-02 15:04:05"), Version, hostname)

	if err := sender.SendFile(backupPath, caption); err != nil {
		log.Printf("backup: failed to send telegram backup: %v", err)
		return fmt.Errorf("failed to send file to telegram: %w", err)
	}

	log.Println("backup: telegram backup sent successfully")

	return nil
}

// sseInterval returns the current SSE broadcast interval.
func (s *Server) sseInterval() time.Duration {
	v := s.sseIntervalNs.Load()
	if v <= 0 {
		return 5 * time.Second
	}
	return time.Duration(v)
}

// cachedAlertCount returns the unacknowledged alert count, refreshing at most
// once per 30 seconds to avoid a DB query on every SSE tick.
func (s *Server) cachedAlertCount() int {
	s.alertCountMu.Lock()
	defer s.alertCountMu.Unlock()
	if time.Now().Before(s.alertCountExpiry) {
		return s.alertCountCached
	}
	count, _ := s.db.UnacknowledgedAlertCount()
	s.alertCountCached = count
	s.alertCountExpiry = time.Now().Add(30 * time.Second)
	return s.alertCountCached
}

// invalidateAlertCount forces the next cachedAlertCount call to hit the DB.
func (s *Server) invalidateAlertCount() {
	s.alertCountMu.Lock()
	s.alertCountExpiry = time.Time{}
	s.alertCountMu.Unlock()
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
	mux.Handle("GET /logs", s.requireAuth(http.HandlerFunc(s.handleLogsPage)))
	mux.Handle("GET /settings", s.requireAuth(http.HandlerFunc(s.handleSettings)))
	mux.Handle("GET /hardware", s.requireAuth(http.HandlerFunc(s.handleHardwarePage)))
	mux.Handle("POST /api/hardware/apply", s.requireAuth(http.HandlerFunc(s.handleHardwareApply)))

	// API routes (require auth)
	mux.Handle("GET /api/sse/dashboard", s.requireAuth(http.HandlerFunc(s.handleSSE)))
	mux.Handle("GET /api/docker/{id}", s.requireAuth(http.HandlerFunc(s.handleDockerDetail)))
	mux.Handle("GET /api/docker/{id}/logs", s.requireAuth(http.HandlerFunc(s.handleDockerLogs)))
	mux.Handle("POST /api/docker/{id}/start", s.requireAuth(http.HandlerFunc(s.handleDockerStart)))
	mux.Handle("POST /api/docker/{id}/stop", s.requireAuth(http.HandlerFunc(s.handleDockerStop)))
	mux.Handle("POST /api/docker/{id}/restart", s.requireAuth(http.HandlerFunc(s.handleDockerRestart)))
	mux.Handle("POST /api/alerts/rules", s.requireAuth(http.HandlerFunc(s.handleAlertRuleCreate)))
	mux.Handle("POST /api/alerts/rules/{id}/toggle", s.requireAuth(http.HandlerFunc(s.handleAlertRuleToggle)))
	mux.Handle("DELETE /api/alerts/rules/{id}", s.requireAuth(http.HandlerFunc(s.handleAlertRuleDelete)))
	mux.Handle("POST /api/alerts/{id}/acknowledge", s.requireAuth(http.HandlerFunc(s.handleAlertAcknowledge)))
	mux.Handle("GET /api/settings/backup", s.requireAuth(http.HandlerFunc(s.handleSettingsBackup)))
	mux.Handle("POST /api/settings/backup/run", s.requireAuth(http.HandlerFunc(s.handleSettingsBackupRun)))
	mux.Handle("POST /api/notifications/{channel}", s.requireAuth(http.HandlerFunc(s.handleNotificationSave)))
	mux.Handle("POST /api/notifications/{channel}/test", s.requireAuth(http.HandlerFunc(s.handleNotificationTest)))
	mux.Handle("POST /api/performance", s.requireAuth(http.HandlerFunc(s.handlePerformanceSave)))
	mux.Handle("POST /api/services/{name}/start", s.requireAuth(http.HandlerFunc(s.handleServiceStart)))
	mux.Handle("POST /api/services/{name}/stop", s.requireAuth(http.HandlerFunc(s.handleServiceStop)))
	mux.Handle("POST /api/services/{name}/restart", s.requireAuth(http.HandlerFunc(s.handleServiceRestart)))
	mux.Handle("GET /api/tailscale/status", s.requireAuth(http.HandlerFunc(s.handleTailscaleStatus)))
	mux.Handle("GET /api/system/logs", s.requireAuth(http.HandlerFunc(s.handleFetchSystemLogs)))
	mux.Handle("POST /api/system/restart", s.requireAuth(http.HandlerFunc(s.handleSystemRestart)))
	mux.Handle("POST /api/system/shutdown", s.requireAuth(http.HandlerFunc(s.handleSystemShutdown)))
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
			deleted, err := s.db.DeleteExpiredSessions()
			if err != nil {
				log.Printf("retention: session cleanup failed: %v", err)
			} else if deleted > 0 {
				log.Printf("retention: deleted %d expired sessions", deleted)
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
