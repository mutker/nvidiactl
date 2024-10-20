package main

import (
	"context"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeberg.org/mutker/nvidiactl/internal/config"
	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/gpu"
	"codeberg.org/mutker/nvidiactl/internal/logger"
	"codeberg.org/mutker/nvidiactl/internal/telemetry"
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

type AppState struct {
	cfg            *config.Config
	autoFanControl bool
	gpuDevice      *gpu.GPU
	telemetry      *telemetry.Database
}

func main() {
	a, err := New()
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.ErrorWithCode(appErr).Send()
		} else {
			logger.ErrorWithCode(errors.Wrap(errors.ErrMainLoop, err)).Send()
		}
	}
	defer func() {
		err := a.gpuDevice.Shutdown()
		if err != nil && !errors.IsNVMLSuccess(err) {
			logger.ErrorWithCode(errors.Wrap(errors.ErrShutdownGPU, err)).Send()
		} else {
			logger.Info().Msg("nvidiactl shutdown completed successfully.")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go handleSignals(cancel)

	if err := a.loop(ctx); err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			logger.ErrorWithCode(appErr).Send()
		} else {
			logger.ErrorWithCode(errors.Wrap(errors.ErrMainLoop, err)).Send()
		}
	}
	a.cleanup()
}

func New() (*AppState, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, errors.Wrap(errors.ErrInitApp, err)
	}

	logger.Init(cfg.Debug, cfg.Verbose, logger.IsService())
	logger.Debug().Msg("Config loaded")

	gpuDevice, err := gpu.New()
	if err != nil {
		return nil, errors.Wrap(errors.ErrInitApp, err)
	}

	var telemetryInstance *telemetry.Database
	if cfg.Telemetry {
		var err error
		telemetryInstance, err = telemetry.NewTelemetryDB(cfg.TelemetryDB)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to initialize telemetry DB")
			return nil, errors.Wrap(errors.ErrInitApp, err)
		}
		logger.Debug().Msg("Telemetry collection enabled")
	} else {
		logger.Debug().Msg("Telemetry collection disabled")
	}

	return &AppState{
		cfg:       cfg,
		gpuDevice: gpuDevice,
		telemetry: telemetryInstance,
	}, nil
}

func handleSignals(cancel context.CancelFunc) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	logger.Info().Msg("Received termination signal.")
	cancel()
}

func (a *AppState) loop(ctx context.Context) error {
	if a.cfg.Interval <= 0 {
		return errors.New(errors.ErrInvalidInterval)
	}

	interval := time.Duration(a.cfg.Interval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if a.cfg.Monitor {
		logger.Info().Msg("Monitor mode activated. Logging GPU status...")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			state, err := a.getGPUState()
			if err != nil {
				return err
			}

			if !a.cfg.Monitor {
				state, err = a.setGPUState(&state)
				if err != nil {
					return err
				}
			} else {
				// In monitor mode, calculate target values without applying them
				state.TargetFanSpeed = a.calculateFanSpeed(state.AverageTemperature, a.cfg.Temperature, a.cfg.FanSpeed)
				state.TargetPowerLimit = a.calculatePowerLimit(state.CurrentTemperature, a.cfg.Temperature,
					state.CurrentFanSpeed, a.cfg.FanSpeed, state.CurrentPowerLimit)
			}

			a.logGPUState(state)
		}
	}
}

func (a *AppState) cleanup() {
	powerLimits := a.gpuDevice.GetPowerLimits()
	powerLimitToSet := min(powerLimits.Default, powerLimits.Max)
	if err := a.gpuDevice.SetPowerLimit(powerLimitToSet); err != nil {
		logger.ErrorWithCode(errors.Wrap(errors.ErrResetPowerLimit, err)).Send()
	}
	if err := a.gpuDevice.EnableAutoFanControl(); err != nil {
		logger.ErrorWithCode(errors.Wrap(errors.ErrEnableAutoFan, err)).Send()
	}
	if a.telemetry != nil {
		if err := a.telemetry.Close(); err != nil {
			logger.Error().Err(err).Msg("Failed to close telemetry")
		}
	}
	logger.Info().Msg("Exiting...")
}

func (a *AppState) getGPUState() (GPUState, error) {
	currentTemperature, err := a.gpuDevice.GetTemperature()
	if err != nil {
		return GPUState{}, errors.Wrap(errors.ErrGetGPUState, err)
	}

	currentFanSpeeds := a.gpuDevice.GetCurrentFanSpeeds()
	currentPowerLimit := a.gpuDevice.GetCurrentPowerLimit()

	avgTemp := a.gpuDevice.UpdateTemperatureHistory(currentTemperature)
	avgPowerLimit := a.gpuDevice.UpdatePowerLimitHistory(currentPowerLimit)

	return GPUState{
		CurrentTemperature: currentTemperature,
		AverageTemperature: avgTemp,
		CurrentFanSpeed:    currentFanSpeeds[0],
		CurrentPowerLimit:  currentPowerLimit,
		AveragePowerLimit:  avgPowerLimit,
	}, nil
}

func (a *AppState) setGPUState(state *GPUState) (GPUState, error) {
	targetFanSpeed := a.calculateFanSpeed(state.AverageTemperature, a.cfg.Temperature, a.cfg.FanSpeed)
	targetPowerLimit := a.calculatePowerLimit(state.CurrentTemperature, a.cfg.Temperature,
		state.CurrentFanSpeed, a.cfg.FanSpeed, state.CurrentPowerLimit)

	if err := a.handleFanControl(state, targetFanSpeed); err != nil {
		return *state, errors.Wrap(errors.ErrSetGPUState, err)
	}

	if err := a.handlePowerLimit(state, targetPowerLimit); err != nil {
		return *state, errors.Wrap(errors.ErrSetGPUState, err)
	}

	state.TargetFanSpeed = targetFanSpeed
	state.TargetPowerLimit = targetPowerLimit

	return *state, nil
}

