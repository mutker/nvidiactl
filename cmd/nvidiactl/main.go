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
	"codeberg.org/mutker/nvidiactl/internal/metrics"
	"codeberg.org/mutker/nvidiactl/internal/pid"
)

const (
	minTemperature       = 50
	powerLimitWindowSize = 5
	maxPowerLimitChange  = 10
	wattsPerDegree       = 5
	powerLimitHysteresis = 5
	performancePowFactor = 1.5
	normalPowFactor      = 2.0
	cleanupTimeout       = 5 * time.Second
	operationTimeout     = 2 * time.Second
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
	cfg            config.Provider
	autoFanControl bool
	gpuDevice      gpu.Controller
	metrics        metrics.MetricsCollector
	logger         logger.Logger
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	errFactory := errors.New()

	// Initialize with default log level first
	log := logger.New(string(config.LogLevelInfo), logger.IsService())

	if err := pid.Write(); err != nil {
		var domainErr errors.Error
		if !errors.As(err, &domainErr) {
			domainErr = errFactory.Wrap(errors.ErrInitApp, err)
		}
		log.ErrorWithCode(domainErr).Send()
		return errFactory.Wrap(errors.ErrInitApp, err)
	}
	defer func() {
		if err := pid.Remove(); err != nil {
			log.Error().Err(err).Msg("Failed to remove PID file")
		}
	}()

	a, err := initialize(log)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown signal
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		a.logger.Info().Msgf("Received termination signal: %v", sig)

		// First cancel the context to stop the main loop
		cancel()

		// Then perform cleanup once
		cleanupDone := make(chan struct{})
		go func() {
			a.cleanup()
			close(cleanupDone)
		}()

		select {
		case <-cleanupDone:
			a.logger.Info().Msg("Graceful shutdown completed")
			os.Exit(0)
		case <-time.After(cleanupTimeout):
			a.logger.Error().Msg("Forced shutdown after timeout")
			os.Exit(1)
		}
	}()

	if err := a.loop(ctx); err != nil {
		var domainErr errors.Error
		if !errors.As(err, &domainErr) {
			domainErr = errFactory.Wrap(errors.ErrMainLoop, err)
		}
		a.logger.ErrorWithCode(domainErr).Send()
		a.cleanup()
		return err
	}
	return nil
}

func initialize(log logger.Logger) (*AppState, error) {
	errFactory := errors.New()

	log.Debug().
		Str("config_env", os.Getenv("NVIDIACTL_CONFIG")).
		Msg("Starting nvidiactl...")

	// Create application state
	a, err := newApplication(log)
	if err != nil {
		var domainErr errors.Error
		if !errors.As(err, &domainErr) {
			domainErr = errFactory.Wrap(errors.ErrMainLoop, err)
		}
		log.ErrorWithCode(domainErr).Send()
		return nil, err
	}

	// Re-initialize logger with config settings
	if a.cfg.GetLogLevel() != string(config.DefaultLogLevel) {
		a.logger = logger.New(a.cfg.GetLogLevel(), logger.IsService())
	}

	a.logger.Info().
		Str("log_level", a.cfg.GetLogLevel()).
		Bool("monitor_mode", a.cfg.IsMonitorMode()).
		Bool("performance_mode", a.cfg.IsPerformanceMode()).
		Bool("metrics", a.cfg.IsMetricsEnabled()).
		Msg("Configuration loaded and applied")

	return a, nil
}

