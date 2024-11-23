package config

import (
	"strings"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const DefaultLogLevel = "info"

type Config struct {
	Interval    int    `mapstructure:"interval"`
	Temperature int    `mapstructure:"temperature"`
	FanSpeed    int    `mapstructure:"fanspeed"`
	Hysteresis  int    `mapstructure:"hysteresis"`
	Performance bool   `mapstructure:"performance"`
	Monitor     bool   `mapstructure:"monitor"`
	LogLevel    string `mapstructure:"log_level"`
	Metrics     bool   `mapstructure:"metrics"`
	MetricsDB   string `mapstructure:"database"`
}

func Load() (*Config, error) {
	v := viper.New()

	setDefaults(v)
	defineFlags(v)

	if err := bindFlags(v); err != nil {
		return nil, err
	}

	if err := loadConfigFile(v); err != nil {
		return nil, err
	}

	bindEnvVariables(v)

	cfg := createConfig(v)
	setLogLevel(cfg)

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("interval", 2)
	v.SetDefault("temperature", 80)
	v.SetDefault("fanspeed", 100)
	v.SetDefault("hysteresis", 4)
	v.SetDefault("performance", false)
	v.SetDefault("monitor", false)
	v.SetDefault("log_level", DefaultLogLevel)
	v.SetDefault("metrics", false)
	v.SetDefault("database", "/var/lib/nvidiactl/metrics.db")

	// Register all config keys that might be in the config file
	// v.RegisterAlias("fan_speed", "fanspeed")
	// v.RegisterAlias("fan-speed", "fanspeed")
}

func bindFlags(v *viper.Viper) error {
	errFactory := errors.New()
	flags := map[string]string{
		"config":      "config",
		"log_level":   "log-level",
		"interval":    "interval",
		"temperature": "temperature",
		"fanspeed":    "fanspeed",
		"hysteresis":  "hysteresis",
		"performance": "performance",
		"monitor":     "monitor",
		"metrics":     "metrics",
		"database":    "database",
	}

	for configKey, flagName := range flags {
		if err := v.BindPFlag(configKey, pflag.Lookup(flagName)); err != nil {
			return errFactory.Wrap(errors.ErrBindFlags, err)
		}
	}

	return nil
}

func loadConfigFile(v *viper.Viper) error {
	errFactory := errors.New()
	v.SetConfigName("nvidiactl.conf")
	v.SetConfigType("toml")

	v.AddConfigPath("/etc")
	v.AddConfigPath(".")

	configFile := v.GetString("config")
	if configFile != "" {
		v.SetConfigFile(configFile)
	}

	// Check explicit config file from flag/env
	configFile = v.GetString("config")
	if configFile != "" {
		logger.Debug().Str("configFile", configFile).Msg("Using explicit config file path")
		v.SetConfigFile(configFile)
	}

	// Try to read config
	err := v.ReadInConfig()
	if err != nil {
		var configFileNotFound viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFound) {
			logger.Debug().Msg("No config file found. Using defaults and flags.")
			return nil
		}
		logger.Debug().
			Err(err).
			Str("configFile", configFile).
			Str("searchPaths", "/etc,./").
			Msg("Error reading config file")

		return errFactory.Wrap(errors.ErrReadConfig, err)
	}

	logger.Debug().
		Str("file", v.ConfigFileUsed()).
		Msg("Config file loaded")

	logger.Debug().
		Interface("config", v.AllSettings()).
		Msg("Loaded config values")

	return nil
}

func bindEnvVariables(v *viper.Viper) {
	v.SetEnvPrefix("NVIDIACTL")
	v.AutomaticEnv()

	// Replace dots with underscores in env vars
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
}

func createConfig(v *viper.Viper) *Config {
	return &Config{
		Interval:    v.GetInt("interval"),
		Temperature: v.GetInt("temperature"),
		FanSpeed:    v.GetInt("fanspeed"),
		Hysteresis:  v.GetInt("hysteresis"),
		Performance: v.GetBool("performance"),
		Monitor:     v.GetBool("monitor"),
		LogLevel:    v.GetString("log_level"),
		Metrics:     v.GetBool("metrics"),
		MetricsDB:   v.GetString("database"),
	}
}

func defineFlags(v *viper.Viper) {
	// Define all flags
	pflag.String("config", "", "Path to config file")
	pflag.String("log-level", v.GetString("log_level"), "Log level (debug, info, warning, error)")
	pflag.Int("interval", v.GetInt("interval"), "Interval between updates (in seconds)")
	pflag.Int("temperature", v.GetInt("temperature"), "Maximum allowed temperature (in Celsius)")
	pflag.Int("fanspeed", v.GetInt("fanspeed"), "Maximum allowed fan speed (in percent)")
	pflag.Int("hysteresis", v.GetInt("hysteresis"), "Temperature change required before adjusting fan speed")
	pflag.Bool("performance", v.GetBool("performance"), "Enable performance mode (disable power limit adjustments)")
	pflag.Bool("monitor", v.GetBool("monitor"), "Enable monitor mode (only log, don't change settings)")
	pflag.Bool("metrics", v.GetBool("metrics"), "Enable metrics collection")
	pflag.String("database", v.GetString("database"), "Path to the metrics database file")

	// Parse all flags
	pflag.Parse()
}

func validateConfig(cfg *Config) error {
	errFactory := errors.New()

	if cfg.Interval <= 0 {
		return errFactory.WithData(errors.ErrInvalidInterval, cfg.Interval)
	}

	validLevels := map[string]bool{
		"debug":   true,
		"info":    true,
		"warning": true,
		"error":   true,
	}
	if !validLevels[cfg.LogLevel] {
		return errFactory.WithData(errors.ErrInvalidLogLevel, cfg.LogLevel)
	}

	return nil
}

func setLogLevel(cfg *Config) {
	// If monitor mode is enabled, ensure minimum info level logging
	if cfg.Monitor && cfg.LogLevel == "warning" {
		cfg.LogLevel = "info"
	}
}
