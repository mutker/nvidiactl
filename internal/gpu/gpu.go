package gpu

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	temperatureWindowSize = 5
	powerLimitWindowSize  = 5
	milliWattsToWatts     = 1000
)

var (
	ErrUninitializedGPU  = errors.New("GPU not initialized")
	ErrNVMLFailure       = errors.New("NVML operation failed")
	ErrPowerLimitTooHigh = errors.New("power limit too high")
	ErrPowerLimitTooLow  = errors.New("power limit too low")
)

type Limits struct {
	Min, Max, Default int
}

type GPU struct {
	device             nvml.Device
	fanCount           int
	fanSpeedLimits     Limits
	powerLimits        Limits
	currentFanSpeeds   []int
	lastFanSpeeds      []int
	currentPowerLimit  int
	lastPowerLimit     int
	temperatureHistory []int
	powerLimitHistory  []int
	mu                 sync.RWMutex
}

func New() (*GPU, error) {
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	device, ret := nvml.DeviceGetHandleByIndex(0)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	g := &GPU{device: device}
	if err := g.initialize(); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *GPU) initialize() error {
	if name, ret := g.device.GetName(); ret == nvml.SUCCESS {
		logger.Info().Msgf("Detected GPU: %v", name)
	} else {
		logger.Warn().Msgf("Failed to get GPU name: %v", nvml.ErrorString(ret))
	}

	var err error
	if g.fanCount, err = g.GetFanCount(); err != nil {
		return fmt.Errorf("failed to get GPU fan count: %w", err)
	}
	logger.Debug().Msgf("Detected fans: %d", g.fanCount)

	if err := g.initFanSpeed(); err != nil {
		return fmt.Errorf("failed to initialize fan speed: %w", err)
	}

	if err := g.initPowerLimits(); err != nil {
		return fmt.Errorf("failed to initialize power limits: %w", err)
	}

	return nil
}

func (g *GPU) initFanSpeed() error {
	var err error
	g.fanSpeedLimits, err = g.getMinMaxFanSpeed()
	if err != nil {
		return err
	}

	g.currentFanSpeeds = make([]int, g.fanCount)
	g.lastFanSpeeds = make([]int, g.fanCount)

	for i := 0; i < g.fanCount; i++ {
		if g.currentFanSpeeds[i], err = g.getFanSpeed(i); err != nil {
			return fmt.Errorf("failed to get current fan speed for fan %d: %w", i, err)
		}
		g.lastFanSpeeds[i] = g.currentFanSpeeds[i]
	}

	logger.Debug().Msgf("Detected fan speeds: %v", g.currentFanSpeeds)

	return nil
}

func (g *GPU) initPowerLimits() error {
	var err error
	g.powerLimits, err = g.GetMinMaxPowerLimits()
	if err != nil {
		return err
	}

	if g.currentPowerLimit, err = g.GetPowerLimit(); err != nil {
		return err
	}

	logger.Debug().Msgf("Detected power limit: %dW", g.currentPowerLimit)

	return g.SetPowerLimit(g.powerLimits.Default)
}

func (g *GPU) Shutdown() error {
	return nvml.Shutdown()
}

func (g *GPU) GetFanCount() (int, error) {
	count, ret := g.device.GetNumFans()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	return count, nil
}

func (g *GPU) GetTemperature() (int, error) {
	temp, ret := g.device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	return int(temp), nil
}

func (g *GPU) getFanSpeed(fanIndex int) (int, error) {
	speed, ret := g.device.GetFanSpeed_v2(fanIndex)
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	return int(speed), nil
}

