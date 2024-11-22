package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"codeberg.org/mutker/nvidiactl/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	tempDir, err := os.MkdirTemp("", "config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configContent := []byte(`
interval = 5
temperature = 75
fanspeed = 80
hysteresis = 3
performance = true
monitor = false
log_level = "debug"
telemetry = true
database = "/path/to/telemetry.db"
`)
	configPath := filepath.Join(tempDir, "nvidiactl.toml")
	err = os.WriteFile(configPath, configContent, 0o600)
	require.NoError(t, err)

	// Set environment variable to point to the test config file
	t.Setenv("NVIDIACTL_CONFIG", configPath)

	// Load the config
	cfg, err := config.Load()
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 5, cfg.Interval, "Expected Interval 5")
	assert.Equal(t, 75, cfg.Temperature, "Expected Temperature 75")
	assert.Equal(t, 80, cfg.FanSpeed, "Expected FanSpeed 80")
	assert.Equal(t, 3, cfg.Hysteresis, "Expected Hysteresis 3")
	assert.True(t, cfg.Performance, "Expected Performance true")
	assert.False(t, cfg.Monitor, "Expected Monitor false")
	assert.Equal(t, "debug", cfg.LogLevel, "Expected LogLevel debug")
	assert.True(t, cfg.Telemetry, "Expected Telemetry true")
	assert.Equal(t, "/path/to/telemetry.db", cfg.TelemetryDB, "Expected TelemetryDB /path/to/telemetry.db")
}

func TestLoadDefaults(t *testing.T) {
	// Ensure no config file is used
	t.Setenv("NVIDIACTL_CONFIG", "")

	cfg, err := config.Load()
	require.NoError(t, err, "Failed to load config")

	// Assert default values
	assert.Equal(t, 2, cfg.Interval, "Expected default Interval 2")
	assert.Equal(t, 80, cfg.Temperature, "Expected default Temperature 80")
	assert.Equal(t, 100, cfg.FanSpeed, "Expected default FanSpeed 100")
	assert.Equal(t, 4, cfg.Hysteresis, "Expected default Hysteresis 4")
	assert.False(t, cfg.Performance, "Expected default Performance false")
	assert.False(t, cfg.Monitor, "Expected default Monitor false")
	assert.Equal(t, config.DefaultLogLevel, cfg.LogLevel, "Expected default LogLevel info")
}

func TestLoadConfigFileInvalidFormat(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create an invalid test config file
	configContent := []byte(`
This is not a valid TOML file
`)
	configPath := filepath.Join(tempDir, "nvidiactl.toml")
	err = os.WriteFile(configPath, configContent, 0o600)
	require.NoError(t, err)

	// Set environment variable to point to the invalid config file
	t.Setenv("NVIDIACTL_CONFIG", configPath)

	// Try to load the config
	_, err = config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to read config file")
}

func TestInvalidLogLevel(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configContent := []byte(`
log_level = "invalid"
`)
	configPath := filepath.Join(tempDir, "nvidiactl.toml")
	err = os.WriteFile(configPath, configContent, 0o600)
	require.NoError(t, err)

	t.Setenv("NVIDIACTL_CONFIG", configPath)

	_, err = config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_log_level")
}

func TestLogLevelFlag(t *testing.T) {
	// Save original args and restore after test
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set test args
	os.Args = []string{"cmd", "--log-level", "debug"}

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg.LogLevel, "Expected LogLevel to be set by flag")
}
