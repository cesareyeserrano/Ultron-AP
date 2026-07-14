package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/alerts"
	"github.com/cesareyeserrano/ultron-ap/internal/auth"
	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/help"
	"github.com/cesareyeserrano/ultron-ap/internal/insights"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices"
	landevicesstore "github.com/cesareyeserrano/ultron-ap/internal/network/landevices/store"
	"github.com/cesareyeserrano/ultron-ap/internal/network/wanmonitor"
	"github.com/cesareyeserrano/ultron-ap/internal/notify"
	"github.com/cesareyeserrano/ultron-ap/internal/privileged"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
	"github.com/cesareyeserrano/ultron-ap/web"
)

// Version and BuildCommit are populated by the linker at build time
// via -ldflags "-X 'github.com/cesareyeserrano/ultron-ap/internal/server.Version=…' -X '…BuildCommit=…'".
// Defaults below are what `go run ./cmd/ultron-ap` (no Makefile) sees;
// the Makefile build / build-arm targets override them with the real
// release tag and `git rev-parse --short HEAD`. (BL-021 / BG-033.)
var (
	Version     = "v1.0.0"
	BuildCommit = "unknown"
)

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
	assetVersion      string                        // content hash of app.css for cache-busting (CSS1)
	startedAt         time.Time

	// sseIntervalNs holds the SSE broadcast interval as nanoseconds (atomic).
	sseIntervalNs atomic.Int64
	// Note: the dashboard chart window/points are NOT server-global — each SSE
	// client carries its own selection (see sseClient) so one viewer's window
	// choice does not leak into other clients' charts (BG-046).

	// backupCfg holds the automated-backup settings as one immutable snapshot
	// swapped atomically (D1). The previous 12 independent atomics could be read
	// half-updated — e.g. a backup running with a new destination but the old
	// key ref — because ApplyBackupConfig wrote them one at a time.
	backupCfg          atomic.Pointer[backupSettings]
	backupRescheduleCh chan struct{}
	privileged         *privileged.Client
	gateway            *gatewayprobe.Probe
	wan                *wanmonitor.Monitor
	landevices         *landevices.Orchestrator
	landevicesStore    *landevicesstore.Store
	insights           *insights.Service
	help               *help.Service

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
		bruteForce: auth.NewPersistentBruteForceTracker(db),
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
	s.backupCfg.Store(&backupSettings{
		enabled:       true,
		intervalHours: 24, // Default 24h
		retention:     7,
		destination:   "local_only",
		localPath:     "",
		scheduleMode:  "interval",
		scheduleHour:  3,
		scheduleMin:   0,
		encrypt:       false,
		keyRef:        "",
		uploadTimeout: 30,
		maxUploadMB:   50,
	})

	s.assetVersion = computeAssetVersion(web.Static, "static")
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
	if s.alertEng != nil {
		s.alertEng.SetTransitionCooldowns(
			time.Duration(cfg.DockerCooldownMin)*time.Minute,
			time.Duration(cfg.SystemdCooldownMin)*time.Minute,
		)
	}
}

// backupSettings is the immutable automated-backup config snapshot held in
// s.backupCfg. It is only ever replaced wholesale (never mutated in place) so
// readers always observe a self-consistent set of values (D1).
type backupSettings struct {
	enabled       bool
	intervalHours int
	retention     int
	destination   string
	localPath     string
	scheduleMode  string
	scheduleHour  int
	scheduleMin   int
	encrypt       bool
	keyRef        string
	uploadTimeout int
	maxUploadMB   int
}

// currentBackupSettings loads the active snapshot, returning safe defaults if
// it has not been initialised yet.
func (s *Server) currentBackupSettings() backupSettings {
	if bs := s.backupCfg.Load(); bs != nil {
		return *bs
	}
	return backupSettings{
		enabled: true, intervalHours: 24, retention: 7,
		destination: "local_only", scheduleMode: "interval",
		scheduleHour: 3, uploadTimeout: 30, maxUploadMB: 50,
	}
}

func (s *Server) ApplyBackupConfig(cfg database.BackupConfig) {
	// Start from the current snapshot so the two "keep old value on invalid
	// input" cases below preserve what was there, then swap the whole struct in
	// one atomic Store.
	next := s.currentBackupSettings()
	next.enabled = cfg.Enabled
	if cfg.IntervalHours >= 1 {
		next.intervalHours = cfg.IntervalHours
	}
	if cfg.RetentionCount >= 1 {
		next.retention = cfg.RetentionCount
	}
	next.destination = cfg.DestinationMode
	next.localPath = cfg.LocalPath
	next.scheduleMode = cfg.ScheduleMode
	next.scheduleHour = cfg.ScheduleHour
	next.scheduleMin = cfg.ScheduleMinute
	next.encrypt = cfg.EncryptEnabled
	next.keyRef = cfg.EncryptionKeyRef
	next.uploadTimeout = cfg.UploadTimeoutSec
	next.maxUploadMB = cfg.MaxUploadSizeMB
	s.backupCfg.Store(&next)
	s.requestBackupReschedule()
}