func (g *GPU) getMinMaxFanSpeed() (Limits, error) {
	minLimit, maxLimit, ret := g.device.GetMinMaxFanSpeed()
	if ret != nvml.SUCCESS {
		return Limits{}, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	return Limits{Min: minLimit, Max: maxLimit}, nil
}

func (g *GPU) SetFanSpeed(fanSpeed int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := 0; i < g.fanCount; i++ {
		if ret := nvml.DeviceSetFanSpeed_v2(g.device, i, fanSpeed); ret != nvml.SUCCESS {
			return fmt.Errorf("%w: failed to set fan %d speed: %v", ErrNVMLFailure, i, nvml.ErrorString(ret))
		}
		g.lastFanSpeeds[i] = g.currentFanSpeeds[i]
		g.currentFanSpeeds[i] = fanSpeed
	}
	logger.Debug().Msgf("Set fan speed: %d%%", fanSpeed)

	return nil
}

func (g *GPU) EnableAutoFanControl() error {
	for i := 0; i < g.fanCount; i++ {
		if ret := nvml.DeviceSetDefaultFanSpeed_v2(g.device, i); ret != nvml.SUCCESS {
			return fmt.Errorf("%w: failed to set default fan speed for fan %d: %v", ErrNVMLFailure, i, nvml.ErrorString(ret))
		}
	}
	logger.Debug().Msg("Auto fan control: enabled")

	return nil
}

func (g *GPU) GetPowerLimit() (int, error) {
	limit, ret := g.device.GetPowerManagementLimit()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	return int(limit / milliWattsToWatts), nil
}

func (g *GPU) GetPowerLimits() Limits {
	return g.powerLimits
}

func (g *GPU) GetFanSpeedLimits() Limits {
	return g.fanSpeedLimits
}

func (g *GPU) GetMinMaxPowerLimits() (Limits, error) {
	minLimit, maxLimit, ret := g.device.GetPowerManagementLimitConstraints()
	if ret != nvml.SUCCESS {
		return Limits{}, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	def, ret := g.device.GetPowerManagementDefaultLimit()
	if ret != nvml.SUCCESS {
		return Limits{}, fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	return Limits{
		Min:     int(minLimit / milliWattsToWatts),
		Max:     int(maxLimit / milliWattsToWatts),
		Default: int(def / milliWattsToWatts),
	}, nil
}

func (g *GPU) SetPowerLimit(powerLimit int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check for negative values
	if powerLimit < 0 {
		return fmt.Errorf("%w: %d", ErrPowerLimitTooLow, powerLimit)
	}

	// Check for potential overflow
	if powerLimit > math.MaxUint32/milliWattsToWatts {
		return fmt.Errorf("%w: %d", ErrPowerLimitTooHigh, powerLimit)
	}

	limitInMilliWatts := uint32(powerLimit) * milliWattsToWatts

	if ret := g.device.SetPowerManagementLimit(limitInMilliWatts); ret != nvml.SUCCESS {
		return fmt.Errorf("%w: %v", ErrNVMLFailure, nvml.ErrorString(ret))
	}

	logger.Debug().Msgf("Set power limit: %dW", powerLimit)
	g.lastPowerLimit = g.currentPowerLimit
	g.currentPowerLimit = powerLimit

	return nil
}

func (g *GPU) UpdateTemperatureHistory(currentTemperature int) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.temperatureHistory = append(g.temperatureHistory, currentTemperature)
	if len(g.temperatureHistory) > temperatureWindowSize {
		g.temperatureHistory = g.temperatureHistory[1:]
	}

	sum := 0
	for _, temp := range g.temperatureHistory {
		sum += temp
	}

	return sum / len(g.temperatureHistory)
}

func (g *GPU) UpdatePowerLimitHistory(newPowerLimit int) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.powerLimitHistory = append(g.powerLimitHistory, newPowerLimit)
	if len(g.powerLimitHistory) > powerLimitWindowSize {
		g.powerLimitHistory = g.powerLimitHistory[1:]
	}

	sum := 0
	for _, limit := range g.powerLimitHistory {
		sum += limit
	}

	return sum / len(g.powerLimitHistory)
}

func (g *GPU) GetCurrentFanSpeeds() []int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.currentFanSpeeds
}

func (g *GPU) GetLastFanSpeeds() []int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastFanSpeeds
}

func (g *GPU) GetCurrentPowerLimit() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.currentPowerLimit
}

func (g *GPU) GetLastPowerLimit() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastPowerLimit
}

func (g *GPU) ClampFanSpeed(fanSpeed int) int {
	return clamp(fanSpeed, g.fanSpeedLimits.Min, g.fanSpeedLimits.Max)
}

func (g *GPU) ClampPowerLimit(powerLimit int) int {
	return clamp(powerLimit, g.powerLimits.Min, g.powerLimits.Max)
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
