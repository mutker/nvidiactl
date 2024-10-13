package gpu

import (
	"errors"
	"fmt"
	"sync"

	"codeberg.org/mutker/nvidiactl/internal/logger"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	temperatureWindowSize = 5
	powerLimitWindowSize  = 5
)

var (
	deviceSync  sync.Once
	cacheGPU    nvml.Device
	initErr     error
	initialized bool

	fanCount           int
	minFanSpeedLimit   int
	maxFanSpeedLimit   int
	currentFanSpeeds   []int
	lastFanSpeeds      []int
	currentPowerLimit  int
	lastPowerLimit     int
	defaultPowerLimit  int
	minPowerLimit      int
	maxPowerLimit      int
	temperatureHistory []int
	powerLimitHistory  []int
)

func Initialize() error {
	deviceSync.Do(func() {
		ret := nvml.Init()
		if ret != nvml.SUCCESS {
			initErr = fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
			return
		}

		var device nvml.Device
		device, ret = nvml.DeviceGetHandleByIndex(0)
		if ret != nvml.SUCCESS {
			initErr = fmt.Errorf("failed to get device handle: %v", nvml.ErrorString(ret))
			return
		}
		cacheGPU = device

		initialized = true
	})

	if initialized {
		// Log GPU information only once after successful initialization
		name, err := GetName()
		if err == nil {
			logger.Info().Msgf("Detected GPU: %v", name)
		}

		fanCount, err := GetFanCount()
		if err == nil {
			logger.Info().Msgf("Detected GPU fans: %d", fanCount)
		}
	}

	return initErr
}

func InitializeSettings(maxFanSpeed, maxTemperature int) error {
	var err error

	fanCount, err = GetFanCount()
	if err != nil {
		return fmt.Errorf("failed to get GPU fan count: %w", err)
	}

	if err := initFanSpeed(maxFanSpeed); err != nil {
		return fmt.Errorf("failed to initialize fan speed: %w", err)
	}

	if err := initPowerLimits(); err != nil {
		return fmt.Errorf("failed to initialize power limits: %w", err)
	}

	return nil
}

func initFanSpeed(maxFanSpeed int) error {
	var err error
	minFanSpeedLimit, maxFanSpeedLimit, err = GetMinMaxFanSpeed()
	if err != nil {
		return fmt.Errorf("failed to get min/max fan speed: %v", err)
	}

	maxFanSpeedLimit = minInt(maxFanSpeedLimit, maxFanSpeed)

	currentFanSpeeds = make([]int, fanCount)
	lastFanSpeeds = make([]int, fanCount)

	for i := 0; i < fanCount; i++ {
		currentFanSpeeds[i], err = GetFanSpeed(i)
		if err != nil {
			return fmt.Errorf("failed to get current fan speed for fan %d: %v", i, err)
		}
		lastFanSpeeds[i] = currentFanSpeeds[i]
	}

	logger.Info().Msgf("Initial fan speeds detected: %v", currentFanSpeeds)
	return nil
}

func initPowerLimits() error {
	var err error
	minPowerLimit, maxPowerLimit, defaultPowerLimit, err = GetMinMaxPowerLimits()
	if err != nil {
		return fmt.Errorf("failed to get power limits: %v", err)
	}

	currentPowerLimit, err = GetPowerLimit()
	if err != nil {
		return fmt.Errorf("failed to get current power limit: %v", err)
	}

	logger.Debug().Msgf("Initial power limit: %dW", currentPowerLimit)
	return SetPowerLimit(defaultPowerLimit)
}

func Shutdown() {
	if initialized {
		ret := nvml.Shutdown()
		if ret != nvml.SUCCESS {
			logger.Error().Msgf("Failed to shutdown NVML: %v", nvml.ErrorString(ret))
		}
	}
}

func GetHandle() (nvml.Device, error) {
	if !initialized {
		return nil, errors.New("GPU not initialized")
	}
	return cacheGPU, nil
}

func GetName() (string, error) {
	device, err := GetHandle()
	if err != nil {
		return "", err
	}

	name, ret := device.GetName()
	if ret != nvml.SUCCESS {
		err := errors.New(nvml.ErrorString(ret))
		logger.Error().Err(err).Msg("failed to get GPU name")
		return "", err
	}

	return name, nil
}

func GetFanCount() (int, error) {
	device, err := GetHandle()
	if err != nil {
		return 0, err
	}

	count, ret := device.GetNumFans()
	if ret != nvml.SUCCESS {
		err := fmt.Errorf("failed to get number of fans: %v", nvml.ErrorString(ret))
		logger.Error().Err(err).Msg("failed to get GPU fan count")
		return 0, err
	}

	return int(count), nil
}

func GetTemperature() (int, error) {
	device, _ := GetHandle()
	temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret != nvml.SUCCESS {
		err := errors.New(nvml.ErrorString(ret))
		logger.Error().Err(err).Msg("failed to get GPU temperature")
		return 0, err
	}
	return int(temp), nil
}

func GetFanSpeed(fanIndex int) (int, error) {
	device, err := GetHandle()
	if err != nil {
		return 0, fmt.Errorf("failed to get device handle: %v", err)
	}

	fanSpeed, ret := device.GetFanSpeed_v2(fanIndex)
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get fan %d speed: %v", fanIndex, nvml.ErrorString(ret))
	}

	return int(fanSpeed), nil
}