func newApplication(log logger.Logger) (*AppState, error) {
	errFactory := errors.New()

	loader := config.NewLoader(log)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		log.Debug().Err(err).Msg("Failed to load configuration")
		return nil, errFactory.Wrap(errors.ErrInitApp, err)
	}

	// Re-initialize logger with config settings
	if cfg.GetLogLevel() != string(config.DefaultLogLevel) {
		log = logger.New(cfg.GetLogLevel(), logger.IsService())
	}

	gpuDevice, err := gpu.New(log)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to create GPU controller")
		return nil, errFactory.Wrap(errors.ErrInitApp, err)
	}

	if err := gpuDevice.Initialize(); err != nil {
		log.Debug().Err(err).Msg("Failed to initialize GPU controller")
		return nil, errFactory.Wrap(errors.ErrInitApp, err)
	}

	var collector metrics.MetricsCollector
	if cfg.IsMetricsEnabled() {
		collector, err = metrics.NewService(metrics.Config{
			DBPath:  cfg.GetMetricsDBPath(),
			Enabled: cfg.IsMetricsEnabled(),
		}, log)
		if err != nil {
			var appErr errors.Error
			if !errors.As(err, &appErr) {
				appErr = errFactory.Wrap(errors.ErrInitMetrics, err)
			}
			log.ErrorWithCode(appErr).Msg("Failed to initialize metrics collection")
			return nil, errFactory.Wrap(errors.ErrInitApp, err)
		}
	}

	return &AppState{
		cfg:       cfg,
		gpuDevice: gpuDevice,
		metrics:   collector,
		logger:    log,
	}, nil
}

func (a *AppState) loop(ctx context.Context) error {
	errFactory := errors.New()

	if a.cfg.GetInterval() <= 0 {
		return errFactory.New(errors.ErrInvalidInterval)
	}

	interval := time.Duration(a.cfg.GetInterval()) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if a.cfg.IsMonitorMode() {
		a.logger.Info().Msg("Monitor mode activated. Logging GPU status...")
	}

	a.logger.Debug().Msgf("Starting main loop with %v interval", interval)

	for {
		select {
		case <-ctx.Done():
			a.logger.Debug().Msg("Context canceled, exiting loop")
			return nil
		case <-ticker.C:
			a.logger.Debug().Msg("Updating GPU state...")

			state, err := a.getGPUState()
			if err != nil {
				a.logger.Debug().Err(err).Msg("Failed to get GPU state")
				return err
			}

			if !a.cfg.IsMonitorMode() {
				state, err = a.setGPUState(&state)
				if err != nil {
					a.logger.Debug().Err(err).Msg("Failed to set GPU state")
					return err
				}
			} else {
				state.TargetFanSpeed = a.calculateFanSpeed(state.AverageTemperature, a.cfg.GetTemperature(), a.cfg.GetFanSpeed())
				state.TargetPowerLimit = a.calculatePowerLimit(state.CurrentTemperature, a.cfg.GetTemperature(),
					state.CurrentFanSpeed, a.cfg.GetFanSpeed(), state.CurrentPowerLimit)
			}

			a.logGPUState(ctx, state)
		}
	}
}

func (a *AppState) cleanup() {
	errFactory := errors.New()
	a.logger.Debug().Msg("Starting application cleanup...")

	if a.gpuDevice != nil {
		powerLimits := a.gpuDevice.GetPowerLimits()
		powerLimitToSet := min(powerLimits.Default, powerLimits.Max)
		if err := a.gpuDevice.SetPowerLimit(powerLimitToSet); err != nil {
			a.logger.ErrorWithCode(errFactory.Wrap(errors.ErrResetPowerLimit, err)).Send()
		}

		if err := a.gpuDevice.EnableAutoFanControl(); err != nil {
			a.logger.ErrorWithCode(errFactory.Wrap(errors.ErrEnableAutoFan, err)).Send()
		}

		if err := a.gpuDevice.Shutdown(); err != nil {
			a.logger.ErrorWithCode(errFactory.Wrap(errors.ErrShutdownGPU, err)).Send()
		}
	}

	if a.metrics != nil {
		if err := a.metrics.Close(); err != nil {
			a.logger.Error().Err(err).Msg("Failed to close metrics")
		}
	}

	a.logger.Info().Msg("Exiting...")
}

