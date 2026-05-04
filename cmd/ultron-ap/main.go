package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/cesareyeserrano/ultron-ap/internal/alerts"
	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/notify"
	"github.com/cesareyeserrano/ultron-ap/internal/server"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Log level: %s", cfg.LogLevel)

	// Initialize database
	db, err := database.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Bootstrap admin user on first run
	if err := bootstrapAdmin(cfg, db); err != nil {
		log.Fatalf("Failed to bootstrap admin user: %v", err)
	}

	// Load performance config early so intervals can be applied before monitors start.
	perf, err := db.GetPerformanceConfig()
	if err != nil {
		log.Printf("performance config unavailable, using defaults: %v", err)
		perf = database.DefaultPerformanceConfig()
	}

	// Start metrics collector.
	// Ring buffer: 24 hours at the default 5s interval = 17 280 snapshots.
	// This enables meaningful timeline windows (60m/12h/24h).
	reader := metrics.NewSystemReader()
	reader.SetDiskInterval(time.Duration(perf.DiskIntervalMin) * time.Minute)
	collector := metrics.NewCollector(reader, cfg.MetricsInterval, 24*time.Hour)
	collector.Start(context.Background())
	defer collector.Stop()

	// Start Docker monitor
	dockerMon := docker.NewMonitor()
	dockerMon.SetInterval(time.Duration(perf.DockerIntervalSec) * time.Second)
	dockerMon.Start(context.Background())
	defer dockerMon.Stop()

	// Start Systemd monitor
	systemdMon := systemd.NewMonitor()
	systemdMon.SetInterval(time.Duration(perf.SystemdIntervalSec) * time.Second)
	systemdMon.Start(context.Background())
	defer systemdMon.Stop()

	// Seed default alert rules
	if err := db.SeedDefaultAlertConfigs(); err != nil {
		log.Fatalf("Failed to seed default alert configs: %v", err)
	}

	// Start notification dispatcher
	dispatcher := notify.NewDispatcher(db)
	dispatcher.Start()
	defer dispatcher.Stop()

	// Start alert engine
	alertEng := alerts.NewEngine(db, collector, dockerMon, systemdMon, cfg.MetricsInterval)
	alertEng.SetAlertCallback(dispatcher.Dispatch)
	alertEng.Start(context.Background())
	defer alertEng.Stop()

	// Start gateway ICMP probe (unprivileged, requires net.ipv4.ping_group_range
	// on Linux). Snapshot is read by the dashboard.
	gateway := gatewayprobe.New(5 * time.Second)
	gateway.Start(context.Background())
	defer gateway.Stop()
	log.Printf("Gateway probe started (interval=5s)")

	// Create server and apply the persisted performance config (SSE interval + any
	// remaining fields not yet applied above).
	srv := server.New(cfg, db, reader, collector, dockerMon, systemdMon, alertEng)
	srv.SetGatewayProbe(gateway)
	srv.ApplyPerformanceConfig(perf)
	backupCfg, err := db.GetBackupConfig()
	if err != nil {
		log.Printf("backup config unavailable, using defaults: %v", err)
		backupCfg = database.DefaultBackupConfig()
	}
	srv.ApplyBackupConfig(backupCfg)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for interrupt signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("Received signal: %v", sig)
	case err := <-errCh:
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited cleanly")
}

func bootstrapAdmin(cfg *config.Config, db *database.DB) error {
	count, err := db.UserCount()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	if cfg.AdminPass == "" {
		log.Fatal("ULTRON_ADMIN_PASS is required for initial admin setup")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := db.CreateUser(cfg.AdminUser, string(hash)); err != nil {
		return err
	}

	log.Printf("Admin user %q created", cfg.AdminUser)
	return nil
}
