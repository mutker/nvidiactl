package config

import (
	"codeberg.org/mutker/nvidiactl/internal/errors"
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
	Verbose     bool
	Debug       bool
	Telemetry   bool
	TelemetryDB string
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

	if cfg.Monitor && !cfg.Debug {
		cfg.Verbose = true
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	setLogLevel(cfg)

	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("interval", 2)
	v.SetDefault("temperature", 80)
	v.SetDefault("fanspeed", 100)
	v.SetDefault("hysteresis", 4)
	v.SetDefault("performance", false)
	v.SetDefault("monitor", false)
	v.SetDefault("debug", false)
	v.SetDefault("verbose", false)
	v.SetDefault("telemetry", false)
	v.SetDefault("database", "/var/lib/nvidiactl/telemetry.db")
}

func defineFlags(v *viper.Viper) {
	pflag.Bool("debug", v.GetBool("debug"), "Enable debugging mode")
	pflag.Bool("verbose", v.GetBool("verbose"), "Enable verbose logging")
	pflag.Int("interval", v.GetInt("interval"), "Interval between updates (in seconds)")
	pflag.Int("temperature", v.GetInt("temperature"), "Maximum allowed temperature (in Celsius)")
	pflag.Int("fanspeed", v.GetInt("fanspeed"), "Maximum allowed fan speed (in percent)")
	pflag.Int("hysteresis", v.GetInt("hysteresis"), "Temperature change required before adjusting fan speed")
	pflag.Bool("performance", v.GetBool("performance"), "Enable performance mode (disable power limit adjustments)")
	pflag.Bool("monitor", v.GetBool("monitor"), "Enable monitor mode (only log, don't change settings)")
	pflag.Bool("telemetry", v.GetBool("telemetry"), "Enable telemetry collection")
	pflag.String("database", v.GetString("database"), "Path to the telemetry database file")
	pflag.Parse()
}

func bindFlags(v *viper.Viper) error {
	if err := v.BindPFlags(pflag.CommandLine); err != nil {
		return errors.Wrap(errors.ErrBindFlags, err)
	}

	return nil
}

func loadConfigFile(v *viper.Viper) error {
	v.SetConfigName("nvidiactl")
	v.SetConfigType("toml")

	v.AddConfigPath("/etc")
	v.AddConfigPath(".")

	configFile := v.GetString("config")
	if configFile != "" {
		v.SetConfigFile(configFile)
	}

	err := v.ReadInConfig()
	if err != nil {
		logger.Info().Msg("No config file found. Using defaults and flags.")
		return errors.Wrap(errors.ErrReadConfig, err)
	}

	logger.Info().Msgf("Using config file: %s", v.ConfigFileUsed())

	return nil
}

func bindEnvVariables(v *viper.Viper) {
	v.SetEnvPrefix("NVIDIACTL")
	v.AutomaticEnv()
}

func createConfig(v *viper.Viper) *Config {
	return &Config{
		Interval:    v.GetInt("interval"),
		Temperature: v.GetInt("temperature"),
		FanSpeed:    v.GetInt("fanspeed"),
		Hysteresis:  v.GetInt("hysteresis"),
		Performance: v.GetBool("performance"),
		Monitor:     v.GetBool("monitor"),
		Debug:       v.GetBool("debug"),
		Verbose:     v.GetBool("verbose"),
		Telemetry:   v.GetBool("telemetry"),
		TelemetryDB: v.GetString("database"),
	}
}

func validateConfig(cfg *Config) error {
	if cfg.Interval <= 0 {
		return errors.WithData(errors.ErrInvalidInterval, cfg.Interval)
	}

	return nil
}

func setLogLevel(cfg *Config) {
	switch {
	case cfg.Debug:
		logger.SetLogLevel(logger.DebugLevel)
	case cfg.Verbose:
		logger.SetLogLevel(logger.InfoLevel)
	default:
		logger.SetLogLevel(logger.WarnLevel)
	}
}
