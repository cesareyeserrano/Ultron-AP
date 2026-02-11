package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/server"
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

	// Start metrics collector
	reader := metrics.NewSystemReader()
	collector := metrics.NewCollector(reader, cfg.MetricsInterval, 24*time.Hour)
	collector.Start(context.Background())
	defer collector.Stop()

	// Create server
	srv := server.New(cfg, db, collector)

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
