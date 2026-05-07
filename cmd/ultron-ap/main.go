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
	"github.com/cesareyeserrano/ultron-ap/internal/help"
	"github.com/cesareyeserrano/ultron-ap/internal/insights"
	insightsstore "github.com/cesareyeserrano/ultron-ap/internal/insights/store"
	"github.com/cesareyeserrano/ultron-ap/internal/metrics"
	"github.com/cesareyeserrano/ultron-ap/internal/network/gatewayprobe"
	"github.com/cesareyeserrano/ultron-ap/internal/network/landevices"
	landevicesstore "github.com/cesareyeserrano/ultron-ap/internal/network/landevices/store"
	"github.com/cesareyeserrano/ultron-ap/internal/network/wanmonitor"
	"github.com/cesareyeserrano/ultron-ap/internal/notify/cause"
	"github.com/cesareyeserrano/ultron-ap/internal/notify"
	"github.com/cesareyeserrano/ultron-ap/internal/server"
	"github.com/cesareyeserrano/ultron-ap/internal/systemd"
)

func main() {
	// First line in the journal so an operator inspecting the running
	// service can confirm what build is on the box without SSHing in to
	// hash the binary. Make build/build-arm inject the real commit via
	// ldflags; an unbuilt-via-make `go run` reports BuildCommit=unknown.
	// (BL-021 / BG-033.)
	log.Printf("Ultron-AP starting: version=%s commit=%s", server.Version, server.BuildCommit)

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

	// Start notification dispatcher. The metrics / systemd / docker
	// monitors are passed in so resource alerts can populate FR-022
	// trend hints, systemd alerts can populate FR-020 surface blocks,
	// and docker alerts can populate FR-021 surface blocks. The cause
	// deriver wires FR-029 probable-cause lines.
	//
	// @aitri-trace FR-020 FR-021 FR-022 FR-029
	dispatcher := notify.NewDispatcher(db)
	dispatcher.SetMetricsReader(collector)
	dispatcher.SetSystemdReader(systemdMon)
	dispatcher.SetDockerReader(dockerMon)
	procSampler := cause.NewProcessSampler()
	procSampler.Start()
	defer procSampler.Stop()
	dispatcher.SetCauseDeriver(cause.New(procSampler))
	dispatcher.Start()
	defer dispatcher.Stop()

	// Start alert engine. Rich callback wires through to DispatchEvent so
	// the renderer receives FirstFiredAt + Rule context.
	//
	// @aitri-trace FR-016 FR-019 FR-024
	alertEng := alerts.NewEngine(db, collector, dockerMon, systemdMon, cfg.MetricsInterval)
	alertEng.SetRichAlertCallback(func(alert *database.Alert, rule *database.AlertConfig, firstFiredAt time.Time) {
		evt := &notify.Event{
			Alert:        alert,
			Rule:         rule,
			Kind:         notify.EventFire,
			Surface:      notify.SurfaceFromSource(alert.Source),
			FirstFiredAt: firstFiredAt,
		}
		dispatcher.DispatchEvent(evt)
	})
	// Resolve callback: build a synthetic Alert (no DB row) so the
	// renderer has Severity/Source for the subject line, and emit a
	// notify.Event{Kind: EventResolve} with the fire-window timestamps.
	//
	// @aitri-trace FR-018 BL-023
	alertEng.SetResolveCallback(func(rule *database.AlertConfig, sourceID, severity string, firstFiredAt, resolvedAt time.Time) {
		alert := &database.Alert{
			Severity:  severity,
			Source:    strings.TrimPrefix(strings.TrimPrefix(sourceID, "metric:"), ""),
			CreatedAt: resolvedAt,
		}
		// metric:cpu → "cpu"; docker:nginx stays as-is so SurfaceFromSource
		// picks the docker surface.
		if strings.HasPrefix(sourceID, "metric:") {
			alert.Source = strings.TrimPrefix(sourceID, "metric:")
		} else {
			alert.Source = sourceID
		}
		if rule != nil {
			alert.ConfigID = &rule.ID
		}
		evt := &notify.Event{
			Alert:        alert,
			Rule:         rule,
			Kind:         notify.EventResolve,
			Surface:      notify.SurfaceFromSource(alert.Source),
			FirstFiredAt: firstFiredAt,
			ResolvedAt:   resolvedAt,
		}
		dispatcher.DispatchEvent(evt)
	})
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
	// LAN devices feature (FR-030..FR-038): periodic ICMP sweep of the local
	// /24, ARP-cache pairing, OUI vendor lookup, persistence + state machine.
	landevicesStore := landevicesstore.New(db.DB)
	landevicesOrch := landevices.New(landevices.Config{
		Store:     landevicesStore,
		RoutePath: "/proc/net/route",
		ArpPath:   "/proc/net/arp",
		Logger:    log.Printf,
	})
	landevicesOrch.Start(context.Background())
	defer landevicesOrch.Stop()
	log.Printf("LAN devices orchestrator started (default cadence=%s)", landevices.BaseCadence)

	// Insights engine (FR-039..FR-047): declarative rules over telemetry.
	// Driven by the SSE broadcast loop on the parent metrics tick — no new
	// ticker, no new datastore, no notify-package coupling (NFR-021).
	insightsStore := insightsstore.New(db.DB)
	insightsSvc := insights.New(insights.Config{
		Store:  insightsStore,
		Logger: log.Printf,
	})
	if err := insightsSvc.LoadBundled(); err != nil {
		log.Printf("insights: load bundled rules: %v", err)
	}
	log.Printf("Insights engine initialised (driven by SSE broadcast tick)")
	defer insightsSvc.Stop()

	// Help page (FR-048..FR-056). Embedded glossary parsed at boot. The links
	// validator runs once after both glossary and rules are loaded — warnings
	// about missing anchors land in the journal so a typo in a rule's links
	// surfaces at the next start, not in production.
	helpSvc, err := help.New(log.Printf)
	if err != nil {
		log.Printf("help: load glossary: %v", err)
	} else {
		helpSvc.ValidateLinks(insightsSvc.RuleLinks())
	}

	srv := server.New(cfg, db, reader, collector, dockerMon, systemdMon, alertEng)
	srv.SetInsights(insightsSvc)
	if helpSvc != nil {
		srv.SetHelp(helpSvc)
	}
	srv.SetGatewayProbe(gateway)
	srv.SetWANMonitor(wan)
	srv.SetLANDevices(landevicesOrch, landevicesStore)
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
