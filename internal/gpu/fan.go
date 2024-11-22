package gpu

import (
	"sync"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type fanController struct {
	device     nvml.Device
	count      int
	limits     FanSpeedLimits
	speeds     []FanSpeed
	lastSpeeds []FanSpeed
	autoMode   bool
	mu         sync.RWMutex
}

func newFanController(device nvml.Device) (FanController, error) {
	fc := &fanController{
		device:   device,
		autoMode: true, // Assume auto mode by default
	}

	// Get fan count
	count, ret := device.GetNumFans()
	if ret != nvml.SUCCESS {
		return nil, errors.Wrap(ErrFanCountFailed, nvml.ErrorString(ret))
	}
	fc.count = count

	// Initialize speed slices
	fc.speeds = make([]FanSpeed, fc.count)
	fc.lastSpeeds = make([]FanSpeed, fc.count)

	// Get fan speed limits
	minSpeed, maxSpeed, ret := device.GetMinMaxFanSpeed()
	if ret != nvml.SUCCESS {
		return nil, errors.Wrap(ErrGetFanLimitsFailed, nvml.ErrorString(ret))
	}

	fc.limits = FanSpeedLimits{
		Min: FanSpeed(minSpeed),
		Max: FanSpeed(maxSpeed),
		// For default, we'll use the current speed as it represents the GPU's preferred speed
		Default: FanSpeed(minSpeed), // Will be updated below
	}

	// Get current speeds to initialize state
	for i := 0; i < fc.count; i++ {
		speed, ret := device.GetFanSpeed_v2(i)
		if ret != nvml.SUCCESS {
			return nil, errors.Wrap(ErrGetFanSpeedFailed, nvml.ErrorString(ret))
		}
		fc.speeds[i] = FanSpeed(speed)
		fc.lastSpeeds[i] = FanSpeed(speed)
	}

	// Use the first fan's current speed as the default
	if fc.count > 0 {
		fc.limits.Default = fc.speeds[0]
	}

	return fc, nil
}

func (fc *fanController) GetSpeed(fanIndex int) (FanSpeed, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if fanIndex < 0 || fanIndex >= fc.count {
		return 0, errors.WithData(errors.ErrInvalidArgument, "fan index out of range")
	}

	speed, ret := fc.device.GetFanSpeed_v2(fanIndex)
	if ret != nvml.SUCCESS {
		return 0, errors.Wrap(ErrGetFanSpeedFailed, nvml.ErrorString(ret))
	}

	return FanSpeed(speed), nil
}

func (fc *fanController) SetSpeed(speed FanSpeed) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Validate speed is within limits
	if speed < fc.limits.Min || speed > fc.limits.Max {
		return errors.WithData(errors.ErrInvalidArgument, "fan speed out of range")
	}

	// Store current speeds as last speeds
	copy(fc.lastSpeeds, fc.speeds)

	// Set speed for all fans
	for i := 0; i < fc.count; i++ {
		if ret := nvml.DeviceSetFanSpeed_v2(fc.device, i, int(speed)); ret != nvml.SUCCESS {
			return errors.Wrap(ErrFanControlFailed, nvml.ErrorString(ret))
		}
		fc.speeds[i] = speed
	}

	fc.autoMode = false

	return nil
}

func (fc *fanController) GetSpeedLimits() FanSpeedLimits {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.limits
}

func (fc *fanController) EnableAuto() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Store current speeds as last speeds
	copy(fc.lastSpeeds, fc.speeds)

	// Enable auto mode for all fans
	for i := 0; i < fc.count; i++ {
		if ret := nvml.DeviceSetDefaultFanSpeed_v2(fc.device, i); ret != nvml.SUCCESS {
			return errors.Wrap(ErrFanControlFailed, nvml.ErrorString(ret))
		}
	}

	fc.autoMode = true

	return nil
}

func (fc *fanController) DisableAuto() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// When disabling auto mode, maintain current speeds
	for i := 0; i < fc.count; i++ {
		currentSpeed, ret := fc.device.GetFanSpeed_v2(i)
		if ret != nvml.SUCCESS {
			return errors.Wrap(ErrGetFanSpeedFailed, nvml.ErrorString(ret))
		}

		// Set the current speed explicitly to disable auto mode
		if ret := nvml.DeviceSetFanSpeed_v2(fc.device, i, int(currentSpeed)); ret != nvml.SUCCESS {
			return errors.Wrap(ErrFanControlFailed, nvml.ErrorString(ret))
		}

		fc.speeds[i] = FanSpeed(currentSpeed)
	}

	fc.autoMode = false

	return nil
}

func (fc *fanController) IsAutoMode() bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.autoMode
}

func (fc *fanController) GetLastSpeeds() []FanSpeed {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	speeds := make([]FanSpeed, len(fc.lastSpeeds))
	copy(speeds, fc.lastSpeeds)

	return speeds
}

func (fc *fanController) GetCurrentSpeeds() []FanSpeed {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	logger.Debug().Msg("Getting current fan speeds from controller")
	speeds := make([]FanSpeed, len(fc.speeds))
	copy(speeds, fc.speeds)

	// Update current speeds
	for i := 0; i < fc.count; i++ {
		speed, ret := fc.device.GetFanSpeed_v2(i)
		if ret != nvml.SUCCESS {
			logger.Debug().Msgf("Failed to get fan %d speed: %s", i, nvml.ErrorString(ret))
			continue
		}
		speeds[i] = FanSpeed(speed)
	}

	logger.Debug().Interface("speeds", speeds).Msg("Current fan speeds retrieved")

	return speeds
}
