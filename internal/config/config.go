package config

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            int
	DBPath          string
	LogLevel        string
	AdminUser       string
	AdminPass       string
	SessionTTL      time.Duration
	MetricsInterval time.Duration
	HelperSocket    string
	HelperTimeout   time.Duration

	// NetRetentionDays is the retention window for NetSample rows
	// (ULTRON_NET_RETENTION_DAYS, default 30, minimum 1). Without it the table
	// grew unbounded: 8.4M rows and 694 MB — 96% of the database — by
	// 2026-09-06, at ~67k rows and 5.5 MB a day.
	//
	// Sanitised HERE rather than at the point of use, and that placement is the
	// security control: a window of 0 means "delete the entire history" and a
	// negative one puts the cutoff in the future, which deletes everything too.
	// Neither value may ever leave this function. See NFR-102.
	NetRetentionDays int

	// NetInterval is how often the network probe samples
	// (ULTRON_NET_INTERVAL_SECONDS, default 5s, minimum 1s). The default
	// matches the previously hardcoded value exactly, so a deploy that does
	// not touch the environment changes nothing observable (NFR-110).
	NetInterval time.Duration

	// BackupRoot is the only directory under which admin-supplied backup
	// destination paths may live. An empty form value falls back to
	// <BackupRoot>/backups. ULTRON_BACKUP_ROOT overrides; default is the
	// directory containing DBPath, which matches the historical implicit
	// destination.
	BackupRoot string

	// CSPEnforce, when true, sends Content-Security-Policy (enforced)
	// instead of Content-Security-Policy-Report-Only. Both headers
	// always include report-uri /api/csp-report. Default is false: ship
	// reports to /api/csp-report for one release cycle, monitor the
	// log, then flip ULTRON_CSP_ENFORCE=1 once the policy is verified
	// to break nothing. See BL-012 / BG-032.
	CSPEnforce bool

	// TrustedProxies is the parsed list of upstream proxies whose
	// X-Forwarded-For headers we believe. When empty (the default), every
	// XFF value is ignored and the per-IP rate limit always uses the
	// real TCP peer (RemoteAddr). This is the safe default for the Pi
	// deployment — the binary is reached directly via Tailscale, not
	// through a reverse proxy, so any XFF header in production is
	// attacker-controlled. Operators running behind nginx/caddy/etc set
	// ULTRON_TRUSTED_PROXIES=10.0.0.1,192.168.1.0/24 to opt in.
	TrustedProxies []*net.IPNet
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// defaultNetRetentionDays keeps 30 days of network samples: ~2.1M rows and
// ~175 MB at the measured rate, against 719 MB unbounded.
const defaultNetRetentionDays = 30

// minNetRetentionDays is 1, not 0. Zero would delete the whole history on the
// next prune, which is never what an operator means to configure.
const minNetRetentionDays = 1

// defaultNetInterval is the value the probe was hardcoded to before this was
// configurable. Keeping it identical is what makes an unchanged environment a
// no-op (NFR-110).
const defaultNetInterval = 5 * time.Second

// logf is overridable in tests to capture the warnings this file emits,
// matching the seam internal/ups/config.go already uses.
var logf = log.Printf

