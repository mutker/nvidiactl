package config

import (
	"flag"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

type Config struct {
	Interval          int
	Temperature       int
	FanSpeed          int
	Hysteresis        int
	PerformanceMode   bool
	Monitor           bool
	Debug             bool
	Verbose           bool
	MaxTemperature    int `default:"80"`
	DefaultInterval   int `default:"2"`
	MaxFanSpeed       int `default:"100"`
	DefaultHysteresis int `default:"4"`
}

func Load() (*Config, error) {
	config := &Config{}

	// Define flags
	debugFlag := flag.Bool("debug", false, "Enable debugging mode")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose logging")
	flag.IntVar(&config.Interval, "interval", config.Interval, "Interval between updates")
	flag.IntVar(&config.Temperature, "temperature", config.Temperature, "Maximum allowed temperature")
	flag.IntVar(&config.FanSpeed, "fanspeed", config.FanSpeed, "Maximum allowed fan speed")
	flag.IntVar(&config.Hysteresis, "hysteresis", config.Hysteresis, "Temperature hysteresis")
	flag.BoolVar(&config.PerformanceMode, "performance", config.PerformanceMode, "Performance mode: Do not adjust power limit")
	flag.BoolVar(&config.Monitor, "monitor", config.Monitor, "Only monitor temperature and fan speed")

	// Parse flags
	flag.Parse()

	// Apply debug and verbose flags
	config.Debug = *debugFlag
	config.Verbose = *verboseFlag

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
	viper.Set("debug", config.Debug)
	viper.Set("verbose", config.Verbose)
	flag.Visit(func(f *flag.Flag) {
		viper.Set(f.Name, f.Value.String())
	})

	// Unmarshal the configuration
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set log level based on config
	if config.Debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else if config.Verbose {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}

	return config, nil
}