func (a *AppState) getGPUState() (GPUState, error) {
	errFactory := errors.New()
	a.logger.Debug().Msg("Getting GPU state...")

	// Get temperature with timeout
	tempChan := make(chan gpu.Temperature)
	tempErrChan := make(chan error)
	go func() {
		temp, err := a.gpuDevice.GetTemperature()
		if err != nil {
			tempErrChan <- err
			return
		}
		tempChan <- temp
	}()

	var currentTemperature gpu.Temperature
	select {
	case temp := <-tempChan:
		currentTemperature = temp
		a.logger.Debug().Int("temperature", int(currentTemperature)).Msg("Current temperature retrieved")
	case err := <-tempErrChan:
		a.logger.Debug().Err(err).Msg("Failed to get temperature")
		return GPUState{}, errFactory.Wrap(errors.ErrGetGPUState, err)
	case <-time.After(operationTimeout):
		return GPUState{}, errFactory.New(errors.ErrGetGPUState)
	}

	// Get fan speeds
	a.logger.Debug().Msg("Getting current fan speeds...")
	currentFanSpeeds := a.gpuDevice.GetCurrentFanSpeeds()
	a.logger.Debug().Interface("fanSpeeds", currentFanSpeeds).Msg("Current fan speeds retrieved")

	// Get power limit
	a.logger.Debug().Msg("Getting current power limit...")
	currentPowerLimit := a.gpuDevice.GetCurrentPowerLimit()

	// Update histories with timeout
	historyChan := make(chan struct{})
	var avgTemp gpu.Temperature
	var avgPowerLimit gpu.PowerLimit

	go func() {
		avgTemp = a.gpuDevice.UpdateTemperatureHistory(currentTemperature)

		avgPowerLimit = a.gpuDevice.UpdatePowerLimitHistory(currentPowerLimit)
		a.logger.Debug().Int("avgPowerLimit", int(avgPowerLimit)).Msg("Power limit history updated")

		close(historyChan)
	}()

	select {
	case <-historyChan:
		// History updates completed successfully
		a.logger.Debug().Msg("Power and temperature history updates completed successfully")
	case <-time.After(operationTimeout):
		a.logger.Warn().Msg("Power and temperature history updates timed out")
		// Use current values as averages if history update times out
		avgTemp = currentTemperature
		avgPowerLimit = currentPowerLimit
	}

	state := GPUState{
		CurrentTemperature: int(currentTemperature),
		AverageTemperature: int(avgTemp),
		CurrentFanSpeed:    int(currentFanSpeeds[0]),
		CurrentPowerLimit:  int(currentPowerLimit),
		AveragePowerLimit:  int(avgPowerLimit),
	}

	return state, nil
}

func (a *AppState) setGPUState(state *GPUState) (GPUState, error) {
	errFactory := errors.New()

	targetFanSpeed := a.calculateFanSpeed(state.AverageTemperature, a.cfg.GetTemperature(), a.cfg.GetFanSpeed())
	targetPowerLimit := a.calculatePowerLimit(state.CurrentTemperature, a.cfg.GetTemperature(),
		state.CurrentFanSpeed, a.cfg.GetFanSpeed(), state.CurrentPowerLimit)

	if err := a.handleFanControl(state, targetFanSpeed); err != nil {
		return *state, errFactory.Wrap(errors.ErrSetGPUState, err)
	}

	if err := a.handlePowerLimit(state, targetPowerLimit); err != nil {
		return *state, errFactory.Wrap(errors.ErrSetGPUState, err)
	}

	state.TargetFanSpeed = targetFanSpeed
	state.TargetPowerLimit = targetPowerLimit

	return *state, nil
}