func Load() (*Config, error) {
	cfg := &Config{
		Port:            8080,
		DBPath:          "/var/lib/ultron-ap/ultron.db",
		LogLevel:        "info",
		AdminUser:       "admin",
		AdminPass:       "",
		SessionTTL:      24 * time.Hour,
		MetricsInterval: 5 * time.Second,
		HelperSocket:    "/run/ultron-helper.sock",
		HelperTimeout:   5 * time.Second,

		NetRetentionDays: defaultNetRetentionDays,
		NetInterval:      defaultNetInterval,
	}

	if v := os.Getenv("ULTRON_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: %w", v, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid port: %d (must be 1-65535)", port)
		}
		cfg.Port = port
	}

	if v := os.Getenv("ULTRON_DB_PATH"); v != "" {
		cfg.DBPath = v
	}

	if v := os.Getenv("ULTRON_LOG_LEVEL"); v != "" {
		level := strings.ToLower(v)
		if !validLogLevels[level] {
			log.Printf("WARNING: invalid log level %q, defaulting to \"info\"", v)
			level = "info"
		}
		cfg.LogLevel = level
	}

	if v := os.Getenv("ULTRON_ADMIN_USER"); v != "" {
		cfg.AdminUser = v
	}

	if v := os.Getenv("ULTRON_ADMIN_PASS"); v != "" {
		cfg.AdminPass = v
	}

	if v := os.Getenv("ULTRON_SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid session TTL %q: %w", v, err)
		}
		// Bound it like the other duration vars. A zero/negative TTL would set
		// every session's ExpiresAt in the past (locking everyone out and
		// turning the cookie MaxAge negative → immediate delete); an absurdly
		// large one creates effectively immortal sessions. (M10)
		if d < time.Minute {
			return nil, fmt.Errorf("session TTL %q too small (minimum 1m)", v)
		}
		if d > 30*24*time.Hour {
			return nil, fmt.Errorf("session TTL %q too large (maximum 720h)", v)
		}
		cfg.SessionTTL = d
	}

	if v := os.Getenv("ULTRON_METRICS_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid metrics interval %q: %w", v, err)
		}
		if d < 1*time.Second {
			return nil, fmt.Errorf("invalid metrics interval: must be >= 1s, got %v", d)
		}
		cfg.MetricsInterval = d
	}

	// ULTRON_NET_RETENTION_DAYS and ULTRON_NET_INTERVAL_SECONDS warn and fall
	// back instead of returning an error, unlike their neighbours above.
	//
	// That divergence is deliberate (ADR-002): a monitoring panel that refuses
	// to boot over a typo in a retention window leaves the operator with no
	// visibility at exactly the moment they need to log in and fix it. The
	// safe default plus a loud journal line is the better failure mode. The
	// existing entries keep their behaviour — changing those would be a
	// regression outside this feature's scope.
	if v := strings.TrimSpace(os.Getenv("ULTRON_NET_RETENTION_DAYS")); v != "" {
		n, err := strconv.Atoi(v)
		switch {
		case err != nil:
			logf("net: invalid ULTRON_NET_RETENTION_DAYS=%q, using default %d", v, defaultNetRetentionDays)
		case n < minNetRetentionDays:
			logf("net: invalid ULTRON_NET_RETENTION_DAYS=%q (minimum %d), using default %d",
				v, minNetRetentionDays, defaultNetRetentionDays)
		default:
			cfg.NetRetentionDays = n
		}
	}

	if v := strings.TrimSpace(os.Getenv("ULTRON_NET_INTERVAL_SECONDS")); v != "" {
		n, err := strconv.Atoi(v)
		switch {
		case err != nil:
			logf("net: invalid ULTRON_NET_INTERVAL_SECONDS=%q, using default %v", v, defaultNetInterval)
		case n < 1:
			logf("net: invalid ULTRON_NET_INTERVAL_SECONDS=%q (minimum 1), using default %v", v, defaultNetInterval)
		default:
			cfg.NetInterval = time.Duration(n) * time.Second
		}
	}

	if v := os.Getenv("ULTRON_HELPER_SOCKET"); v != "" {
		cfg.HelperSocket = strings.TrimSpace(v)
		if cfg.HelperSocket == "" {
			cfg.HelperSocket = "/run/ultron-helper.sock"
		}
	}
	if v := os.Getenv("ULTRON_HELPER_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid helper timeout %q: %w", v, err)
		}
		if d < 1*time.Second {
			return nil, fmt.Errorf("invalid helper timeout: must be >= 1s, got %v", d)
		}
		cfg.HelperTimeout = d
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("ULTRON_CSP_ENFORCE"))) {
	case "1", "true", "yes", "on":
		cfg.CSPEnforce = true
	}

	cfg.BackupRoot = filepath.Clean(filepath.Dir(cfg.DBPath))
	if v := strings.TrimSpace(os.Getenv("ULTRON_BACKUP_ROOT")); v != "" {
		if !filepath.IsAbs(v) {
			return nil, fmt.Errorf("invalid ULTRON_BACKUP_ROOT %q: must be an absolute path", v)
		}
		cfg.BackupRoot = filepath.Clean(v)
	}

	if v := os.Getenv("ULTRON_TRUSTED_PROXIES"); strings.TrimSpace(v) != "" {
		nets, err := parseTrustedProxies(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ULTRON_TRUSTED_PROXIES %q: %w", v, err)
		}
		cfg.TrustedProxies = nets
	}

	return cfg, nil
}

// parseTrustedProxies parses a comma-separated list of IPs and CIDRs into
// *net.IPNet entries. A bare IP is converted to its /32 (IPv4) or /128 (IPv6)
// equivalent so the membership test is uniform.
func parseTrustedProxies(raw string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(part); err == nil {
			// Reject an all-encompassing mask (0.0.0.0/0 or ::/0): trusting
			// every peer turns on X-Forwarded-Proto/For spoofing for anyone
			// (B3). A trusted-proxy allowlist must be specific.
			if ones, _ := n.Mask.Size(); ones == 0 {
				return nil, fmt.Errorf("entry %q trusts all peers; specify the actual proxy address/range", part)
			}
			out = append(out, n)
			continue
		}
		ip := net.ParseIP(part)
		if ip == nil {
			return nil, fmt.Errorf("entry %q is neither an IP nor a CIDR", part)
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return out, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}
