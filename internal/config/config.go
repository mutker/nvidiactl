package config

import (
	"flag"
	"fmt"

	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/spf13/viper"
)

type Config struct {
	Interval        int
	Temperature     int
	FanSpeed        int
	Hysteresis      int
	PerformanceMode bool
	Monitor         bool
	Debug           bool
	Verbose         bool
	MaxTemperature  int
	MaxFanSpeed     int
}

func Load() (*Config, error) {
	// Set defaults
	cfg := &Config{
		Interval:       2,
		Temperature:    80,
		FanSpeed:       100,
		Hysteresis:     4,
		MaxTemperature: 80,
		MaxFanSpeed:    100,
	}

	// Define flags
	debugFlag := flag.Bool("debug", false, "Enable debugging mode")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")
	flag.IntVar(&cfg.Interval, "interval", cfg.Interval, "Interval between updates")
	flag.IntVar(&cfg.Temperature, "temperature", cfg.Temperature, "Maximum allowed temperature")
	flag.IntVar(&cfg.FanSpeed, "fanspeed", cfg.FanSpeed, "Maximum allowed fan speed")
	flag.IntVar(&cfg.Hysteresis, "hysteresis", cfg.Hysteresis, "Temperature hysteresis")
	flag.BoolVar(&cfg.PerformanceMode, "performance", cfg.PerformanceMode, "Performance mode: Do not adjust power limit")
	flag.BoolVar(&cfg.Monitor, "monitor", cfg.Monitor, "Only monitor temperature and fan speed")

	// Parse flags
	flag.Parse()

	// Apply debug and verbose flags
	cfg.Debug = *debugFlag
	cfg.Verbose = *verboseFlag

	// Load configuration from file
	viper.SetConfigName("nvidiactl.conf")
	viper.SetConfigType("toml")
	viper.AddConfigPath("/etc")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Override config file values with command line flags
	viper.Set("debug", cfg.Debug)
	viper.Set("verbose", cfg.Verbose)
	flag.Visit(func(f *flag.Flag) {
		viper.Set(f.Name, f.Value.String())
	})

	// Unmarshal the configuration
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
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
