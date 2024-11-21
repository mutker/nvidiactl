package gpu

import (
	"sync"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	defaultDeviceIndex    = 0
	temperatureWindowSize = 5
)

type controller struct {
	nvml            nvmlController
	device          nvml.Device
	fanController   FanController
	powerController PowerController
	tempHistory     []Temperature
	tempMu          sync.RWMutex // Separate mutex for temperature history
	initialized     bool
	mu              sync.RWMutex
}

// New creates a new GPU controller instance
func New() (Controller, error) {
	c := &controller{
		nvml:        &nvmlWrapper{},
		tempHistory: make([]Temperature, 0, temperatureWindowSize),
	}

	return c, nil
}

// Initialize prepares the GPU controller for operation
func (c *controller) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	logger.Debug().Msg("Initializing NVML...")
	if err := c.nvml.Initialize(); err != nil {
		logger.Debug().Err(err).Msg("NVML initialization failed")
		return errors.Wrap(ErrInitFailed, err)
	}
	logger.Debug().Msg("NVML initialized successfully")

	logger.Debug().Msg("Getting GPU device...")
	device, err := c.nvml.GetDevice(defaultDeviceIndex)
	if err != nil {
		logger.Debug().Err(err).Msg("Failed to get GPU device")
		return errors.Wrap(ErrInitFailed, err)
	}
	logger.Debug().Msg("GPU device acquired")
	c.device = device

	// Initialize fan controller
	logger.Debug().Msg("Initializing fan controller...")
	fanCtrl, err := newFanController(device)
	if err != nil {
		logger.Debug().Err(err).Msg("Failed to initialize fan controller")
		return errors.Wrap(ErrInitFailed, err)
	}
	logger.Debug().Msg("Fan controller initialized")
	c.fanController = fanCtrl

	// Initialize power controller
	logger.Debug().Msg("Initializing power controller...")
	powerCtrl, err := newPowerController(device)
	if err != nil {
		logger.Debug().Err(err).Msg("Failed to initialize power controller")
		return errors.Wrap(ErrInitFailed, err)
	}
	logger.Debug().Msg("Power controller initialized")
	c.powerController = powerCtrl

	c.initialized = true
	logger.Debug().Msg("GPU controller initialization complete")

	return nil
}

// Shutdown performs cleanup of GPU resources
func (c *controller) Shutdown() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.initialized {
		return nil
	}

	logger.Debug().Msg("Shutting down GPU controller...")
	if err := c.nvml.Shutdown(); err != nil {
		logger.Debug().Err(err).Msg("NVML shutdown failed")
		return errors.Wrap(ErrShutdownFailed, err)
	}

	c.initialized = false
	logger.Debug().Msg("GPU controller shutdown complete")

	return nil
}

// GetTemperature returns the current GPU temperature
func (c *controller) GetTemperature() (Temperature, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return 0, errors.New(ErrNotInitialized)
	}

	logger.Debug().Msg("Reading GPU temperature...")
	temp, ret := c.device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret != nvml.SUCCESS {
		logger.Debug().Str("error", nvml.ErrorString(ret)).Msg("Failed to read temperature")
		return 0, errors.Wrap(ErrTemperatureReadFailed, nvml.ErrorString(ret))
	}

	temperature := Temperature(temp)
	logger.Debug().Int("temperature", int(temperature)).Msg("Temperature read successful")

	return temperature, nil
}

// GetAverageTemperature returns the moving average of GPU temperature
func (c *controller) GetAverageTemperature() Temperature {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.tempHistory) == 0 {
		return 0
	}

	var sum Temperature
	for _, temp := range c.tempHistory {
		sum += temp
	}

	return sum / Temperature(len(c.tempHistory))
}

func (c *controller) UpdateTemperatureHistory(temp Temperature) Temperature {
	logger.Debug().Int("temp", int(temp)).Msg("Starting temperature history update")

	c.tempMu.Lock()
	defer c.tempMu.Unlock()

	c.tempHistory = append(c.tempHistory, temp)
	if len(c.tempHistory) > temperatureWindowSize {
		c.tempHistory = c.tempHistory[1:]
	}

	var sum Temperature
	for _, t := range c.tempHistory {
		sum += t
	}
	avg := sum / Temperature(len(c.tempHistory))

	logger.Debug().
		Int("historySize", len(c.tempHistory)).
		Int("average", int(avg)).
		Msg("Temperature history updated")

	return avg
}

// GetFanControl returns the fan controller interface
func (c *controller) GetFanControl() FanController {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fanController
}

// GetCurrentFanSpeeds returns the current speeds of all fans
func (c *controller) GetCurrentFanSpeeds() []FanSpeed {
	if c.fanController == nil {
		return nil
	}
	return c.fanController.GetCurrentSpeeds()
}

func (c *controller) SetFanSpeed(speed FanSpeed) error {
	if c.fanController == nil {
		return errors.New(ErrNotInitialized)
	}
	if err := c.fanController.SetSpeed(speed); err != nil {
		return errors.Wrap(ErrSetFanSpeed, err)
	}
	return nil
}

func (c *controller) GetLastFanSpeeds() []FanSpeed {
	if c.fanController == nil {
		return nil
	}
	return c.fanController.GetLastSpeeds()
}

func (c *controller) GetFanSpeedLimits() FanSpeedLimits {
	if c.fanController == nil {
		return FanSpeedLimits{}
	}
	return c.fanController.GetSpeedLimits()
}

// EnableAutoFanControl enables automatic fan control
func (c *controller) EnableAutoFanControl() error {
	if !c.initialized {
		return errors.New(ErrNotInitialized)
	}
	if err := c.fanController.EnableAuto(); err != nil {
		return errors.Wrap(ErrEnableAutoFan, err)
	}
	return nil
}

// DisableAutoFanControl disables automatic fan control
func (c *controller) DisableAutoFanControl() error {
	if !c.initialized {
		return errors.New(ErrNotInitialized)
	}
	if err := c.fanController.DisableAuto(); err != nil {
		return errors.Wrap(ErrDisableAutoFan, err)
	}
	return nil
}

// GetPowerControl returns the power controller interface
func (c *controller) GetPowerControl() PowerController {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.powerController
}

// GetCurrentPowerLimit returns the current power limit
func (c *controller) GetCurrentPowerLimit() PowerLimit {
	if c.powerController == nil {
		return 0
	}
	return c.powerController.GetCurrentLimit()
}

// SetPowerLimit sets the power limit
func (c *controller) SetPowerLimit(limit PowerLimit) error {
	if c.powerController == nil {
		return errors.New(ErrNotInitialized)
	}
	if err := c.powerController.SetLimit(limit); err != nil {
		return errors.Wrap(ErrSetPowerLimit, err)
	}
	return nil
}

// GetPowerLimits returns the power limit constraints
func (c *controller) GetPowerLimits() PowerLimits {
	if c.powerController == nil {
		return PowerLimits{}
	}
	return c.powerController.GetLimits()
}

func (c *controller) UpdatePowerLimitHistory(limit PowerLimit) PowerLimit {
	if c.powerController == nil {
		return 0
	}
	return c.powerController.UpdateHistory(limit)
}

// Name returns the GPU device name
func (c *controller) Name() (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return "", errors.New(ErrNotInitialized)
	}

	name, ret := c.device.GetName()
	if ret != nvml.SUCCESS {
		return "", errors.Wrap(ErrDeviceInfoFailed, nvml.ErrorString(ret))
	}

	return name, nil
}
