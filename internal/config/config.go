package config

import (
	"context"
	"strings"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const DefaultLogLevel = LogLevelInfo

// viperConfig implements Provider interface using viper
type viperConfig struct {
	v *viper.Viper
}

// defaultLoader implements Loader interface
type defaultLoader struct {
	v *viper.Viper
}

// NewLoader creates a new configuration loader
func NewLoader() Loader {
	v := viper.New()
	return &defaultLoader{v: v}
}

func (l *defaultLoader) Load(_ context.Context, opts ...Option) (Provider, error) {
	errFactory := errors.New()

	// Apply options
	o := &options{
		envPrefix: "NVIDIACTL",
	}
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, errFactory.Wrap(errors.ErrLoadConfig, err)
		}
	}

	setDefaults(l.v)
	defineFlags(l.v)

	if err := bindFlags(l.v); err != nil {
		return nil, err
	}

	if err := loadConfigFile(l.v, o.configPath); err != nil {
		return nil, err
	}

	bindEnvVariables(l.v, o.envPrefix)

	if err := l.Validate(); err != nil {
		return nil, err
	}

	return &viperConfig{v: l.v}, nil
}

func (l *defaultLoader) Validate() error {
	errFactory := errors.New()

	if l.v.GetInt("interval") <= 0 {
		return errFactory.WithData(errors.ErrInvalidInterval, l.v.GetInt("interval"))
	}

	logLevel := LogLevel(l.v.GetString("log_level"))
	if !logLevel.IsValid() {
		return errFactory.WithData(errors.ErrInvalidLogLevel, logLevel)
	}

	return nil
}

// Provider interface implementation
func (c *viperConfig) GetInterval() int {
	return c.v.GetInt("interval")
}

func (c *viperConfig) GetTemperature() int {
	return c.v.GetInt("temperature")
}

func (c *viperConfig) GetFanSpeed() int {
	return c.v.GetInt("fanspeed")
}

func (c *viperConfig) GetHysteresis() int {
	return c.v.GetInt("hysteresis")
}

func (c *viperConfig) IsPerformanceMode() bool {
	return c.v.GetBool("performance")
}

func (c *viperConfig) IsMonitorMode() bool {
	return c.v.GetBool("monitor")
}

func (c *viperConfig) GetLogLevel() string {
	return c.v.GetString("log_level")
}

func (c *viperConfig) IsMetricsEnabled() bool {
	return c.v.GetBool("metrics")
}

func (c *viperConfig) GetMetricsDBPath() string {
	return c.v.GetString("database")
}

// Internal helper functions
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
}

func defineFlags(v *viper.Viper) {
	pflag.String("config", "", "path to config file")
	pflag.String("log-level", v.GetString("log_level"), "log level (debug, info, warning, error)")
	pflag.Int("interval", v.GetInt("interval"), "interval between updates in seconds")
	pflag.Int("temperature", v.GetInt("temperature"), "maximum allowed temperature in Celsius")
	pflag.Int("fanspeed", v.GetInt("fanspeed"), "maximum allowed fan speed in percent")
	pflag.Int("hysteresis", v.GetInt("hysteresis"), "temperature change required before adjusting fan speed")
	pflag.Bool("performance", v.GetBool("performance"), "enable performance mode")
	pflag.Bool("monitor", v.GetBool("monitor"), "enable monitor mode")
	pflag.Bool("metrics", v.GetBool("metrics"), "enable metrics collection")
	pflag.String("database", v.GetString("database"), "path to the metrics database file")

	pflag.Parse()
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

func loadConfigFile(v *viper.Viper, configPath string) error {
	errFactory := errors.New()

	v.SetConfigName("nvidiactl.conf")
	v.SetConfigType("toml")

	v.AddConfigPath("/etc")
	v.AddConfigPath(".")

	if configPath != "" {
		v.SetConfigFile(configPath)
	}

	err := v.ReadInConfig()
	if err != nil {
		var configFileNotFound viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFound) {
			logger.Debug().Msg("no config file found, using defaults and flags")
			return nil
		}
		logger.Debug().
			Err(err).
			Str("configPath", configPath).
			Str("searchPaths", "/etc,./").
			Msg("error reading config file")

		return errFactory.Wrap(errors.ErrLoadConfig, err)
	}

	logger.Debug().
		Str("file", v.ConfigFileUsed()).
		Msg("config file loaded")

	return nil
}

func bindEnvVariables(v *viper.Viper, prefix string) {
	v.SetEnvPrefix(prefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
}