// computeAssetVersion returns a short content hash of every bundled static
// asset under root, used as the ?v= cache-busting token on the CSS/JS/icon
// links. Deriving it from file content means editing any asset (`make css`, a
// JS tweak) automatically invalidates the cache — no more hand-bumped version
// strings drifting out of sync across templates (CSS1, and the JS links).
// WalkDir visits in lexical order, so the hash is deterministic. Falls back to
// the build commit if the tree can't be read.
func computeAssetVersion(fsys fs.FS, root string) string {
	h := sha256.New()
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		h.Write([]byte(path)) // bind the path so a rename changes the hash
		h.Write([]byte{0})
		h.Write(data)
		return nil
	})
	if err != nil {
		return BuildCommit
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
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
	bs := s.currentBackupSettings()
	return nextBackupDelay(
		now,
		bs.enabled,
		bs.scheduleMode,
		bs.intervalHours,
		bs.scheduleHour,
		bs.scheduleMin,
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

			if !s.currentBackupSettings().enabled {
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

	// Load one consistent snapshot for the whole run (D1): destination, key ref
	// and encrypt flag can no longer be observed half-updated relative to each
	// other.
	bs := s.currentBackupSettings()
	retentionCount := bs.retention
	if retentionCount < 1 {
		retentionCount = 7
	}
	destinationMode := bs.destination
	if destinationMode == "" {
		destinationMode = "local_only"
	}
	backupPathOverride := bs.localPath
	keyRef := bs.keyRef

	// 1. Create local backup file
	backupDir := filepath.Join(filepath.Dir(s.cfg.DBPath), "backups")
	if strings.TrimSpace(backupPathOverride) != "" {
		// M2: re-validate the override at run time, not just at config-save
		// time. A symlink component planted after save could otherwise redirect
		// the plaintext VACUUM INTO outside BackupRoot. ValidateBackupPath
		// re-checks containment and the symlink chain on every run.
		validated, err := database.ValidateBackupPath(backupPathOverride, s.cfg.BackupRoot)
		if err != nil {
			log.Printf("backup: rejecting local path override %q: %v", backupPathOverride, err)
			return fmt.Errorf("backup local path invalid: %w", err)
		}
		backupDir = validated
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

	// If the artefact will be uploaded, fail fast on size before paying
	// the cost of encryption. Streaming encrypt bounds memory but disk
	// I/O for a large DB is still worth skipping when the upload will
	// reject it anyway. Plaintext size is a valid upper bound: V2
	// streaming AEAD adds only ~22 B header + 21 B per 64 KiB chunk.
	willUpload := destinationMode != "local_only"
	if willUpload {
		maxUploadMB := int64(bs.maxUploadMB)
		if maxUploadMB < 1 {
			maxUploadMB = 50
		}
		if info, err := os.Stat(backupPath); err == nil {
			if info.Size() > maxUploadMB*1024*1024 {
				return fmt.Errorf("backup artifact exceeds max upload size (%d MB)", maxUploadMB)
			}
		}
	}

	if bs.encrypt {
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
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			log.Printf("backup: retention read failed: %v", err)
			return
		}
		// M11: only ever delete our own backup artefacts. The previous code
		// deleted the lexicographically-first entries over *all* directory
		// contents, which could remove unrelated files (or the wrong backup
		// when .db and .db.enc names interleave) if LocalPath pointed at a
		// shared directory. Filter to "ultron-*" regular files and delete the
		// oldest by modtime.
		type backupFile struct {
			name    string
			modTime time.Time
		}
		var backups []backupFile
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "ultron-") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			backups = append(backups, backupFile{name: e.Name(), modTime: info.ModTime()})
		}
		if len(backups) <= retentionCount {
			return
		}
		sort.Slice(backups, func(i, j int) bool { return backups[i].modTime.Before(backups[j].modTime) })
		for i := 0; i < len(backups)-retentionCount; i++ {
			path := filepath.Join(backupDir, backups[i].name)
			if err := os.Remove(path); err != nil {
				log.Printf("backup: retention remove failed for %s: %v", path, err)
			}
		}
	}()

	if !willUpload {
		log.Printf("backup: destination=%s, skipping remote upload", destinationMode)
		return nil
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

	timeoutSec := bs.uploadTimeout
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
	mux.HandleFunc("GET /version", s.handleVersion)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)
	// CSP violation reports are emitted by the browser without session
	// cookies or CSRF — they MUST NOT require auth. Body size and
	// content-type are policed inside the handler.
	mux.HandleFunc("POST /api/csp-report", s.handleCSPReport)

	// Static files
	staticFS, _ := fs.Sub(web.Static, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Protected routes (require auth)
	mux.Handle("POST /logout", s.requireAuth(http.HandlerFunc(s.handleLogout)))
	mux.Handle("GET /{$}", s.requireAuth(http.HandlerFunc(s.handleDashboard)))
	mux.Handle("GET /docker", s.requireAuth(http.HandlerFunc(s.handleDockerPage)))
	mux.Handle("GET /services", s.requireAuth(http.HandlerFunc(s.handleServicesPage)))
	mux.Handle("GET /alerts", s.requireAuth(http.HandlerFunc(s.handleAlertsPage)))
	mux.Handle("GET /history", s.requireAuth(http.HandlerFunc(s.handleHistoryPage)))
	mux.Handle("GET /network", s.requireAuth(http.HandlerFunc(s.handleNetworkPage)))
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
	mux.Handle("POST /api/alerts/clear", s.requireAuth(http.HandlerFunc(s.handleAlertsClear)))
	mux.Handle("GET /api/settings/backup", s.requireAuth(http.HandlerFunc(s.handleSettingsBackup)))
	mux.Handle("POST /api/settings/backup/run", s.requireAuth(http.HandlerFunc(s.handleSettingsBackupRun)))
	mux.Handle("GET /api/settings/backups/{name}", s.requireAuth(http.HandlerFunc(s.handleBackupDownload)))
	mux.Handle("GET /api/settings/encryption-key/probe", s.requireAuth(http.HandlerFunc(s.handleEncryptionKeyProbe)))
	// FR-079: more specific than /api/notifications/{channel}, so Go's mux
	// routes it here rather than treating "mute" as a channel name.
	mux.Handle("POST /api/notifications/mute/clear", s.requireAuth(http.HandlerFunc(s.handleMuteClear)))
	mux.Handle("POST /api/notifications/{channel}", s.requireAuth(http.HandlerFunc(s.handleNotificationSave)))
	mux.Handle("POST /api/notifications/{channel}/test", s.requireAuth(http.HandlerFunc(s.handleNotificationTest)))
	mux.Handle("POST /api/performance", s.requireAuth(http.HandlerFunc(s.handlePerformanceSave)))
	mux.Handle("POST /api/backup/config", s.requireAuth(http.HandlerFunc(s.handleBackupConfigSave)))
	mux.Handle("POST /api/settings/hardware", s.requireAuth(http.HandlerFunc(s.handleHardwareSave)))
	mux.Handle("GET /api/services/{name}/logs", s.requireAuth(http.HandlerFunc(s.handleServiceLogs)))
	mux.Handle("POST /api/services/{name}/start", s.requireAuth(http.HandlerFunc(s.handleServiceStart)))
	mux.Handle("POST /api/services/{name}/stop", s.requireAuth(http.HandlerFunc(s.handleServiceStop)))
	mux.Handle("POST /api/services/{name}/restart", s.requireAuth(http.HandlerFunc(s.handleServiceRestart)))
	mux.Handle("GET /api/tailscale/status", s.requireAuth(http.HandlerFunc(s.handleTailscaleStatus)))
	mux.Handle("GET /api/system/logs", s.requireAuth(http.HandlerFunc(s.handleFetchSystemLogs)))
	mux.Handle("POST /api/system/restart", s.requireAuth(http.HandlerFunc(s.handleSystemRestart)))
	mux.Handle("POST /api/system/shutdown", s.requireAuth(http.HandlerFunc(s.handleSystemShutdown)))
	mux.Handle("POST /api/history/clear", s.requireAuth(http.HandlerFunc(s.handleHistoryClear)))

	// LAN devices feature (FR-036, FR-037)
	mux.Handle("GET /api/network/lan-devices", s.requireAuth(http.HandlerFunc(s.handleLANDevicesAPI)))
	mux.Handle("GET /api/network/lan-devices/status", s.requireAuth(http.HandlerFunc(s.handleLANDevicesStatus)))
	mux.Handle("GET /network/lan-devices/fragment", s.requireAuth(http.HandlerFunc(s.handleLANDevicesFragment)))

	// Insights engine (FR-043, FR-044)
	mux.Handle("GET /api/insights/verdicts", s.requireAuth(http.HandlerFunc(s.handleInsightsVerdicts)))
	mux.Handle("GET /insights/fragment", s.requireAuth(http.HandlerFunc(s.handleInsightsFragment)))

	// Help page (FR-048..FR-056). Wrapped in the parent dashboard chrome by
	// handleHelpPage; the embedded help.Service writes the body fragment.
	mux.Handle("GET /help", s.requireAuth(http.HandlerFunc(s.handleHelpPage)))
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

// SetGatewayProbe attaches a gateway ICMP probe whose latest snapshot is
// rendered on the dashboard. Optional — Server works without one.
func (s *Server) SetGatewayProbe(p *gatewayprobe.Probe) {
	s.gateway = p
}

// SetWANMonitor attaches the WAN up/down detector whose snapshot is shown
// as a status badge on the dashboard. Optional.
func (s *Server) SetWANMonitor(m *wanmonitor.Monitor) {
	s.wan = m
}

// SetHelp attaches the help-page service. Required for /help to be reachable;
// when nil the route returns 503.
//
// @aitri-trace FR-048
func (s *Server) SetHelp(h *help.Service) {
	s.help = h
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

	// Release the SSE streams FIRST. http.Server.Shutdown waits for active
	// connections to finish, and an SSE stream only finishes when the browser
	// disconnects — so with a dashboard tab open it always hit its deadline,
	// the process exited 1, and systemd recorded a failed unit on every
	// restart (BG-075).
	if s.sseBroker != nil {
		s.sseBroker.shutdown()
	}

	return s.httpServer.Shutdown(ctx)
}
