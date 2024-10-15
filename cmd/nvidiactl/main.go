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
	powerLimitHysteresis = 5
	performancePowFactor = 1.5
	normalPowFactor      = 2.0
)

type GPUState struct {
	CurrentTemperature int
	AverageTemperature int
	CurrentFanSpeed    int
	TargetFanSpeed     int
	CurrentPowerLimit  int
	TargetPowerLimit   int
	AveragePowerLimit  int
}

var (
	cfg            *config.Config
	autoFanControl bool
	gpuDevice      *gpu.GPU
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

	gpuDevice, err = gpu.New()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize GPU")
	}
}

func main() {
	defer gpuDevice.Shutdown()

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
			state, err := getGPUState()
			if err != nil {
				return err
			}

			if !cfg.Monitor {
				state, err = setGPUState(&state)
				if err != nil {
					return err
				}
			}

			logGPUState(state)
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

func cleanup() {
	powerLimits := gpuDevice.GetPowerLimits()
	powerLimitToSet := min(powerLimits.Default, powerLimits.Max)
	if err := gpuDevice.SetPowerLimit(powerLimitToSet); err != nil {
		logger.Error().Err(err).Msg("failed to reset power limit")
	}
	if err := gpuDevice.EnableAutoFanControl(); err != nil {
		logger.Error().Err(err).Msg("failed to enable auto fan control")
	}
	logger.Info().Msg("Exiting...")
}

func getGPUState() (GPUState, error) {
	currentTemperature, err := gpuDevice.GetTemperature()
	if err != nil {
		return GPUState{}, err
	}

	currentFanSpeeds := gpuDevice.GetCurrentFanSpeeds()
	currentPowerLimit := gpuDevice.GetCurrentPowerLimit()

	avgTemp := gpuDevice.UpdateTemperatureHistory(currentTemperature)
	avgPowerLimit := gpuDevice.UpdatePowerLimitHistory(currentPowerLimit)

	return GPUState{
		CurrentTemperature: currentTemperature,
		AverageTemperature: avgTemp,
		CurrentFanSpeed:    currentFanSpeeds[0],
		CurrentPowerLimit:  currentPowerLimit,
		AveragePowerLimit:  avgPowerLimit,
	}, nil
}

func setGPUState(state *GPUState) (GPUState, error) {
	targetFanSpeed := calculateFanSpeed(state.AverageTemperature, cfg.Temperature, cfg.FanSpeed)
	targetPowerLimit := calculatePowerLimit(state.CurrentTemperature, cfg.Temperature, state.CurrentFanSpeed, cfg.FanSpeed, state.CurrentPowerLimit)

	if state.AverageTemperature <= minTemperature {
		if !autoFanControl {
			if err := gpuDevice.EnableAutoFanControl(); err != nil {
				return *state, err
			}
			autoFanControl = true
		}
	} else {
		if autoFanControl {
			logger.Debug().Msgf("Temperature (%d°C) above minimum (%d°C). Switching to manual fan control.",
				state.AverageTemperature, minTemperature)
			autoFanControl = false
		}

		if !autoFanControl && !applyHysteresis(targetFanSpeed, state.CurrentFanSpeed, cfg.Hysteresis) {
			if err := gpuDevice.SetFanSpeed(targetFanSpeed); err != nil {
				return *state, err
			}
			logger.Debug().Msgf("Fan speed changed from %d to %d", state.CurrentFanSpeed, targetFanSpeed)
		}
	}

	if !cfg.Performance {
		if !applyHysteresis(targetPowerLimit, state.CurrentPowerLimit, powerLimitHysteresis) {
			if err := gpuDevice.SetPowerLimit(targetPowerLimit); err != nil {
				return *state, err
			}
			logger.Debug().Msgf("Power limit changed from %d to %d", state.CurrentPowerLimit, targetPowerLimit)
		}
	} else {
		maxPowerLimit := gpuDevice.GetPowerLimits().Max
		if state.CurrentPowerLimit < maxPowerLimit {
			if err := gpuDevice.SetPowerLimit(maxPowerLimit); err != nil {
				return *state, err
			}
			logger.Debug().Msgf("Power limit set to max: %d", maxPowerLimit)
			targetPowerLimit = maxPowerLimit
		}
	}

	state.TargetFanSpeed = targetFanSpeed
	state.TargetPowerLimit = targetPowerLimit

	return *state, nil
}

func logGPUState(state GPUState) {
	if cfg.Debug {
		lastFanSpeeds := gpuDevice.GetLastFanSpeeds()
		powerLimits := gpuDevice.GetPowerLimits()
		fanSpeedLimits := gpuDevice.GetFanSpeedLimits()

		logger.Debug().
			Int("current_fan_speed", state.CurrentFanSpeed).
			Int("target_fan_speed", state.TargetFanSpeed).
			Interface("last_set_fan_speeds", lastFanSpeeds).
			Int("max_fan_speed", cfg.FanSpeed).
			Int("current_temperature", state.CurrentTemperature).
			Int("average_temperature", state.AverageTemperature).
			Int("min_temperature", minTemperature).
			Int("max_temperature", cfg.Temperature).
			Int("current_power_limit", state.CurrentPowerLimit).
			Int("target_power_limit", state.TargetPowerLimit).
			Int("average_power_limit", state.AveragePowerLimit).
			Int("min_power_limit", powerLimits.Min).
			Int("max_power_limit", powerLimits.Max).
			Int("min_fan_speed", fanSpeedLimits.Min).
			Int("max_fan_speed", fanSpeedLimits.Max).
			Int("hysteresis", cfg.Hysteresis).
			Bool("monitor", cfg.Monitor).
			Bool("performance", cfg.Performance).
			Bool("auto_fan_control", autoFanControl).
			Msg("")
	} else if cfg.Verbose || cfg.Monitor {
		logger.Info().
			Int("fan_speed", state.CurrentFanSpeed).
			Int("target_fan_speed", state.TargetFanSpeed).
			Int("temperature", state.CurrentTemperature).
			Int("avg_temperature", state.AverageTemperature).
			Int("power_limit", state.CurrentPowerLimit).
			Int("target_power_limit", state.TargetPowerLimit).
			Int("avg_power_limit", state.AveragePowerLimit).
			Msg("")
	}
}

func calculateFanSpeed(averageTemperature, maxTemperature, configMaxFanSpeed int) int {
	fanSpeedLimits := gpuDevice.GetFanSpeedLimits()
	minFanSpeed := fanSpeedLimits.Min
	maxFanSpeed := fanSpeedLimits.Max

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
		return math.Pow(tempPercentage, performancePowFactor)
	}

	return math.Pow(tempPercentage, normalPowFactor)
}

func calculatePowerLimit(
	currentTemperature, targetTemperature, currentFanSpeed, maxFanSpeed, currentPowerLimit int,
) int {
	powerLimits := gpuDevice.GetPowerLimits()

	tempDiff := currentTemperature - targetTemperature
	if tempDiff > 0 && currentFanSpeed >= maxFanSpeed {
		adjustment := min(tempDiff*wattsPerDegree, maxPowerLimitChange)

		return clamp(currentPowerLimit-adjustment, powerLimits.Min, powerLimits.Max)
	}

	if tempDiff < 0 {
		adjustment := min(-tempDiff*wattsPerDegree, maxPowerLimitChange)

		return clamp(currentPowerLimit+adjustment, powerLimits.Min, powerLimits.Max)
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
