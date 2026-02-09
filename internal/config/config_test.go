package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"ULTRON_PORT", "ULTRON_DB_PATH", "ULTRON_LOG_LEVEL"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "/var/lib/ultron-ap/ultron.db", cfg.DBPath)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoad_CustomPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_PORT", "9090")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Port)
}

func TestLoad_InvalidPortNonNumeric(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_PORT", "abc")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid port")
}

func TestLoad_InvalidPortOutOfRange(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_PORT", "99999")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid port: 99999")
}

func TestLoad_InvalidPortZero(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_PORT", "0")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid port: 0")
}

func TestLoad_CustomDBPath(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_DB_PATH", "/tmp/test.db")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
}

func TestLoad_CustomLogLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_LOG_LEVEL", "debug")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_LOG_LEVEL", "verbose")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoad_LogLevelCaseInsensitive(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_LOG_LEVEL", "DEBUG")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_AllEnvVars(t *testing.T) {
	clearEnv(t)
	t.Setenv("ULTRON_PORT", "3000")
	t.Setenv("ULTRON_DB_PATH", "/data/ultron.db")
	t.Setenv("ULTRON_LOG_LEVEL", "warn")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 3000, cfg.Port)
	assert.Equal(t, "/data/ultron.db", cfg.DBPath)
	assert.Equal(t, "warn", cfg.LogLevel)
}

func TestConfig_Addr(t *testing.T) {
	cfg := &Config{Port: 8080}
	assert.Equal(t, ":8080", cfg.Addr())

	cfg.Port = 9090
	assert.Equal(t, ":9090", cfg.Addr())
}
