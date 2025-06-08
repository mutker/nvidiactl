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
	logger     logger.Logger
}

func newFanController(device nvml.Device, log logger.Logger) (FanController, error) {
	errFactory := errors.New()
	fc := &fanController{
		device:   device,
		autoMode: true,
		logger:   log,
	}

	count, ret := device.GetNumFans()
	if !IsNVMLSuccess(ret) {
		return nil, errFactory.Wrap(ErrFanCountFailed, newNVMLError(ret))
	}
	fc.count = count

	fc.speeds = make([]FanSpeed, fc.count)
	fc.lastSpeeds = make([]FanSpeed, fc.count)

	minSpeed, maxSpeed, ret := device.GetMinMaxFanSpeed()
	if !IsNVMLSuccess(ret) {
		return nil, errFactory.Wrap(ErrGetFanLimitsFailed, newNVMLError(ret))
	}

	fc.limits = FanSpeedLimits{
		Min:     FanSpeed(minSpeed),
		Max:     FanSpeed(maxSpeed),
		Default: FanSpeed(minSpeed),
	}

	for i := 0; i < fc.count; i++ {
		speed, ret := device.GetFanSpeed_v2(i)
		if !IsNVMLSuccess(ret) {
			return nil, errFactory.Wrap(ErrGetFanSpeedFailed, newNVMLError(ret))
		}
		fc.speeds[i] = FanSpeed(speed)
		fc.lastSpeeds[i] = FanSpeed(speed)
	}

	if fc.count > 0 {
		fc.limits.Default = fc.speeds[0]
	}

	return fc, nil
}

func (fc *fanController) GetSpeed(fanIndex int) (FanSpeed, error) {
	errFactory := errors.New()
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	if fanIndex < 0 || fanIndex >= fc.count {
		return 0, errFactory.WithData(errors.ErrInvalidArgument, "fan index out of range")
	}

	speed, ret := fc.device.GetFanSpeed_v2(fanIndex)
	if !IsNVMLSuccess(ret) {
		return 0, errFactory.Wrap(ErrGetFanSpeedFailed, newNVMLError(ret))
	}

	return FanSpeed(speed), nil
}

func (fc *fanController) SetSpeed(speed FanSpeed) error {
	errFactory := errors.New()
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if speed < fc.limits.Min || speed > fc.limits.Max {
		return errFactory.WithData(errors.ErrInvalidArgument, "fan speed out of range")
	}

	copy(fc.lastSpeeds, fc.speeds)

	for i := 0; i < fc.count; i++ {
		if ret := nvml.DeviceSetFanSpeed_v2(fc.device, i, int(speed)); !IsNVMLSuccess(ret) {
			return errFactory.Wrap(ErrSetFanSpeed, newNVMLError(ret))
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
	errFactory := errors.New()
	fc.mu.Lock()
	defer fc.mu.Unlock()

	copy(fc.lastSpeeds, fc.speeds)

	for i := 0; i < fc.count; i++ {
		if ret := nvml.DeviceSetDefaultFanSpeed_v2(fc.device, i); !IsNVMLSuccess(ret) {
			return errFactory.Wrap(ErrFanControlFailed, newNVMLError(ret))
		}
	}

	fc.autoMode = true

	return nil
}

func (fc *fanController) DisableAuto() error {
	errFactory := errors.New()
	fc.mu.Lock()
	defer fc.mu.Unlock()

	for i := 0; i < fc.count; i++ {
		currentSpeed, ret := fc.device.GetFanSpeed_v2(i)
		if !IsNVMLSuccess(ret) {
			return errFactory.Wrap(ErrGetFanSpeedFailed, newNVMLError(ret))
		}

		if ret := nvml.DeviceSetFanSpeed_v2(fc.device, i, int(currentSpeed)); !IsNVMLSuccess(ret) {
			return errFactory.Wrap(ErrFanControlFailed, newNVMLError(ret))
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

	speeds := make([]FanSpeed, len(fc.speeds))
	copy(speeds, fc.speeds)

	// Update current speeds
	for i := 0; i < fc.count; i++ {
		speed, ret := fc.device.GetFanSpeed_v2(i)
		if ret != nvml.SUCCESS {
			fc.logger.Debug().Msgf("Failed to get fan %d speed: %s", i, nvml.ErrorString(ret))
			continue
		}
		speeds[i] = FanSpeed(speed)
	}

	fc.logger.Debug().Interface("fanSpeeds", speeds).Msg("Current fan speeds retrieved")

	return speeds
}