func (a *AppState) handleFanControl(state *GPUState, targetFanSpeed int) error {
	if state.AverageTemperature <= minTemperature {
		if !a.autoFanControl {
			if err := a.gpuDevice.EnableAutoFanControl(); err != nil {
				return errors.Wrap(errors.ErrEnableAutoFan, err)
			}
			a.autoFanControl = true
		}
	} else {
		if a.autoFanControl {
			logger.Debug().Msgf("Temperature (%d°C) above minimum (%d°C). Switching to manual fan control.",
				state.AverageTemperature, minTemperature)
			a.autoFanControl = false
		}
		if !a.autoFanControl && !applyHysteresis(targetFanSpeed, state.CurrentFanSpeed, a.cfg.Hysteresis) {
			if err := a.gpuDevice.SetFanSpeed(targetFanSpeed); err != nil {
				return errors.Wrap(errors.ErrSetFanSpeed, err)
			}
			logger.Debug().Msgf("Fan speed changed from %d to %d", state.CurrentFanSpeed, targetFanSpeed)
		}
	}

	return nil
}

func (a *AppState) handlePowerLimit(state *GPUState, targetPowerLimit int) error {
	if !a.cfg.Performance {
		if !applyHysteresis(targetPowerLimit, state.CurrentPowerLimit, powerLimitHysteresis) {
			if err := a.gpuDevice.SetPowerLimit(targetPowerLimit); err != nil {
				return errors.Wrap(errors.ErrSetPowerLimit, err)
			}
			logger.Debug().Msgf("Power limit changed from %d to %d", state.CurrentPowerLimit, targetPowerLimit)
		}
	} else {
		maxPowerLimit := a.gpuDevice.GetPowerLimits().Max
		if state.CurrentPowerLimit < maxPowerLimit {
			if err := a.gpuDevice.SetPowerLimit(maxPowerLimit); err != nil {
				return errors.Wrap(errors.ErrSetPowerLimit, err)
			}
			logger.Debug().Msgf("Power limit set to max: %d", maxPowerLimit)
		}
	}

	return nil
}

func (a *AppState) logGPUState(state GPUState) {
	if a.cfg.Debug {
		lastFanSpeeds := a.gpuDevice.GetLastFanSpeeds()
		powerLimits := a.gpuDevice.GetPowerLimits()
		fanSpeedLimits := a.gpuDevice.GetFanSpeedLimits()

		logger.Debug().
			Int("current_fan_speed", state.CurrentFanSpeed).
			Int("target_fan_speed", state.TargetFanSpeed).
			Interface("last_set_fan_speeds", lastFanSpeeds).
			Int("max_fan_speed", a.cfg.FanSpeed).
			Int("current_temperature", state.CurrentTemperature).
			Int("average_temperature", state.AverageTemperature).
			Int("min_temperature", minTemperature).
			Int("max_temperature", a.cfg.Temperature).
			Int("current_power_limit", state.CurrentPowerLimit).
			Int("target_power_limit", state.TargetPowerLimit).
			Int("average_power_limit", state.AveragePowerLimit).
			Int("min_power_limit", powerLimits.Min).
			Int("max_power_limit", powerLimits.Max).
			Int("min_fan_speed", fanSpeedLimits.Min).
			Int("max_fan_speed", fanSpeedLimits.Max).
			Int("hysteresis", a.cfg.Hysteresis).
			Bool("monitor", a.cfg.Monitor).
			Bool("performance", a.cfg.Performance).
			Bool("auto_fan_control", a.autoFanControl).
			Msg("")
	} else if a.cfg.Verbose {
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
	// Record telemetry data if enabled
	if a.cfg.Telemetry && a.telemetry != nil {
		a.telemetry.CollectMetrics(&telemetry.Metrics{
			Timestamp:          time.Now(),
			FanSpeed:           state.CurrentFanSpeed,
			TargetFanSpeed:     state.TargetFanSpeed,
			Temperature:        state.CurrentTemperature,
			AverageTemperature: state.AverageTemperature,
			PowerLimit:         state.CurrentPowerLimit,
			TargetPowerLimit:   state.TargetPowerLimit,
			AveragePowerLimit:  state.AveragePowerLimit,
			AutoFanControl:     a.autoFanControl,
			PerformanceMode:    a.cfg.Performance,
		})
	}
}

func (a *AppState) calculateFanSpeed(averageTemperature, maxTemperature, configMaxFanSpeed int) int {
	fanSpeedLimits := a.gpuDevice.GetFanSpeedLimits()
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

	fanSpeedPercentage := a.calculateFanSpeedPercentage(tempPercentage)
	fanSpeedRange := maxFanSpeed - minFanSpeed
	targetFanSpeed := int(float64(fanSpeedRange)*fanSpeedPercentage) + minFanSpeed

	return clamp(targetFanSpeed, minFanSpeed, maxFanSpeed)
}

func (a *AppState) calculateFanSpeedPercentage(tempPercentage float64) float64 {
	if a.cfg.Performance {
		return math.Pow(tempPercentage, performancePowFactor)
	}

	return math.Pow(tempPercentage, normalPowFactor)
}

func (a *AppState) calculatePowerLimit(
	currentTemperature, targetTemperature, currentFanSpeed, maxFanSpeed, currentPowerLimit int,
) int {
	powerLimits := a.gpuDevice.GetPowerLimits()

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
