package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/cesareyeserrano/ultron-ap/internal/alerts"
	"github.com/cesareyeserrano/ultron-ap/internal/config"
	"github.com/cesareyeserrano/ultron-ap/internal/database"
	"github.com/cesareyeserrano/ultron-ap/internal/docker"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/network/wanmonitor"
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

	// Surface ULTRON_SECRET_KEY misconfiguration in the journal at boot —
	// before any notification config can land in the DB unencrypted (BL-007).
	database.WarnIfMissingSecretKey()

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

	// Start network ICMP probes (unprivileged, requires net.ipv4.ping_group_range
	// on Linux). Snapshots are read by the dashboard; each sample is also
	// persisted to NetSample for the historical chart (FR-021). The WAN monitor
	// observes the same stream to detect WAN_DOWN/WAN_UP transitions (FR-018).
	netTargets := defaultNetTargets()
	publicLabel, gatewayLabel := pickWANLabels(netTargets)
	wan := wanmonitor.New(publicLabel, gatewayLabel, 3, func(e wanmonitor.Event) {
		log.Printf("WAN %s: %s", e.Kind, e.Detail)
		if err := db.InsertNetEvent(database.NetEvent{
			TS:     e.TS,
			Kind:   e.Kind,
			Detail: e.Detail,
		}); err != nil {
			log.Printf("net event insert failed: %v", err)
		}
		// Surface the transition through the existing alert pipeline so it
		// reaches Telegram/Email notifiers (FR-022 MVP slice).
		var severity, message string
		switch e.Kind {
		case "wan_down":
			severity = "critical"
			message = "WAN down — " + e.Detail
		case "wan_up":
			severity = "info"
			message = "WAN recovered — " + e.Detail
		default:
			return
		}
		alert := &database.Alert{
			Severity:  severity,
			Message:   message,
			Source:    "wan",
			CreatedAt: e.TS,
		}
		if err := db.CreateAlert(alert); err != nil {
			log.Printf("wan alert create failed: %v", err)
			return
		}
		dispatcher.Dispatch(alert)
	})

	lastLoggedProbeErr := map[string]string{}
	gateway := gatewayprobe.New(5*time.Second, func(snap gatewayprobe.Snapshot) {
		var rtt *float64
		if snap.Status == gatewayprobe.StatusOK {
			v := snap.RTTMs
			rtt = &v
		}
		kind := string(snap.Kind)
		if kind == "" {
			kind = "icmp"
		}
		if err := db.InsertNetSample(database.NetSample{
			TS:     snap.LastProbe,
			Target: snap.Target,
			Kind:   kind,
			RTTMs:  rtt,
			Status: string(snap.Status),
		}); err != nil {
			log.Printf("net sample insert failed: %v", err)
		}
		// Feed the WAN monitor.
		wan.Observe(snap)
		// Log the first occurrence of a probe error per target and re-log
		// only when the message text changes, so a steady-state failure
		// doesn't spam the journal every 5 seconds.
		if snap.Status != gatewayprobe.StatusOK && snap.LastError != "" && lastLoggedProbeErr[snap.Label] != snap.LastError {
			log.Printf("net probe error (label=%s status=%s target=%s): %s", snap.Label, snap.Status, snap.Target, snap.LastError)
			lastLoggedProbeErr[snap.Label] = snap.LastError
		}
	}, netTargets)
	gateway.Start(context.Background())
	defer gateway.Stop()
	log.Printf("Network probes started (interval=5s, targets=%d, persist=NetSample, wan=%s↔%s)",
		len(netTargets), gatewayLabel, publicLabel)

	// Create server and apply the persisted performance config (SSE interval + any
	// remaining fields not yet applied above).
	srv := server.New(cfg, db, reader, collector, dockerMon, systemdMon, alertEng)
	srv.SetGatewayProbe(gateway)
	srv.SetWANMonitor(wan)
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

// pickWANLabels chooses which target label is the LAN gateway and which is
// the public-internet reference for the WAN monitor. Convention:
// - The first target whose Host is "" (auto-resolved gateway) is the gateway.
// - The next non-empty-Host target is the public reference.
// Falls back to the first two labels in order if the convention is not met.
func pickWANLabels(targets []gatewayprobe.Target) (publicLabel, gatewayLabel string) {
	for _, t := range targets {
		if gatewayLabel == "" && t.Host == "" {
			gatewayLabel = t.Label
			continue
		}
		if publicLabel == "" && t.Host != "" {
			publicLabel = t.Label
		}
	}
	if gatewayLabel == "" && len(targets) > 0 {
		gatewayLabel = targets[0].Label
	}
	if publicLabel == "" && len(targets) > 1 {
		publicLabel = targets[1].Label
	}
	return publicLabel, gatewayLabel
}

// defaultNetTargets returns the network probe target list. Defaults to one
// auto-resolved gateway (ICMP), one public ICMP target (1.1.1.1), and one
// DNS target that resolves cloudflare.com against 1.1.1.1.
//
// ULTRON_NET_TARGETS overrides via comma-separated entries. Each entry is:
//
//	[<kind>:]<label>[=<host>[/<query_name>]]
//
// Examples:
//
//	gateway                                — ICMP, auto-resolve gateway
//	cloudflare=1.1.1.1                     — ICMP to 1.1.1.1
//	dns:cloudflare-dns=1.1.1.1/cloudflare.com — DNS via 1.1.1.1
func defaultNetTargets() []gatewayprobe.Target {
	raw := strings.TrimSpace(os.Getenv("ULTRON_NET_TARGETS"))
	if raw == "" {
		return []gatewayprobe.Target{
			{Label: "gateway", Kind: gatewayprobe.KindICMP},
			{Label: "cloudflare", Host: "1.1.1.1", Kind: gatewayprobe.KindICMP},
			{Label: "dns", Host: "1.1.1.1", Kind: gatewayprobe.KindDNS, QueryName: "cloudflare.com"},
		}
	}
	var out []gatewayprobe.Target
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		t := gatewayprobe.Target{Kind: gatewayprobe.KindICMP}
		// Optional kind prefix: "dns:label=..." or just "label=...".
		if kindPart, rest, found := strings.Cut(part, ":"); found && rest != "" {
			switch strings.TrimSpace(kindPart) {
			case "dns":
				t.Kind = gatewayprobe.KindDNS
				part = rest
			case "icmp":
				t.Kind = gatewayprobe.KindICMP
				part = rest
			}
		}
		label, hostAndQuery, _ := strings.Cut(part, "=")
		t.Label = strings.TrimSpace(label)
		host, query, _ := strings.Cut(hostAndQuery, "/")
		t.Host = strings.TrimSpace(host)
		t.QueryName = strings.TrimSpace(query)
		out = append(out, t)
	}
	return out
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
