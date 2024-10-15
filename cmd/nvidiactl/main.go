package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeberg.org/mutker/nvidiactl/internal/config"
	"codeberg.org/mutker/nvidiactl/internal/gpu"
	"codeberg.org/mutker/nvidiactl/internal/logger"
)

const (
	minTemperature       = 50
	powerLimitWindowSize = 5
	maxPowerLimitChange  = 10
	wattsPerDegree       = 5
)

var (
	cfg            *config.Config
	autoFanControl bool
)

func init() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.Debug, cfg.Verbose, logger.IsService())
	logger.Debug().Msg("Config loaded")

	if err := gpu.Initialize(); err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize GPU")
	}
}

func main() {
	defer gpu.Shutdown()

	if err := gpu.InitializeSettings(cfg.FanSpeed, cfg.Temperature); err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize GPU settings")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleSignals(cancel)

	if err := loop(ctx); err != nil {
		logger.Error().Err(err).Msg("error in main loop")
	}
	cleanup()
}

func loop(ctx context.Context) error {
	if cfg.Interval <= 0 {
		return fmt.Errorf("invalid interval: %d", cfg.Interval)
	}

	interval := time.Duration(cfg.Interval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if cfg.Monitor {
		logger.Info().Msg("Monitor mode activated. Logging GPU status...")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := getGPUStats(); err != nil {
				return err
			}
		}
	}
}

// Restore defaults and exit
func cleanup() {
	_, maxPowerLimit, defaultPowerLimit, err := gpu.GetMinMaxPowerLimits()
	if err != nil {
		logger.Error().Err(err).Msg("failed to get power limits")
	} else {
		powerLimitToSet := min(defaultPowerLimit, maxPowerLimit)
		if err := gpu.SetPowerLimit(powerLimitToSet); err != nil {
			logger.Error().Err(err).Msg("failed to reset power limit")
		}
	}
	if err := gpu.EnableAutoFanControl(); err != nil {
		logger.Error().Err(err).Msg("failed to enable auto fan control")
	}
	logger.Info().Msg("Exiting...")
}

func getGPUStats() error {
	currentTemperature, err := gpu.GetTemperature()
	if err != nil {
		return err
	}
	currentFanSpeeds, err := gpu.GetCurrentFanSpeeds()
	if err != nil {
		return err
	}
	currentPowerLimit, err := gpu.GetCurrentPowerLimit()
	if err != nil {
		return err
	}

	avgTemp := gpu.UpdateTemperatureHistory(currentTemperature)
	avgPowerLimit := gpu.UpdatePowerLimitHistory(currentPowerLimit)

	var targetFanSpeed, targetPowerLimit int
	if !cfg.Monitor {
		targetFanSpeed, targetPowerLimit, err = setGPUParameters(avgTemp, currentFanSpeeds[0], currentPowerLimit)
		if err != nil {
			return err
		}
	}

	getTelemetry(currentTemperature, avgTemp, currentFanSpeeds[0], targetFanSpeed, currentPowerLimit, targetPowerLimit, avgPowerLimit)
	return nil
}

func setGPUParameters(avgTemp, currentFanSpeed, currentPowerLimit int) (targetFanSpeed, targetPowerLimit int, err error) {
	targetFanSpeed = calculateFanSpeed(avgTemp, cfg.Temperature, cfg.FanSpeed)
	targetPowerLimit = calculatePowerLimit(avgTemp, cfg.Temperature, currentFanSpeed, cfg.FanSpeed, currentPowerLimit)

	if avgTemp <= minTemperature {
		if !autoFanControl {
			if err := gpu.EnableAutoFanControl(); err != nil {
				return 0, 0, err
			}
			autoFanControl = true
		}
	} else {
		if autoFanControl {
			logger.Debug().Msgf("Temperature (%d°C) above minimum (%d°C). Switching to manual fan control.", avgTemp, minTemperature)
			autoFanControl = false
		}

		if !autoFanControl && !applyHysteresis(targetFanSpeed, currentFanSpeed, cfg.Hysteresis) {
			if err := gpu.SetFanSpeed(targetFanSpeed); err != nil {
				return 0, 0, err
			}
			logger.Debug().Msgf("Fan speed changed from %d to %d", currentFanSpeed, targetFanSpeed)
		}
	}

	if !cfg.Performance {
		if !applyHysteresis(targetPowerLimit, currentPowerLimit, 5) {
			if err := gpu.SetPowerLimit(targetPowerLimit); err != nil {
				return 0, 0, err
			}
			logger.Debug().Msgf("Power limit changed from %d to %d", currentPowerLimit, targetPowerLimit)
		}
	} else {
		maxPowerLimit, err := gpu.GetMaxPowerLimit()
		if err != nil {
			return 0, 0, err
		}
		if currentPowerLimit < maxPowerLimit {
			if err := gpu.SetPowerLimit(maxPowerLimit); err != nil {
				return 0, 0, err
			}
			logger.Debug().Msgf("Power limit set to max: %d", maxPowerLimit)
			targetPowerLimit = maxPowerLimit
		}
	}

	return targetFanSpeed, targetPowerLimit, nil
}

