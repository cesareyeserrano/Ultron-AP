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
	"strings"
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
	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
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
	backupEnabled       atomic.Int64
	backupRetention     atomic.Int64
	backupDestination   atomic.Value // string
	backupLocalPath     atomic.Value // string
	backupScheduleMode  atomic.Value // string
	backupScheduleHour  atomic.Int64
	backupScheduleMin   atomic.Int64
	backupEncrypt       atomic.Int64
	backupKeyRef        atomic.Value // string
	backupUploadTimeout atomic.Int64
	backupMaxUploadMB   atomic.Int64
	backupRescheduleCh  chan struct{}
	privileged          *privileged.Client

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
			Handler:      nil,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 15 * time.Second,
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
		privileged: privileged.NewClient(cfg.HelperSocket, cfg.HelperTimeout),
		// Buffered channel so config updates can request a reschedule without blocking.
		backupRescheduleCh: make(chan struct{}, 1),
	}
	s.sseIntervalNs.Store(int64(5 * time.Second))
	s.backupIntervalHours.Store(24) // Default 24h
	s.backupEnabled.Store(1)
	s.backupRetention.Store(7)
	s.backupDestination.Store("local_only")
	s.backupLocalPath.Store("")
	s.backupScheduleMode.Store("interval")
	s.backupScheduleHour.Store(3)
	s.backupScheduleMin.Store(0)
	s.backupEncrypt.Store(0)
	s.backupKeyRef.Store("")
	s.backupUploadTimeout.Store(30)
	s.backupMaxUploadMB.Store(50)

	s.parseTemplates()
	s.registerRoutes(mux)
	s.httpServer.Handler = s.securityHeaders(mux)
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

func (s *Server) ApplyBackupConfig(cfg database.BackupConfig) {
	if cfg.Enabled {
		s.backupEnabled.Store(1)
	} else {
		s.backupEnabled.Store(0)
	}
	if cfg.IntervalHours >= 1 {
		s.backupIntervalHours.Store(int64(cfg.IntervalHours))
	}
	if cfg.RetentionCount >= 1 {
		s.backupRetention.Store(int64(cfg.RetentionCount))
	}
	s.backupDestination.Store(cfg.DestinationMode)
	s.backupLocalPath.Store(cfg.LocalPath)
	s.backupScheduleMode.Store(cfg.ScheduleMode)
	s.backupScheduleHour.Store(int64(cfg.ScheduleHour))
	s.backupScheduleMin.Store(int64(cfg.ScheduleMinute))
	if cfg.EncryptEnabled {
		s.backupEncrypt.Store(1)
	} else {
		s.backupEncrypt.Store(0)
	}
	s.backupKeyRef.Store(cfg.EncryptionKeyRef)
	s.backupUploadTimeout.Store(int64(cfg.UploadTimeoutSec))
	s.backupMaxUploadMB.Store(int64(cfg.MaxUploadSizeMB))
	s.requestBackupReschedule()
}

func (s *Server) requestBackupReschedule() {
	select {
	case s.backupRescheduleCh <- struct{}{}:
	default:
	}
}

func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func nextBackupDelay(now time.Time, enabled bool, mode string, intervalHours int, hour int, minute int) time.Duration {
	if !enabled {
		return time.Hour
	}
	if hour < 0 {
		hour = 0
	}
	if hour > 23 {
		hour = 23
	}
	if minute < 0 {
		minute = 0
	}
	if minute > 59 {
		minute = 59
	}
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	switch mode {
	case "daily":
		if now.Before(target) {
			return target.Sub(now)
		}
		return target.Add(24 * time.Hour).Sub(now)
	case "weekly":
		if now.Before(target) {
			return target.Sub(now)
		}
		return target.Add(7 * 24 * time.Hour).Sub(now)
	case "biweekly":
		if now.Before(target) {
			return target.Sub(now)
		}
		return target.Add(14 * 24 * time.Hour).Sub(now)
	default:
		if intervalHours < 1 {
			intervalHours = 24
		}
		return time.Duration(intervalHours) * time.Hour
	}
}