func (a *AppState) logGPUState(ctx context.Context, state GPUState) {
	if a.cfg.GetLogLevel() == "debug" {
		lastFanSpeeds := a.gpuDevice.GetLastFanSpeeds()
		powerLimits := a.gpuDevice.GetPowerLimits()

		// If auto fan control is enabled, show target fan speed as 0
		targetFanSpeed := state.TargetFanSpeed
		if a.autoFanControl {
			targetFanSpeed = 0
		}

		a.logger.Debug().
			Int("current_fan_speed", state.CurrentFanSpeed).
			Int("target_fan_speed", targetFanSpeed).
			Interface("last_set_fan_speeds", lastFanSpeeds).
			Int("max_fan_speed", a.cfg.GetFanSpeed()).
			Int("current_temperature", state.CurrentTemperature).
			Int("average_temperature", state.AverageTemperature).
			Int("min_temperature", minTemperature).
			Int("max_temperature", a.cfg.GetTemperature()).
			Int("current_power_limit", state.CurrentPowerLimit).
			Int("target_power_limit", state.TargetPowerLimit).
			Int("average_power_limit", state.AveragePowerLimit).
			Int("current_power_limit", state.CurrentPowerLimit).
			Int("target_power_limit", state.TargetPowerLimit).
			Int("average_power_limit", state.AveragePowerLimit).
			Int("min_power_limit", int(powerLimits.Min)).
			Int("max_power_limit", int(powerLimits.Max)).
			Int("hysteresis", a.cfg.GetHysteresis()).
			Bool("monitor", a.metrics.IsReadOnly()).
			Bool("performance", a.cfg.IsPerformanceMode()).
			Bool("auto_fan_control", a.autoFanControl).
			Msg("")
	} else if a.cfg.GetLogLevel() == "info" {
		targetFanSpeed := state.TargetFanSpeed
		if a.autoFanControl {
			targetFanSpeed = 0
		}

		a.logger.Info().
			Int("current_fan_speed", state.CurrentFanSpeed).
			Int("max_fan_speed", a.cfg.GetFanSpeed()).
			Int("target_fan_speed", targetFanSpeed).
			Int("current_temperature", state.CurrentTemperature).
			Int("max_temperature", a.cfg.GetTemperature()).
			Int("current_power_limit", state.CurrentPowerLimit).
			Int("target_power_limit", state.TargetPowerLimit).
			Msg("")
	}

	// Collect metrics in database, if enabled
	if a.metrics != nil && !a.metrics.IsReadOnly() {
		snapshot := &metrics.MetricsSnapshot{
			Timestamp: time.Now(),
			FanSpeed: metrics.FanMetrics{
				Current: state.CurrentFanSpeed,
				Target:  state.TargetFanSpeed,
			},
			Temperature: metrics.TempMetrics{
				Current: state.CurrentTemperature,
				Average: state.AverageTemperature,
			},
			PowerLimit: metrics.PowerMetrics{
				Current: state.CurrentPowerLimit,
				Target:  state.TargetPowerLimit,
				Average: state.AveragePowerLimit,
			},
			SystemState: metrics.StateMetrics{
				AutoFanControl:  a.autoFanControl,
				PerformanceMode: a.cfg.IsPerformanceMode(),
			},
		}

		if err := a.metrics.Record(ctx, snapshot); err != nil {
			errFactory := errors.New()
			a.logger.ErrorWithCode(errFactory.Wrap(errors.ErrCollectMetrics, err)).Send()
		}
	}
}

func (a *AppState) handleFanControl(state *GPUState, targetFanSpeed int) error {
	errFactory := errors.New()

	if state.AverageTemperature <= minTemperature {
		if !a.autoFanControl {
			if err := a.gpuDevice.EnableAutoFanControl(); err != nil {
				return errFactory.Wrap(errors.ErrEnableAutoFan, err)
			}
			a.autoFanControl = true
		}
	} else {
		if a.autoFanControl {
			a.logger.Debug().Msgf("Temperature (%d°C) above minimum (%d°C). Switching to manual fan control.",
				state.AverageTemperature, minTemperature)
			a.autoFanControl = false
		}
		if !a.autoFanControl && !applyHysteresis(targetFanSpeed, state.CurrentFanSpeed, a.cfg.GetHysteresis()) {
			if err := a.gpuDevice.SetFanSpeed(gpu.FanSpeed(targetFanSpeed)); err != nil {
				return errFactory.Wrap(gpu.ErrSetFanSpeed, err)
			}
			a.logger.Debug().Msgf("Fan speed changed from %d to %d", state.CurrentFanSpeed, targetFanSpeed)
		}
	}

	return nil
}

