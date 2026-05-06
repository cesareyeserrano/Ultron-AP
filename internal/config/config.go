package config

import (
	"fmt"
	"log"
	"net"
	"os"
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
