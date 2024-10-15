package config

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var configExample = []byte(`
interval = 5
temperature = 75
fanspeed = 80
hysteresis = 3
performance = true
monitor = false
debug = true
verbose = false
`)

func TestLoad(t *testing.T) {
	// Create a new Viper instance
	v := viper.New()
	v.SetConfigType("toml")

	// Read the config
	err := v.ReadConfig(bytes.NewBuffer(configExample))
	require.NoError(t, err, "Failed to read config")

	// Unmarshal the config
	var cfg Config
	err = v.Unmarshal(&cfg)
	require.NoError(t, err, "Failed to unmarshal config")

	// Assert
	assert.Equal(t, 5, cfg.Interval, "Expected Interval 5")
	assert.Equal(t, 75, cfg.Temperature, "Expected Temperature 75")
	assert.Equal(t, 80, cfg.FanSpeed, "Expected FanSpeed 80")
	assert.Equal(t, 3, cfg.Hysteresis, "Expected Hysteresis 3")
	assert.True(t, cfg.Performance, "Expected Performance true")
	assert.False(t, cfg.Monitor, "Expected Monitor false")
	assert.True(t, cfg.Debug, "Expected Debug true")
	assert.False(t, cfg.Verbose, "Expected Verbose false")

	// Test that the values are in the config
	assert.True(t, v.InConfig("interval"))
	assert.True(t, v.InConfig("temperature"))
	assert.True(t, v.InConfig("fanspeed"))
	assert.True(t, v.InConfig("performance"))
	assert.False(t, v.InConfig("nonexistent"))
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	// Assert default values
	assert.Equal(t, 2, cfg.Interval, "Expected default Interval 2")
	assert.Equal(t, 80, cfg.Temperature, "Expected default Temperature 80")
	assert.Equal(t, 100, cfg.FanSpeed, "Expected default FanSpeed 100")
	assert.Equal(t, 4, cfg.Hysteresis, "Expected default Hysteresis 4")
	assert.False(t, cfg.Performance, "Expected default Performance false")
	assert.False(t, cfg.Monitor, "Expected default Monitor false")
	assert.False(t, cfg.Debug, "Expected default Debug false")
	assert.False(t, cfg.Verbose, "Expected default Verbose false")
}
