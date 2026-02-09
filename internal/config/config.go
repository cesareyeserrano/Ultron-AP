package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port     int
	DBPath   string
	LogLevel string
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:     8080,
		DBPath:   "/var/lib/ultron-ap/ultron.db",
		LogLevel: "info",
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

	return cfg, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}
