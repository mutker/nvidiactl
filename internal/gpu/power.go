package gpu

import (
	"math"
	"sync"

	"codeberg.org/mutker/nvidiactl/internal/errors"
	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	milliWattsToWatts    = 1000
	powerLimitWindowSize = 5
)

type powerController struct {
	device       nvml.Device
	limits       PowerLimits
	currentLimit PowerLimit
	lastLimit    PowerLimit
	powerHistory []PowerLimit
	mu           sync.RWMutex
	logger       logger.Logger
}

func newPowerController(device nvml.Device, log logger.Logger) (PowerController, error) {
	errFactory := errors.New()
	pc := &powerController{
		device:       device,
		powerHistory: make([]PowerLimit, 0, powerLimitWindowSize),
		logger:       log,
	}

	minLimit, maxLimit, ret := device.GetPowerManagementLimitConstraints()
	if !IsNVMLSuccess(ret) {
		return nil, errFactory.Wrap(ErrPowerLimitsFailed, newNVMLError(ret))
	}

	defaultLimit, ret := device.GetPowerManagementDefaultLimit()
	if !IsNVMLSuccess(ret) {
		return nil, errFactory.Wrap(ErrPowerLimitsFailed, newNVMLError(ret))
	}

	pc.limits = PowerLimits{
		Min:     PowerLimit(minLimit / milliWattsToWatts),
		Max:     PowerLimit(maxLimit / milliWattsToWatts),
		Default: PowerLimit(defaultLimit / milliWattsToWatts),
	}

	currentLimit, ret := device.GetPowerManagementLimit()
	if !IsNVMLSuccess(ret) {
		return nil, errFactory.Wrap(ErrPowerLimitFailed, newNVMLError(ret))
	}

	pc.currentLimit = PowerLimit(currentLimit / milliWattsToWatts)
	pc.lastLimit = pc.currentLimit
	pc.powerHistory = append(pc.powerHistory, pc.currentLimit)

	return pc, nil
}

func (pc *powerController) GetLimit() (PowerLimit, error) {
	errFactory := errors.New()
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	limit, ret := pc.device.GetPowerManagementLimit()
	if !IsNVMLSuccess(ret) {
		return 0, errFactory.Wrap(ErrPowerLimitFailed, newNVMLError(ret))
	}

	return PowerLimit(limit / milliWattsToWatts), nil
}

func (pc *powerController) SetLimit(limit PowerLimit) error {
	errFactory := errors.New()
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if limit < pc.limits.Min || limit > pc.limits.Max {
		return errFactory.WithData(errors.ErrInvalidArgument, "power limit out of range")
	}

	limitInMilliWatts := wattsToMilliWatts(limit)
	ret := pc.device.SetPowerManagementLimit(limitInMilliWatts)
	if !IsNVMLSuccess(ret) {
		return errFactory.Wrap(ErrSetPowerLimit, newNVMLError(ret))
	}

	pc.lastLimit = pc.currentLimit
	pc.currentLimit = limit

	return nil
}

func (pc *powerController) GetLimits() PowerLimits {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.limits
}

func (pc *powerController) GetLastLimit() PowerLimit {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.lastLimit
}

func (pc *powerController) GetCurrentLimit() PowerLimit {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	limit, ret := pc.device.GetPowerManagementLimit()
	if !IsNVMLSuccess(ret) {
		pc.logger.Debug().Msgf("Failed to get power limit: %s", nvml.ErrorString(ret))
		return pc.currentLimit
	}

	currentLimit := PowerLimit(limit / milliWattsToWatts)
	pc.logger.Debug().Int("powerLimit", int(currentLimit)).Msg("Current power limit retrieved")

	return currentLimit
}

func (pc *powerController) ResetToDefault() error {
	return pc.SetLimit(pc.limits.Default)
}

func (pc *powerController) UpdateHistory(limit PowerLimit) PowerLimit {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.powerHistory = append(pc.powerHistory, limit)
	if len(pc.powerHistory) > powerLimitWindowSize {
		pc.powerHistory = pc.powerHistory[1:]
	}

	var sum PowerLimit
	for _, l := range pc.powerHistory {
		sum += l
	}

	return sum / PowerLimit(len(pc.powerHistory))
}

func wattsToMilliWatts(watts PowerLimit) uint32 {
	if watts <= 0 {
		return 0
	}

	// Add explicit bounds checking
	const maxWatts = PowerLimit(math.MaxUint32 / milliWattsToWatts)
	if watts > maxWatts {
		return math.MaxUint32
	}

	result := watts * PowerLimit(milliWattsToWatts)

	//nolint:gosec // G115: Safe - bounds checked above
	return uint32(result)
}
