package config

import (
	"errors"
	"fmt"

	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Config struct {
	Interval    int
	Temperature int
	FanSpeed    int
	Hysteresis  int
	Performance bool
	Monitor     bool
	Debug       bool
	Verbose     bool
}

var ErrInvalidInterval = errors.New("invalid interval")

func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("interval", 2)
	v.SetDefault("temperature", 80)
	v.SetDefault("fanspeed", 100)
	v.SetDefault("hysteresis", 4)
	v.SetDefault("performance", false)
	v.SetDefault("monitor", false)
	v.SetDefault("debug", false)
	v.SetDefault("verbose", false)

	// Define flags
	pflag.Bool("debug", v.GetBool("debug"), "Enable debugging mode")
	pflag.Bool("verbose", v.GetBool("verbose"), "Enable verbose logging")
	pflag.Int("interval", v.GetInt("interval"), "Interval between updates (in seconds)")
	pflag.Int("temperature", v.GetInt("temperature"), "Maximum allowed temperature (in Celsius)")
	pflag.Int("fanspeed", v.GetInt("fanspeed"), "Maximum allowed fan speed (in percent)")
	pflag.Int("hysteresis", v.GetInt("hysteresis"), "Temperature change required before adjusting fan speed")
	pflag.Bool("performance", v.GetBool("performance"), "Enable performance mode (disable power limit adjustments)")
	pflag.Bool("monitor", v.GetBool("monitor"), "Enable monitor mode (only log, don't change settings)")

	// Parse flags
	pflag.Parse()

	// Bind flags to viper
	if err := v.BindPFlags(pflag.CommandLine); err != nil {
		return nil, fmt.Errorf("failed to bind flags: %w", err)
	}

	// Load configuration from file
	v.SetConfigName("nvidiactl")
	v.SetConfigType("toml")
	v.AddConfigPath("/etc")
	v.AddConfigPath(".")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			logger.Info().Msg("No config file found. Using defaults and flags.")
		} else {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Bind environment variables
	v.SetEnvPrefix("NVIDIACTL")
	v.AutomaticEnv()

	// Create and populate the Config struct
	cfg := &Config{
		Interval:    v.GetInt("interval"),
		Temperature: v.GetInt("temperature"),
		FanSpeed:    v.GetInt("fanspeed"),
		Hysteresis:  v.GetInt("hysteresis"),
		Performance: v.GetBool("performance"),
		Monitor:     v.GetBool("monitor"),
		Debug:       v.GetBool("debug"),
		Verbose:     v.GetBool("verbose"),
	}

	// Validate interval
	if cfg.Interval <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidInterval, cfg.Interval)
	}

	// Set log level based on config
	switch {
	case cfg.Debug:
		logger.SetLogLevel(logger.DebugLevel)
	case cfg.Verbose:
		logger.SetLogLevel(logger.InfoLevel)
	default:
		logger.SetLogLevel(logger.WarnLevel)
	}

	return cfg, nil
}