func handleSignals(cancel context.CancelFunc) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	logger.Info().Msg("Received termination signal.")
	cancel()
}

// Logging functions
func getTelemetry(currentTemperature, averageTemperature, currentFanSpeed, targetFanSpeed, currentPowerLimit, targetPowerLimit, averagePowerLimit int) {
	if cfg.Debug {
		lastFanSpeeds, _ := gpu.GetLastFanSpeeds()
		minPowerLimit, _ := gpu.GetMinPowerLimit()
		maxPowerLimit, _ := gpu.GetMaxPowerLimit()
		minFanSpeedLimit, _ := gpu.GetMinFanSpeedLimit()
		maxFanSpeedLimit, _ := gpu.GetMaxFanSpeedLimit()

		logger.Debug().
			Int("current_fan_speed", currentFanSpeed).
			Int("target_fan_speed", targetFanSpeed).
			Interface("last_set_fan_speeds", lastFanSpeeds).
			Int("max_fan_speed", cfg.FanSpeed).
			Int("current_temperature", currentTemperature).
			Int("average_temperature", averageTemperature).
			Int("min_temperature", minTemperature).
			Int("max_temperature", cfg.Temperature).
			Int("current_power_limit", currentPowerLimit).
			Int("target_power_limit", targetPowerLimit).
			Int("average_power_limit", averagePowerLimit).
			Int("min_power_limit", minPowerLimit).
			Int("max_power_limit", maxPowerLimit).
			Int("min_fan_speed", minFanSpeedLimit).
			Int("max_fan_speed", maxFanSpeedLimit).
			Int("hysteresis", cfg.Hysteresis).
			Bool("monitor", cfg.Monitor).
			Bool("performance", cfg.Performance).
			Bool("auto_fan_control", autoFanControl).
			Msg("")
	} else if cfg.Verbose || cfg.Monitor {
		logger.Info().
			Int("fan_speed", currentFanSpeed).
			Int("target_fan_speed", targetFanSpeed).
			Int("temperature", currentTemperature).
			Int("avg_temperature", averageTemperature).
			Int("power_limit", currentPowerLimit).
			Int("target_power_limit", targetPowerLimit).
			Int("avg_power_limit", averagePowerLimit).
			Msg("")
	}
}

func calculateFanSpeed(averageTemperature, maxTemperature, configMaxFanSpeed int) int {
	minFanSpeed, _ := gpu.GetMinFanSpeedLimit()
	maxFanSpeed, _ := gpu.GetMaxFanSpeedLimit()

	maxFanSpeed = min(maxFanSpeed, configMaxFanSpeed)

	if averageTemperature <= minTemperature {
		return minFanSpeed
	}

	if averageTemperature >= maxTemperature {
		return maxFanSpeed
	}

	tempRange := float64(maxTemperature - minTemperature)
	tempPercentage := float64(averageTemperature-minTemperature) / tempRange

	fanSpeedPercentage := calculateFanSpeedPercentage(tempPercentage)
	fanSpeedRange := maxFanSpeed - minFanSpeed
	targetFanSpeed := int(float64(fanSpeedRange)*fanSpeedPercentage) + minFanSpeed

	return clamp(targetFanSpeed, minFanSpeed, maxFanSpeed)
}

func calculateFanSpeedPercentage(tempPercentage float64) float64 {
	if cfg.Performance {
		return math.Pow(tempPercentage, 1.5)
	}
	return math.Pow(tempPercentage, 2)
}

func calculatePowerLimit(currentTemperature, targetTemperature, currentFanSpeed, maxFanSpeed, currentPowerLimit int) int {
	minPowerLimit, maxPowerLimit, _, _ := gpu.GetMinMaxPowerLimits()

	tempDiff := currentTemperature - targetTemperature
	if tempDiff > 0 && currentFanSpeed >= maxFanSpeed {
		adjustment := min(tempDiff*wattsPerDegree, maxPowerLimitChange)
		return clamp(currentPowerLimit-adjustment, minPowerLimit, maxPowerLimit)
	}

	if tempDiff < 0 {
		adjustment := min(-tempDiff*wattsPerDegree, maxPowerLimitChange)
		return clamp(currentPowerLimit+adjustment, minPowerLimit, maxPowerLimit)
	}

	return currentPowerLimit
}

func applyHysteresis(newSpeed, currentSpeed, hysteresis int) bool {
	return abs(newSpeed-currentSpeed) <= hysteresis
}

// Helper functions
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
