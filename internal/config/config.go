package config

import (
	"fmt"
	"log"
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

	return cfg, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}