func GetMinMaxFanSpeed() (minSpeed, maxSpeed int, err error) {
	device, _ := GetHandle()
	minSpeedUint, maxSpeedUint, ret := device.GetMinMaxFanSpeed()
	if ret != nvml.SUCCESS {
		err = fmt.Errorf("failed to get min/max fan speed: %v", nvml.ErrorString(ret))
		logger.Error().Err(err).Msg("failed to get fan speed limits")
		return
	}

	minSpeed, maxSpeed = int(minSpeedUint), int(maxSpeedUint)
	return
}

func SetFanSpeed(fanSpeed int) error {
	device, err := GetHandle()
	if err != nil {
		return err
	}

	for i := 0; i < fanCount; i++ {
		ret := nvml.DeviceSetFanSpeed_v2(device, i, fanSpeed)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to set fan %d speed: %v", i, nvml.ErrorString(ret))
		}
		lastFanSpeeds[i] = currentFanSpeeds[i]
		currentFanSpeeds[i] = fanSpeed
	}
	logger.Debug().Msgf("Set all fans speed to %d%%", fanSpeed)
	return nil
}

func EnableAutoFanControl() error {
	device, err := GetHandle()
	if err != nil {
		return err
	}

	fanCount, err := GetFanCount()
	if err != nil {
		return fmt.Errorf("failed to get fan count: %v", err)
	}

	for i := 0; i < fanCount; i++ {
		ret := nvml.DeviceSetDefaultFanSpeed_v2(device, i)
		if ret != nvml.SUCCESS {
			err := fmt.Errorf("failed to set default fan speed for fan %d: %v", i, nvml.ErrorString(ret))
			logger.Error().Err(err).Msg("failed to set default fan speed")
			return err
		}
	}
	logger.Debug().Msg("Enabled auto fan control for all fans")
	return nil
}

func GetPowerLimit() (int, error) {
	device, err := GetHandle()
	if err != nil {
		return 0, err
	}

	powerLimit, ret := device.GetPowerManagementLimit()
	if ret != nvml.SUCCESS {
		err := fmt.Errorf("failed to get current power limit: %v", nvml.ErrorString(ret))
		logger.Error().Err(err).Msg("failed to get current power limit")
		return 0, err
	}

	return int(powerLimit / 1000), nil // Convert milliwatts to watts
}

func GetMinMaxPowerLimits() (minLimit, maxLimit, defaultLimit int, err error) {
	device, _ := GetHandle()
	minLimitUint, maxLimitUint, ret := device.GetPowerManagementLimitConstraints()
	if ret != nvml.SUCCESS {
		err = fmt.Errorf("failed to get power management limit constraints: %v", nvml.ErrorString(ret))
		logger.Error().Err(err).Msg("failed to get power management limit constraints")
		return
	}

	defaultLimitUint, ret := device.GetPowerManagementDefaultLimit()
	if ret != nvml.SUCCESS {
		err = fmt.Errorf("failed to get default power management limit: %v", nvml.ErrorString(ret))
		logger.Error().Err(err).Msg("failed to get default power management limit")
		return
	}

	minLimit, maxLimit, defaultLimit = int(minLimitUint/1000), int(maxLimitUint/1000), int(defaultLimitUint/1000)
	return
}

func SetPowerLimit(powerLimit int) error {
	device, _ := GetHandle()
	ret := device.SetPowerManagementLimit(uint32(powerLimit * 1000)) // Convert watts to milliwatts
	if ret != nvml.SUCCESS {
		err := fmt.Errorf("failed to set power limit: %v", nvml.ErrorString(ret))
		logger.Error().Err(err).Msg("failed to set power limit")
		return err
	}
	logger.Debug().Msgf("Set power limit to %dW", powerLimit)
	lastPowerLimit = currentPowerLimit
	currentPowerLimit = powerLimit
	return nil
}

func UpdateTemperatureHistory(currentTemperature int) int {
	temperatureHistory = append(temperatureHistory, currentTemperature)
	if len(temperatureHistory) > temperatureWindowSize {
		temperatureHistory = temperatureHistory[1:]
	}

	sum := 0
	for _, temp := range temperatureHistory {
		sum += temp
	}
	return sum / len(temperatureHistory)
}

func UpdatePowerLimitHistory(newPowerLimit int) int {
	powerLimitHistory = append(powerLimitHistory, newPowerLimit)
	if len(powerLimitHistory) > powerLimitWindowSize {
		powerLimitHistory = powerLimitHistory[1:]
	}

	sum := 0
	for _, limit := range powerLimitHistory {
		sum += limit
	}
	return sum / len(powerLimitHistory)
}

func GetCurrentFanSpeeds() ([]int, error) {
	return currentFanSpeeds, nil
}

func GetLastFanSpeeds() ([]int, error) {
	return lastFanSpeeds, nil
}

func GetCurrentPowerLimit() (int, error) {
	return currentPowerLimit, nil
}

func GetLastPowerLimit() (int, error) {
	return lastPowerLimit, nil
}

func GetMinFanSpeedLimit() (int, error) {
	return minFanSpeedLimit, nil
}

func GetMaxFanSpeedLimit() (int, error) {
	return maxFanSpeedLimit, nil
}

func GetMinPowerLimit() (int, error) {
	return minPowerLimit, nil
}

func GetMaxPowerLimit() (int, error) {
	return maxPowerLimit, nil
}

func ClampFanSpeed(fanSpeed int) int {
	if fanSpeed < minFanSpeedLimit {
		return minFanSpeedLimit
	}
	if fanSpeed > maxFanSpeedLimit {
		return maxFanSpeedLimit
	}
	return fanSpeed
}

func ClampPowerLimit(powerLimit int) int {
	if powerLimit < minPowerLimit {
		return minPowerLimit
	}
	if powerLimit > maxPowerLimit {
		return maxPowerLimit
	}
	return powerLimit
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
