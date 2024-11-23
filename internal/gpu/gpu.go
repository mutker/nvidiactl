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

func New() (Controller, error) {
	c := &controller{
		nvml:        &nvmlWrapper{},
		tempHistory: make([]Temperature, 0, temperatureWindowSize),
	}
	return c, nil
}

func (c *controller) Initialize() error {
	errFactory := errors.New()
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	logger.Debug().Msg("Initializing NVML...")
	if err := c.nvml.Initialize(); err != nil {
		logger.Debug().Err(err).Msg("NVML initialization failed")
		return errFactory.Wrap(ErrInitFailed, err)
	}

	logger.Debug().Msg("Getting GPU device...")
	device, err := c.nvml.GetDevice(defaultDeviceIndex)
	if err != nil {
		logger.Debug().Err(err).Msg("Failed to get GPU device")
		return errFactory.Wrap(ErrDeviceNotFound, err)
	}
	c.device = device

	logger.Debug().Msg("Initializing fan controller...")
	fanCtrl, err := newFanController(device)
	if err != nil {
		logger.Debug().Err(err).Msg("Failed to initialize fan controller")
		return errFactory.Wrap(ErrInitFailed, err)
	}
	c.fanController = fanCtrl

	logger.Debug().Msg("Initializing power controller...")
	powerCtrl, err := newPowerController(device)
	if err != nil {
		logger.Debug().Err(err).Msg("Failed to initialize power controller")
		return errFactory.Wrap(ErrInitFailed, err)
	}
	c.powerController = powerCtrl

	c.initialized = true

	return nil
}

// Shutdown performs cleanup of GPU resources
func (c *controller) Shutdown() error {
	errFactory := errors.New()
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.initialized {
		return nil
	}

	if err := c.nvml.Shutdown(); err != nil {
		logger.Debug().Err(err).Msg("NVML shutdown failed")
		return errFactory.Wrap(ErrShutdownFailed, err)
	}

	c.initialized = false

	return nil
}

// GetTemperature returns the current GPU temperature
func (c *controller) GetTemperature() (Temperature, error) {
	errFactory := errors.New()
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return 0, errFactory.New(ErrNotInitialized)
	}

	temp, ret := c.device.GetTemperature(nvml.TEMPERATURE_GPU)
	if !IsNVMLSuccess(ret) {
		err := newNVMLError(ret)
		logger.Debug().Err(err).Msg("Failed to read temperature")
		return 0, errFactory.Wrap(ErrTemperatureReadFailed, err)
	}

	return Temperature(temp), nil
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
		Int("avgTemperature", int(avg)).
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
	errFactory := errors.New()
	if c.fanController == nil {
		return errFactory.New(ErrNotInitialized)
	}
	if err := c.fanController.SetSpeed(speed); err != nil {
		return errFactory.Wrap(ErrSetFanSpeed, err)
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
	errFactory := errors.New()
	if !c.initialized {
		return errFactory.New(ErrNotInitialized)
	}
	if err := c.fanController.EnableAuto(); err != nil {
		return errFactory.Wrap(ErrEnableAutoFan, err)
	}
	return nil
}

// DisableAutoFanControl disables automatic fan control
func (c *controller) DisableAutoFanControl() error {
	errFactory := errors.New()
	if !c.initialized {
		return errFactory.New(ErrNotInitialized)
	}
	if err := c.fanController.DisableAuto(); err != nil {
		return errFactory.Wrap(ErrDisableAutoFan, err)
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
	errFactory := errors.New()
	if c.powerController == nil {
		return errFactory.New(ErrNotInitialized)
	}
	if err := c.powerController.SetLimit(limit); err != nil {
		return errFactory.Wrap(ErrSetPowerLimit, err)
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
	errFactory := errors.New()
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.initialized {
		return "", errFactory.New(ErrNotInitialized)
	}

	name, ret := c.device.GetName()
	if !IsNVMLSuccess(ret) {
		return "", errFactory.Wrap(ErrDeviceInfoFailed, newNVMLError(ret))
	}

	return name, nil
}