func (a *AppState) handlePowerLimit(state *GPUState, targetPowerLimit int) error {
	errFactory := errors.New()

	if !a.cfg.IsPerformanceMode() {
		if !applyHysteresis(targetPowerLimit, state.CurrentPowerLimit, powerLimitHysteresis) {
			if err := a.gpuDevice.SetPowerLimit(gpu.PowerLimit(targetPowerLimit)); err != nil {
				return errFactory.Wrap(gpu.ErrSetPowerLimit, err)
			}
			a.logger.Debug().Msgf("Power limit changed from %d to %d", state.CurrentPowerLimit, targetPowerLimit)
		}
	} else {
		maxPowerLimit := a.gpuDevice.GetPowerLimits().Max
		if state.CurrentPowerLimit < int(maxPowerLimit) {
			if err := a.gpuDevice.SetPowerLimit(maxPowerLimit); err != nil {
				return errFactory.Wrap(gpu.ErrSetPowerLimit, err)
			}
			a.logger.Debug().Msgf("Power limit set to max: %d", maxPowerLimit)
		}
	}

	return nil
}

func (a *AppState) calculateFanSpeed(averageTemperature, maxTemperature, configMaxFanSpeed int) int {
	fanSpeedLimits := a.gpuDevice.GetFanSpeedLimits()
	minFanSpeed := fanSpeedLimits.Min
	maxFanSpeed := fanSpeedLimits.Max

	maxFanSpeed = gpu.FanSpeed(min(int(maxFanSpeed), configMaxFanSpeed))

	if averageTemperature <= minTemperature {
		return int(minFanSpeed)
	}

	if averageTemperature >= maxTemperature {
		return int(maxFanSpeed)
	}

	tempRange := float64(maxTemperature - minTemperature)
	tempPercentage := float64(averageTemperature-minTemperature) / tempRange

	fanSpeedPercentage := a.calculateFanSpeedPercentage(tempPercentage)
	fanSpeedRange := int(maxFanSpeed) - int(minFanSpeed)
	targetFanSpeed := int(float64(fanSpeedRange)*fanSpeedPercentage) + int(minFanSpeed)

	return clamp(targetFanSpeed, int(minFanSpeed), int(maxFanSpeed))
}

func (a *AppState) calculateFanSpeedPercentage(tempPercentage float64) float64 {
	if a.cfg.IsPerformanceMode() {
		return math.Pow(tempPercentage, performancePowFactor)
	}

	return math.Pow(tempPercentage, normalPowFactor)
}

func (a *AppState) calculatePowerLimit(
	currentTemperature, targetTemperature, currentFanSpeed, maxFanSpeed, currentPowerLimit int,
) int {
	powerLimits := a.gpuDevice.GetPowerLimits()

	tempDiff := currentTemperature - targetTemperature

	// As a secondary control, decrease power limit if temperature is high and fan is at max.
	if tempDiff > 0 && currentFanSpeed >= maxFanSpeed {
		adjustment := min(tempDiff*wattsPerDegree, maxPowerLimitChange)
		return clamp(currentPowerLimit-adjustment, int(powerLimits.Min), int(powerLimits.Max))
	}

	// Symmetrically, if the temperature is safely below the target, restore the power limit.
	// To prevent the "ratcheting down" effect, the restoration must be more aggressive
	// than the reduction to overcome the power limit hysteresis.
	if tempDiff < 0 {
		// A more aggressive restoration factor ensures that even a small temperature drop
		// results in a power limit increase large enough to overcome the hysteresis.
		const restorationFactor = 2
		adjustment := min(-tempDiff*wattsPerDegree*restorationFactor, maxPowerLimitChange)
		return clamp(currentPowerLimit+adjustment, int(powerLimits.Min), int(powerLimits.Max))
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
