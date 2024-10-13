package main

import (
	"context"
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
		logger.Fatal().Err(err).Msg("Failed to load config")
	}

	logger.Init(cfg.Debug, cfg.Verbose, logger.IsService())

	if err := gpu.Initialize(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize GPU")
	}
}

func main() {
	defer gpu.Shutdown()

	if err := gpu.InitializeSettings(cfg.FanSpeed, cfg.Temperature); err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize GPU settings")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleSignals(cancel)
	loop(ctx)
	cleanup()
}

func cleanup() {
	_, maxPowerLimit, defaultPowerLimit, err := gpu.GetMinMaxPowerLimits()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get power limits")
	} else {
		// Set power limit to the default, but don't exceed the max power limit
		powerLimitToSet := min(defaultPowerLimit, maxPowerLimit)
		if err := gpu.SetPowerLimit(powerLimitToSet); err != nil {
			logger.Error().Err(err).Msg("Failed to reset power limit")
		}
	}
	if err := gpu.EnableAutoFanControl(); err != nil {
		logger.Error().Err(err).Msg("Failed to enable auto fan control")
	}
	logger.Info().Msg("Exiting...")
}

func loop(ctx context.Context) {
	ticker := time.NewTicker(time.Second * time.Duration(cfg.Interval))
	defer ticker.Stop()

	if cfg.Monitor {
		logger.Init(false, true, logger.IsService())
		logger.Info().Msg("Monitor mode activated. Logging GPU status...")
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentTemperature, _ := gpu.GetTemperature()
			currentFanSpeeds, _ := gpu.GetCurrentFanSpeeds()
			currentPowerLimit, _ := gpu.GetCurrentPowerLimit()

			avgTemp := gpu.UpdateTemperatureHistory(currentTemperature)
			avgPowerLimit := gpu.UpdatePowerLimitHistory(currentPowerLimit)

			if !cfg.Monitor {
				targetFanSpeed := calculateFanSpeed(avgTemp, cfg.Temperature, cfg.FanSpeed)
				targetPowerLimit := calculatePowerLimit(avgTemp, cfg.Temperature, currentFanSpeeds[0], cfg.FanSpeed, currentPowerLimit)

				if avgTemp <= minTemperature {
					if !autoFanControl {
						if err := gpu.EnableAutoFanControl(); err != nil {
							logger.Error().Err(err).Msg("Failed to enable auto fan control")
						}
						autoFanControl = true
					}
				} else {
					if autoFanControl {
						logger.Debug().Msgf("Temperature (%d°C) above minimum (%d°C). Switching to manual fan control.", avgTemp, minTemperature)
						autoFanControl = false
					}

					if !autoFanControl {
						if targetFanSpeed != currentFanSpeeds[0] && !applyHysteresis(targetFanSpeed, currentFanSpeeds[0], cfg.Hysteresis) {
							if err := gpu.SetFanSpeed(targetFanSpeed); err != nil {
								logger.Error().Err(err).Msg("Failed to set fan speeds")
							} else {
								logger.Debug().Msgf("Fan speed changed from %d to %d", currentFanSpeeds[0], targetFanSpeed)
							}
						}
					}
				}

				if !cfg.PerformanceMode {
					lastPowerLimit, _ := gpu.GetLastPowerLimit()
					gradualPowerLimit := getGradualPowerLimit(avgPowerLimit, lastPowerLimit)

					if gradualPowerLimit != currentPowerLimit {
						if err := gpu.SetPowerLimit(gradualPowerLimit); err != nil {
							logger.Error().Err(err).Msg("Failed to set power limit")
						} else {
							logger.Debug().Msgf("Power limit changed from %d to %d", currentPowerLimit, gradualPowerLimit)
						}
					}
				} else {
					maxPowerLimit, _ := gpu.GetMaxPowerLimit()
					if currentPowerLimit < maxPowerLimit {
						if err := gpu.SetPowerLimit(maxPowerLimit); err != nil {
							logger.Error().Err(err).Msg("Failed to set max power limit")
						} else {
							logger.Debug().Msgf("Power limit set to max: %d", maxPowerLimit)
						}
					}
				}

				logStatus(currentTemperature, avgTemp, currentFanSpeeds[0], targetFanSpeed, currentPowerLimit, targetPowerLimit, avgPowerLimit)
			} else {
				logStatus(currentTemperature, avgTemp, currentFanSpeeds[0], 0, currentPowerLimit, 0, avgPowerLimit)
			}
		}
	}
}

func handleSignals(cancel context.CancelFunc) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	logger.Info().Msg("Received termination signal.")
	cancel()
}

// Logging functions
func logStatus(currentTemperature, averageTemperature, currentFanSpeed, targetFanSpeed, currentPowerLimit, targetPowerLimit, averagePowerLimit int) {
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
			Bool("performance", cfg.PerformanceMode).
			Bool("auto_fan_control", autoFanControl).
			Msg("")
	} else if cfg.Verbose || cfg.Monitor {
		logger.Info().
			Int("fan_speed", currentFanSpeed).
			Int("temperature", currentTemperature).
			Int("avg_temperature", averageTemperature).
			Int("power_limit", currentPowerLimit).
			Int("avg_power_limit", averagePowerLimit).
			Msg("")
	}
}

func calculateFanSpeed(averageTemperature, maxTemperature, configMaxFanSpeed int) int {
	minFanSpeed, _ := gpu.GetMinFanSpeedLimit()
	maxFanSpeed, _ := gpu.GetMaxFanSpeedLimit()

	// Ensure configMaxFanSpeed doesn't exceed the hardware's maximum fan speed
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
	if cfg.PerformanceMode {
		return math.Pow(tempPercentage, 1.5)
	}
	return math.Pow(tempPercentage, 2)
}

func calculatePowerLimit(currentTemperature, targetTemperature, currentFanSpeed, maxFanSpeed, currentPowerLimit int) int {
	if cfg.PerformanceMode {
		_, maxPowerLimit, _, _ := gpu.GetMinMaxPowerLimits()
		return maxPowerLimit
	}

	minPowerLimit, maxPowerLimit, _, _ := gpu.GetMinMaxPowerLimits()

	if currentTemperature > targetTemperature && currentFanSpeed >= maxFanSpeed {
		tempDiff := currentTemperature - targetTemperature
		adjustment := min(tempDiff*wattsPerDegree, maxPowerLimitChange)
		newPowerLimit := currentPowerLimit - adjustment
		return clamp(newPowerLimit, minPowerLimit, maxPowerLimit)
	}

	if currentTemperature <= targetTemperature && currentPowerLimit < maxPowerLimit {
		return min(currentPowerLimit+wattsPerDegree, maxPowerLimit)
	}

	return currentPowerLimit
}

func getGradualPowerLimit(targetPowerLimit, currentPowerLimit int) int {
	if targetPowerLimit > currentPowerLimit {
		return min(targetPowerLimit, currentPowerLimit+maxPowerLimitChange)
	} else if targetPowerLimit < currentPowerLimit {
		return max(targetPowerLimit, currentPowerLimit-maxPowerLimitChange)
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