func (s *Server) currentBackupDelay(now time.Time) time.Duration {
	mode, _ := s.backupScheduleMode.Load().(string)
	return nextBackupDelay(
		now,
		s.backupEnabled.Load() == 1,
		mode,
		int(s.backupIntervalHours.Load()),
		int(s.backupScheduleHour.Load()),
		int(s.backupScheduleMin.Load()),
	)
}

// startBackupJob runs automated backups at the configured interval.
func (s *Server) startBackupJob() {
	go func() {
		timer := time.NewTimer(s.currentBackupDelay(time.Now()))
		defer timer.Stop()
		for {
			select {
			case <-s.backupRescheduleCh:
				delay := s.currentBackupDelay(time.Now())
				log.Printf("backup: schedule reloaded, next automated backup in %v", delay.Round(time.Minute))
				resetTimer(timer, delay)
				continue
			case <-timer.C:
			}

			if s.backupEnabled.Load() == 0 {
				log.Printf("backup: disabled, rechecking in 1h")
				resetTimer(timer, 1*time.Hour)
				continue
			}

			log.Printf("backup: running automated backup")
			err := s.performAutomatedBackup()
			s.recordBackupOutcome(err)

			// Schedule next run
			delay := s.currentBackupDelay(time.Now())
			log.Printf("backup: next automated backup scheduled in %v", delay.Round(time.Minute))
			resetTimer(timer, delay)
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

	retentionCount := int(s.backupRetention.Load())
	if retentionCount < 1 {
		retentionCount = 7
	}
	destinationMode, _ := s.backupDestination.Load().(string)
	if destinationMode == "" {
		destinationMode = "local_only"
	}
	backupPathOverride, _ := s.backupLocalPath.Load().(string)
	keyRef, _ := s.backupKeyRef.Load().(string)

	// 1. Create local backup file
	backupDir := filepath.Join(filepath.Dir(s.cfg.DBPath), "backups")
	if strings.TrimSpace(backupPathOverride) != "" {
		backupDir = filepath.Clean(backupPathOverride)
	}
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
	if s.backupEncrypt.Load() == 1 {
		key, err := backupKeyFromRef(keyRef)
		if err != nil {
			return fmt.Errorf("backup encryption key error: %w", err)
		}
		encPath := backupPath + ".enc"
		if err := encryptFileAESGCM(backupPath, encPath, key); err != nil {
			return fmt.Errorf("backup encryption failed: %w", err)
		}
		_ = os.Remove(backupPath)
		backupPath = encPath
		log.Printf("backup: encrypted backup created at %s", backupPath)
	}

	// Always enforce local retention after creating a backup, even if remote upload fails.
	defer func() {
		files, err := os.ReadDir(backupDir)
		if err != nil {
			log.Printf("backup: retention read failed: %v", err)
			return
		}
		if len(files) <= retentionCount {
			return
		}
		for i := 0; i < len(files)-retentionCount; i++ {
			path := filepath.Join(backupDir, files[i].Name())
			if err := os.Remove(path); err != nil {
				log.Printf("backup: retention remove failed for %s: %v", path, err)
			}
		}
	}()

	if destinationMode == "local_only" {
		log.Printf("backup: destination=%s, skipping remote upload", destinationMode)
		return nil
	}
	maxUploadMB := s.backupMaxUploadMB.Load()
	if maxUploadMB < 1 {
		maxUploadMB = 50
	}
	if info, err := os.Stat(backupPath); err == nil {
		if info.Size() > maxUploadMB*1024*1024 {
			return fmt.Errorf("backup artifact exceeds max upload size (%d MB)", maxUploadMB)
		}
	}

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

	timeoutSec := s.backupUploadTimeout.Load()
	if timeoutSec < 5 {
		timeoutSec = 30
	}
	sender := notify.NewTelegramSenderWithTimeout(botToken, chatID, time.Duration(timeoutSec)*time.Second)
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
	mux.Handle("POST /api/backup/config", s.requireAuth(http.HandlerFunc(s.handleBackupConfigSave)))
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
